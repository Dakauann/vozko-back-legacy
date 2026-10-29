package sitetools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/site"
	site_usecase "vozko/usecases/site"
)

const headlineEdit = `{"label":"Título novo","operations":[{"op":"edit_file","path":"src/routes/index.jsx","old":"Agende sua avaliação gratuita e saia com um plano claro","new":"Sorria sem medo"}]}`

func TestAnEditRunsInGoAsEloAndTheEditorReplaysTheSteps(t *testing.T) {
	r := newRig(t)
	res := NewSiteEditTool(r.deps).Execute(context.Background(), r.cc, args(t, headlineEdit))
	if res.Status != copilot.StatusOK {
		t.Fatalf("got %+v", res)
	}
	edit := r.storage.edits[0]
	if edit.Author != site_usecase.EloAuthor(threadID) || edit.Label != "Título novo" || edit.ExpectedVersion != 3 || edit.WorkspaceID != "ws-1" {
		t.Fatalf("got %+v", edit)
	}
	sent := r.editor.sent(copilot.ScreenEdit)
	if len(sent) != 1 {
		t.Fatalf("the editor replays the batch once, got %d", len(sent))
	}
	payload := sent[0].Args.(editPayload)
	if len(payload.Base) != 1 || !strings.Contains(payload.Base["src/routes/index.jsx"].Content, "saia com um plano claro") || payload.Version != 4 || len(payload.Steps) != 1 {
		t.Fatalf("the editor gets the base it merges against and the steps, got %+v", payload)
	}
	committed := payload.Changes.Files["src/routes/index.jsx"]
	if committed.Rev != 2 || !strings.Contains(committed.Content, "Sorria sem medo") || payload.Changes.Deleted == nil || payload.Changes.Renamed == nil {
		t.Fatalf("the editor gets the committed files, got %+v", payload.Changes)
	}
	if step := payload.Steps[0]; step.Op != "edit_file" || step.Index != 0 || step.Files != nil || step.Deleted != nil || step.Renamed != nil {
		t.Fatalf("steps only carry what presence needs, got %+v", step)
	}
	raw, _ := json.Marshal(res.Data)
	text := string(raw)
	if !strings.Contains(text, `"changed":["src/routes/index.jsx r2"]`) || !strings.Contains(text, `"open_gaps":[]`) {
		t.Fatalf("got %s", text)
	}
	if strings.Contains(text, "Sorria sem medo") || strings.Contains(text, "className") {
		t.Fatal("an edit result never echoes file contents")
	}
}

func TestARefusedBatchListsEveryFailureOnceAndAppliesNothing(t *testing.T) {
	r := newRig(t)
	batch := `{"operations":[
		{"op":"edit_file","path":"src/Nada.jsx","old":"a","new":"b"},
		{"op":"edit_file","path":"src/Nada.jsx","old":"c","new":"d"},
		{"op":"set_text","node":"src/routes/index.jsx#10","rev":1,"text":"Oi","color":"red"}
	]}`
	res := NewSiteEditTool(r.deps).Execute(context.Background(), r.cc, args(t, batch))
	if res.Status != copilot.StatusError || len(r.editor.sent(copilot.ScreenEdit)) != 0 {
		t.Fatalf("got %+v", res)
	}
	for _, want := range []string{"nada foi aplicado", "operação 1 (edit_file): path_not_found em path", "operação 2 pulada: dependia da operação 1", "campo color ignorado"} {
		if !strings.Contains(res.Message, want) {
			t.Errorf("the refusal lacks %q: %s", want, res.Message)
		}
	}
}

func TestABuilderFailureOnAnOperationTellsEloHowToRecover(t *testing.T) {
	r := newRig(t)
	r.storage.editErr = &site_usecase.EditRefusal{Failures: []site.Failure{{Index: 0, Code: site.CodeOperationFailed}}}
	res := NewSiteEditTool(r.deps).Execute(context.Background(), r.cc, args(t, headlineEdit))
	if res.Status != copilot.StatusError || !strings.Contains(res.Message, "operação 1 (edit_file): operation_failed: "+codeWords[site.CodeOperationFailed]) || codeWords[site.CodeOperationFailed] == "" {
		t.Fatalf("got %+v", res)
	}
}

