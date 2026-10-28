package sitetools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"vozko/domain/copilot"
	site_usecase "vozko/usecases/site"
)

func TestTheMapListsFilesRoutesFormsAndGapsWithoutFileText(t *testing.T) {
	r := newRig(t)
	r.editor.pages = allRoutes(gappedHome)
	res := NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, map[string]interface{}{})
	if res.Status != copilot.StatusOK {
		t.Fatalf("got %+v", res)
	}
	raw, _ := json.Marshal(res.Data)
	text := string(raw)
	for _, want := range []string{
		`src/routes/index.jsx 6.6k r1; exporta Home; seções beneficios, depoimentos, perguntas, 1 sem id`,
		`src/components/LeadForm.jsx`, `form contato`,
		`"/ src/routes/index.jsx"`,
		`contato (src/components/LeadForm.jsx): sucesso redirect, avisa a equipe`,
		`"screen_route":"/"`, `h1 src/routes/index.jsx#120`,
		`alt_missing em / (src/routes/index.jsx#300)`, `link_empty em / (src/routes/index.jsx#400)`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the map lacks %q in %s", want, text)
		}
	}
	if strings.Contains(text, "Agende sua avaliação gratuita") || strings.Contains(text, "className") {
		t.Fatal("the map never carries file text")
	}
	if !strings.HasSuffix(text, "]}") || strings.LastIndex(text, `"open_gaps"`) < strings.LastIndex(text, `"files"`) {
		t.Fatalf("open_gaps closes the result, got %s", text)
	}
}

func TestTheMapSaysWhenTheEditorHoldsUnsavedFiles(t *testing.T) {
	r := newRig(t)
	data := decoded(t, NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, map[string]interface{}{}).Data)
	unsaved, _ := data["unsaved_in_editor"].([]interface{})
	if len(unsaved) == 0 || unsaved[0] != "knobs.json" {
		t.Fatalf("files the editor did not report are unsaved there, got %v", data["unsaved_in_editor"])
	}
}

func TestAPathReturnsThatFileByLines(t *testing.T) {
	r := newRig(t)
	res := NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"path":"src/App.jsx","from":2,"to":3}`))
	data := decoded(t, res.Data)
	if res.Status != copilot.StatusOK || data["lines"] != "2-3 de 11" || !strings.HasPrefix(data["text"].(string), `import Obrigado`) || strings.Count(data["text"].(string), "\n") != 2 {
		t.Fatalf("got %+v", data)
	}
	if _, gaps := data["open_gaps"]; gaps {
		t.Fatal("a file read stays a file read")
	}
	for name, raw := range map[string]string{
		"missing": `{"path":"src/Nada.jsx"}`,
		"reverse": `{"path":"src/App.jsx","from":5,"to":2}`,
		"past":    `{"path":"src/App.jsx","from":90}`,
	} {
		if res := NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, args(t, raw)); res.Status != copilot.StatusError {
			t.Errorf("%s: want an error, got %+v", name, res)
		}
	}
}

func TestOpenGapsAreReportedUnreadWhenThePreviewFails(t *testing.T) {
	r := newRig(t)
	r.storage.previewErr = &site_usecase.BuildFailedError{Errors: []site_usecase.BuildIssue{{Path: "src/App.jsx", Line: 4, Message: "Unexpected token"}, {Path: "src/App.jsx", Line: 4, Message: "Unexpected token"}}}
	raw, _ := json.Marshal(NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, map[string]interface{}{}).Data)
	text := string(raw)
	if strings.Contains(text, `"open_gaps":`) || !strings.Contains(text, `"open_gaps_unread":"a página não compila`) || strings.Count(text, "Unexpected token") != 1 {
		t.Fatalf("an unknown state is never reported as no gaps, got %s", text)
	}
}

func TestSiteToolsRefuseOutsideTheSiteOrWithoutTheEditor(t *testing.T) {
	r := newRig(t)
	away := r.cc
	away.View = copilot.View{Surface: copilot.SurfaceEmailDesign, ProjectID: projectID}
	detached := r.cc
	detached.Screen = nil
	other := r.cc
	other.WorkspaceID = "ws-2"
	for name, cc := range map[string]copilot.Context{"email": away, "no editor": detached, "another workspace": other} {
		if res := NewSiteReadTool(r.deps).Execute(context.Background(), cc, map[string]interface{}{}); res.Status != copilot.StatusError {
			t.Errorf("%s: want an error, got %+v", name, res)
		}
	}
}

func TestGrepFindsLiteralLinesAcrossFiles(t *testing.T) {
	r := newRig(t)
	res := NewSiteGrepTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"text":"data-vz-cta","files":"src/**/*.jsx"}`))
	data := decoded(t, res.Data)
	matches := data["matches"].([]interface{})
	if res.Status != copilot.StatusOK || len(matches) != 5 || !strings.HasPrefix(matches[0].(string), "src/components/Footer.jsx:11: ") {
		t.Fatalf("got %+v", data)
	}
	none := decoded(t, NewSiteGrepTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"text":"DATA-VZ-CTA","match_case":true}`)).Data)
	if len(none["matches"].([]interface{})) != 0 {
		t.Fatalf("match_case is honored, got %+v", none)
	}
}

type fakeReferences struct {
	catalog site_usecase.ReferenceCatalog
	err     error
	asked   []string
}

func (f *fakeReferences) Catalog(_ context.Context, workspaceID string) (site_usecase.ReferenceCatalog, error) {
	f.asked = append(f.asked, workspaceID)
	return f.catalog, f.err
}

func TestTheMapCarriesTheWorkspaceReferencesOnlyWhenAsked(t *testing.T) {
	r := newRig(t)
	refs := &fakeReferences{catalog: site_usecase.ReferenceCatalog{WhatsAppNumbers: []site_usecase.ReferenceItem{{ID: "phone-1", Name: "Clínica (+55 11 90000-0000)"}}}}
	r.deps.References = refs
	plain := decoded(t, NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, map[string]interface{}{}).Data)
	if _, ok := plain["references"]; ok || len(refs.asked) != 0 {
		t.Fatal("the map stays small unless the references are asked for")
	}
	raw, _ := json.Marshal(NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"references":true}`)).Data)
	if !strings.Contains(string(raw), `"whatsapp_numbers":[{"id":"phone-1","name":"Clínica (+55 11 90000-0000)"}]`) || len(refs.asked) != 1 {
		t.Fatalf("references missing in %s", raw)
	}
	refs.err = context.DeadlineExceeded
	raw, _ = json.Marshal(NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"references":true}`)).Data)
	if !strings.Contains(string(raw), readNoReferences) {
		t.Fatalf("an unreadable catalog says so: %s", raw)
	}
}
