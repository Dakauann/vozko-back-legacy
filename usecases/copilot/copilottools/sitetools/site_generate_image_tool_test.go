package sitetools

import (
	"context"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/mediagen"
	"vozko/domain/workspace"
	"vozko/usecases/copilot/copilottools/tooltest"
)

const generatedMedia = "9c8b7a6f-5e4d-4c3b-8a2f-1e0d9c8b7a6f"

func generated() *mediagen.Job {
	return &mediagen.Job{ID: "job-1", Kind: mediagen.KindImage, Status: mediagen.StatusDone, MediaID: generatedMedia, MediaURL: "https://cdn.example.com/images/ws-1/new.jpg", Model: tooltest.PickedImageModel, Prompt: "consultório"}
}

func generateArgs(t *testing.T, extra string) map[string]interface{} {
	t.Helper()
	return args(t, `{"prompt":"consultório odontológico claro","aspect":"landscape","image_model":"`+tooltest.PickedImageModel+`"`+extra+`}`)
}

func TestAGeneratedImageReplacesTheNodeThroughTheEditPath(t *testing.T) {
	r := newRig(t)
	r.images.Final = generated()
	res := NewSiteGenerateImageTool(r.deps).Execute(context.Background(), r.cc, generateArgs(t, `,"node":"src/routes/index.jsx#120","rev":1,"alt":"Consultório claro"`))
	if res.Status != copilot.StatusOK || res.Media == nil {
		t.Fatalf("got %+v", res)
	}
	if r.images.Requested.BillingReference != "aichat:"+threadID {
		t.Fatalf("the generation is billed to the thread, got %+v", r.images.Requested)
	}
	edit := r.storage.edits[0]
	if !strings.Contains(string(edit.Batch), `"op":"replace_image"`) || !strings.Contains(string(edit.Batch), generatedMedia) || !strings.Contains(string(edit.Batch), `"alt":"Consultório claro"`) {
		t.Fatalf("got %s", edit.Batch)
	}
	data := decoded(t, res.Data)
	if data["media_id"] != generatedMedia || len(r.editor.sent(copilot.ScreenEdit)) != 1 {
		t.Fatalf("got %+v", data)
	}
}

func TestWithoutANodeTheImageOnlyGoesToTheLibrary(t *testing.T) {
	r := newRig(t)
	r.images.Final = generated()
	res := NewSiteGenerateImageTool(r.deps).Execute(context.Background(), r.cc, generateArgs(t, ""))
	if res.Status != copilot.StatusOK || len(r.storage.edits) != 0 || decoded(t, res.Data)["media_id"] != generatedMedia {
		t.Fatalf("got %+v", res)
	}
}

func TestAPlacementThatCannotWorkIsRefusedBeforeBilling(t *testing.T) {
	r := newRig(t)
	r.images.Final = generated()
	tool := NewSiteGenerateImageTool(r.deps)
	for name, extra := range map[string]string{
		"no alt":       `,"node":"src/routes/index.jsx#120","rev":1`,
		"no rev":       `,"node":"src/routes/index.jsx#120","alt":"x"`,
		"missing file": `,"node":"src/Nada.jsx#4","rev":1,"alt":"x"`,
	} {
		if res := tool.Execute(context.Background(), r.cc, generateArgs(t, extra)); res.Status != copilot.StatusError {
			t.Errorf("%s: want a refusal, got %+v", name, res)
		}
		if err := tool.(copilot.Validator).Validate(context.Background(), r.cc, generateArgs(t, extra)); err == nil {
			t.Errorf("%s: the approval card must not be offered", name)
		}
	}
	if r.images.Requested.Kind != "" {
		t.Fatalf("nothing is generated for a placement that cannot work, got %+v", r.images.Requested)
	}
}

func TestAFailedPlacementKeepsTheMediaIDAndNeverRegenerates(t *testing.T) {
	r := newRig(t)
	r.images.Final = generated()
	r.storage.editErr = context.DeadlineExceeded
	res := NewSiteGenerateImageTool(r.deps).Execute(context.Background(), r.cc, generateArgs(t, `,"node":"src/routes/index.jsx#120","rev":1,"alt":"Consultório"`))
	if res.Status != copilot.StatusError || !strings.Contains(res.Message, generatedMedia) || !strings.Contains(res.Message, "nunca gere de novo") {
		t.Fatalf("got %+v", res)
	}
}

func TestImageGenerationNeedsMediaAndPageEditing(t *testing.T) {
	tool := NewSiteGenerateImageTool(Deps{})
	if tool.Meta().Resource != workspace.ResourceMedia {
		t.Fatalf("got %+v", tool.Meta())
	}
	also := tool.(interface {
		AlsoRequires() []workspace.PermissionEntry
	}).AlsoRequires()
	if len(also) != 1 || also[0].Resource != workspace.ResourceSites || also[0].Action != workspace.ActionUpdate {
		t.Fatalf("got %+v", also)
	}
	graded := tool.(copilot.Graded)
	if !graded.NeedsApproval(copilot.ModeAsk) || !graded.NeedsApproval(copilot.ModeEdit) || graded.NeedsApproval(copilot.ModeFull) {
		t.Fatal("paid generations ask unless the person chose Acesso total")
	}
}