func TestAnEditRetriesOnceWhenThePersonSavedInBetween(t *testing.T) {
	r := newRig(t)
	r.storage.conflicts = 1
	res := NewSiteEditTool(r.deps).Execute(context.Background(), r.cc, args(t, headlineEdit))
	if res.Status != copilot.StatusOK || len(r.storage.edits) != 2 || r.storage.edits[1].ExpectedVersion != 4 {
		t.Fatalf("got %+v after %d edits", res, len(r.storage.edits))
	}
	r.storage.conflicts = 5
	if res := NewSiteEditTool(r.deps).Execute(context.Background(), r.cc, args(t, headlineEdit)); res.Status != copilot.StatusError || !strings.Contains(res.Message, "mudou duas vezes") {
		t.Fatalf("got %+v", res)
	}
}

func TestEditorConflictsComeBackAndAnAbsentEditorIsNotAFailure(t *testing.T) {
	r := newRig(t)
	r.editor.replies = map[copilot.ScreenCommandName]copilot.ScreenReply{
		copilot.ScreenEdit: {OK: true, Data: []byte(`{"files":{"src/routes/index.jsx":3},"conflicts":["src/routes/index.jsx:42-44"],"skipped":[0]}`)},
	}
	data := decoded(t, NewSiteEditTool(r.deps).Execute(context.Background(), r.cc, args(t, headlineEdit)).Data)
	if conflicts := data["conflicts"].([]interface{}); conflicts[0] != "src/routes/index.jsx:42-44" {
		t.Fatalf("got %+v", data)
	}
	if skipped := data["skipped_in_editor"].([]interface{}); skipped[0] != float64(1) {
		t.Fatalf("skipped steps are named by operation, got %+v", data)
	}
	r = newRig(t)
	r.editor.errs = map[copilot.ScreenCommandName]error{copilot.ScreenEdit: copilot.ErrScreenUnavailable}
	res := NewSiteEditTool(r.deps).Execute(context.Background(), r.cc, args(t, headlineEdit))
	data = decoded(t, res.Data)
	if res.Status != copilot.StatusOK || !strings.Contains(data["editor"].(string), "guardado no site") {
		t.Fatalf("the batch is stored even when the editor stays silent, got %+v", res)
	}
}

func TestEditsAreRefusedOnPagesThatCannotChange(t *testing.T) {
	for status, want := range map[site_usecase.Status]string{site_usecase.StatusArchived: "arquivado", site_usecase.StatusSuspended: "suspenso"} {
		r := newRig(t)
		r.storage.view.Project.Status = status
		res := NewSiteEditTool(r.deps).Execute(context.Background(), r.cc, args(t, headlineEdit))
		if res.Status != copilot.StatusError || !strings.Contains(res.Message, want) || len(r.storage.edits) != 0 {
			t.Errorf("%s: got %+v", status, res)
		}
	}
}

func TestAnEditWithoutAThreadOrBuilderAppliesNothing(t *testing.T) {
	r := newRig(t)
	r.cc.ChargeReference = ""
	if res := NewSiteEditTool(r.deps).Execute(context.Background(), r.cc, args(t, headlineEdit)); res.Status != copilot.StatusError || len(r.storage.edits) != 0 {
		t.Fatalf("got %+v", res)
	}
	r = newRig(t)
	r.storage.editErr = errors.Join(site_usecase.ErrBuilderUnavailable)
	if res := NewSiteEditTool(r.deps).Execute(context.Background(), r.cc, args(t, headlineEdit)); res.Status != copilot.StatusError || !strings.Contains(res.Message, "construtor") {
		t.Fatalf("got %+v", res)
	}
}

func TestEditValidationAndApprovalFollowTheMode(t *testing.T) {
	r := newRig(t)
	tool := NewSiteEditTool(r.deps)
	graded := tool.(copilot.Graded)
	if !graded.NeedsApproval(copilot.ModeAsk) || graded.NeedsApproval(copilot.ModeEdit) || graded.NeedsApproval(copilot.ModeFull) {
		t.Fatal("edits ask only in Perguntar")
	}
	validator := tool.(copilot.Validator)
	for name, raw := range map[string]string{
		"no operations": `{"operations":[]}`,
		"not a list":    `{"operations":"edit"}`,
		"long label":    `{"label":"` + strings.Repeat("a", 121) + `","operations":[{"op":"delete_file","path":"x"}]}`,
	} {
		if err := validator.Validate(context.Background(), r.cc, args(t, raw)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
