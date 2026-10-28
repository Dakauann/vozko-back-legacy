package sitehttp

import (
	"encoding/json"
	"net/http"
	"testing"

	"vozko/domain/site"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siteinspect"
)

func TestReadersOnlyGetCachedPreviewsAndOutlines(t *testing.T) {
	svc, inspector := newFake(), &fakeInspector{report: siteinspect.Report{Known: true}}
	viewer, _ := routerAs(svc, inspector, fakeAccess{denied: true})
	if rec := call(viewer, http.MethodPost, "/sites/projects/p-1/preview", "", nil); rec.Code != http.StatusOK || svc.cached != 1 || svc.previews != 0 {
		t.Fatalf("a reader never builds: %d %d %d", rec.Code, svc.cached, svc.previews)
	}
	svc.cachedErr = site_usecase.ErrPreviewNotBuilt
	if rec := call(viewer, http.MethodPost, "/sites/projects/p-1/preview", "", nil); rec.Code != http.StatusConflict || decodeError(t, rec).Code != "preview_not_built" {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	call(viewer, http.MethodGet, "/sites/projects/p-1/outline", "", nil)
	editor, _ := routerAs(svc, inspector, fakeAccess{})
	if rec := call(editor, http.MethodPost, "/sites/projects/p-1/preview", "", nil); rec.Code != http.StatusOK || svc.previews != 1 {
		t.Fatalf("an editor builds: %d %d", rec.Code, svc.previews)
	}
	call(editor, http.MethodGet, "/sites/projects/p-1/outline", "", nil)
}

func TestPagingRefusesWhatItCannotRead(t *testing.T) {
	r, _ := router(newFake())
	for _, query := range []string{"limit=abc", "limit=0", "limit=101", "offset=-1", "offset=x"} {
		for _, path := range []string{"/sites/projects?", "/sites/projects/p-1/revisions?"} {
			rec := call(r, http.MethodGet, path+query, "", nil)
			if rec.Code != http.StatusBadRequest || decodeError(t, rec).Code != "page_invalid" {
				t.Fatalf("%s%s: got %d %s", path, query, rec.Code, rec.Body.String())
			}
		}
	}
	if rec := call(r, http.MethodGet, "/sites/projects?limit=100&offset=30", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}

func factsView() *site_usecase.View {
	v := sampleView()
	v.Files = map[string]site.File{
		site.ManifestPath:  {Content: `{"name":"Página","routes":[{"path":"/","page":"pages/index.html"}]}`, Rev: 1},
		site.ThemePath:     {Content: "@import \"tailwindcss\";\n@theme {\n  --color-brand: #ff0066;\n  --hero-gap: 2rem;\n}\n", Rev: 1},
		site.KnobsPath:     {Content: `{"knobs":[{"name":"Espaço","variable":"--hero-gap","kind":"range","min":0,"max":4,"step":0.5,"unit":"rem"}]}`, Rev: 1},
		"pages/index.html": {Content: "<h1>Oi</h1>", Rev: 2},
	}
	v.Project.Status, v.Project.HasUnpublishedChanges, v.HasUnpublishedChanges = site_usecase.StatusPublished, true, true
	v.PrimaryURL = "https://www.pagina.com.br"
	v.Revision.Author = site_usecase.EloAuthor("thread_9")
	return v
}

func TestTheDetailCarriesWhatTheEditorUsedToDerive(t *testing.T) {
	svc := newFake()
	svc.view = factsView()
	r, _ := router(svc)
	rec := call(r, http.MethodGet, "/sites/projects/p-1", "", nil)
	var body DetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.PublishState != "published_with_changes" || body.PrimaryURL != "https://www.pagina.com.br" {
		t.Fatalf("got %s", rec.Body.String())
	}
	f := body.Facts
	if len(f.Routes) != 1 || f.Routes[0].Page != "pages/index.html" || len(f.Tokens) != 2 || f.Tokens[1].Value != "2rem" {
		t.Fatalf("got %+v", f)
	}
	if len(f.Knobs) != 1 || f.Knobs[0].Number == nil || *f.Knobs[0].Number != 2 || len(f.Breakpoints) != 4 || f.Breakpoints[3] != "xl" {
		t.Fatalf("got %+v", f)
	}
	if body.Revision.AuthorKind != "elo" || body.Revision.AuthorID != "thread_9" {
		t.Fatalf("got %+v", body.Revision)
	}
	list := call(r, http.MethodGet, "/sites/projects", "", nil)
	var items ListResponse
	_ = json.Unmarshal(list.Body.Bytes(), &items)
	if items.Items[0].PublishState != "published_with_changes" {
		t.Fatalf("got %s", list.Body.String())
	}
}

func TestEditsAndSavesAnswerTheFilesTheEditorReplaces(t *testing.T) {
	svc := newFake()
	changed := factsView()
	changed.Changes = site.FileChanges{Files: map[string]site.File{"pages/index.html": {Content: "<h1>Novo</h1>", Rev: 3}}, Deleted: []string{"pages/velha.html"}, Renamed: []site.Rename{{From: "a.html", To: "b.html"}}}
	svc.view, svc.edit = changed, &site_usecase.EditResult{View: *changed}
	r, _ := router(svc)
	rec := call(r, http.MethodPost, "/sites/projects/p-1/edits", `{"operations":[{"op":"set_text"}]}`, ifMatch)
	var edit EditResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &edit); err != nil {
		t.Fatal(err)
	}
	if edit.Changes.Files["pages/index.html"].Content != "<h1>Novo</h1>" || edit.Changes.Deleted[0] != "pages/velha.html" || edit.Changes.Renamed[0].To != "b.html" || len(edit.Facts.Tokens) != 2 {
		t.Fatalf("got %s", rec.Body.String())
	}
	rec = call(r, http.MethodPut, "/sites/projects/p-1/files", `{"files":{}}`, ifMatch)
	var saved SavedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Changes.Files["pages/index.html"].Rev != 3 || len(saved.Facts.Routes) != 1 {
		t.Fatalf("got %s", rec.Body.String())
	}
}

func TestEveryProjectAnswerCarriesThePublishState(t *testing.T) {
	svc := newFake()
	svc.view = factsView()
	svc.edit = &site_usecase.EditResult{View: *svc.view}
	r, _ := router(svc)
	answers := map[string]struct {
		method, path, body string
	}{
		"save":    {http.MethodPut, "/sites/projects/p-1/files", `{"files":{}}`},
		"edit":    {http.MethodPost, "/sites/projects/p-1/edits", `{"operations":[{"op":"set_text"}]}`},
		"version": {http.MethodPost, "/sites/projects/p-1/revisions", `{"label":"Antes"}`},
		"restore": {http.MethodPost, "/sites/projects/p-1/revisions/rev-1/restore", ``},
		"detail":  {http.MethodGet, "/sites/projects/p-1", ``},
	}
	for name, a := range answers {
		rec := call(r, a.method, a.path, a.body, ifMatch)
		var body struct {
			Project ProjectResponse `json:"project"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Project.PublishState != "published_with_changes" || !body.Project.HasUnpublishedChanges {
			t.Fatalf("%s: got %d %s", name, rec.Code, rec.Body.String())
		}
	}
	rec := call(r, http.MethodPatch, "/sites/projects/p-1", `{"name":"Outro"}`, ifMatch)
	var patched ProjectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &patched); err != nil || patched.PublishState != "published_with_changes" {
		t.Fatalf("patch: got %s", rec.Body.String())
	}
}

func TestListItemsCarryThePrimaryURL(t *testing.T) {
	svc := newFake()
	svc.view.Project.PrimaryURL = "https://www.pagina.com.br"
	r, _ := router(svc)
	rec := call(r, http.MethodGet, "/sites/projects", "", nil)
	var items ListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil || items.Items[0].PrimaryURL != "https://www.pagina.com.br" {
		t.Fatalf("got %s", rec.Body.String())
	}
}
