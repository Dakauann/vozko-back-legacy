package sitehttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	workspace_domain "vozko/domain/workspace"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siteinspect"
)

func TestTheListCarriesTheThumbnailAndTheUnpublishedFlag(t *testing.T) {
	svc := newFake()
	svc.view.Project.ThumbnailURL = "https://cdn.vozko.test/thumb.jpg"
	svc.view.Project.HasUnpublishedChanges = true
	r, _ := router(svc)
	rec := call(r, http.MethodGet, "/sites/projects", "", nil)
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Items) != 1 {
		t.Fatalf("got %s", rec.Body.String())
	}
	if body.Items[0]["thumbnailUrl"] != "https://cdn.vozko.test/thumb.jpg" || body.Items[0]["hasUnpublishedChanges"] != true {
		t.Fatalf("item %v", body.Items[0])
	}
	rec = call(r, http.MethodGet, "/sites/projects/p-1", "", nil)
	var detail map[string]map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &detail)
	if detail["project"]["thumbnailUrl"] != "https://cdn.vozko.test/thumb.jpg" {
		t.Fatalf("detail %s", rec.Body.String())
	}
}

func TestSectionsListSavedSectionsAndFilterByStack(t *testing.T) {
	svc := newFake()
	r, _ := router(svc)
	rec := call(r, http.MethodGet, "/sites/sections?stack=html", "", nil)
	var body SectionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	last := body.Sections[len(body.Sections)-1]
	if body.Sections[0].Kind != "library" || last.Key != "snippet-faixa" || last.Kind != "snippet" || last.Title.DE != "Faixa" {
		t.Fatalf("got %+v", body.Sections)
	}
	for _, s := range body.Sections {
		if s.Key == "snippet-card" {
			t.Fatal("a react snippet is not offered to an html page")
		}
	}
	if rec := call(r, http.MethodGet, "/sites/sections?stack=vue", "", nil); rec.Code != http.StatusBadRequest || decodeError(t, rec).Code != "section_stack_invalid" {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestSaveSnippetTakesTheNodeAndAnswersTheSection(t *testing.T) {
	svc := newFake()
	r, grants := router(svc)
	rec := call(r, http.MethodPost, "/sites/projects/p-1/snippets", `{"node":"pages/index.html#0","rev":2,"name":"Faixa"}`, nil)
	if rec.Code != http.StatusCreated || grants["POST /sites/projects/{id}/snippets"] != (grant{workspace_domain.ResourceSites, workspace_domain.ActionUpdate}) {
		t.Fatalf("got %d %s %+v", rec.Code, rec.Body.String(), grants)
	}
	if svc.snippetIn != (site_usecase.SaveSnippetInput{WorkspaceID: testWS, ProjectID: "p-1", Node: "pages/index.html#0", Rev: 2, Name: "Faixa"}) {
		t.Fatalf("input %+v", svc.snippetIn)
	}
	var body SnippetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Key != "snippet-faixa" || body.Kind != "snippet" || body.Title.PT != "Faixa" || len(body.Stacks) != 2 || body.DesignSystemID != "d-1" {
		t.Fatalf("got %+v", body)
	}
	svc.err = site_usecase.ErrSnippetsFull
	if rec := call(r, http.MethodPost, "/sites/projects/p-1/snippets", `{"node":"pages/index.html#0","rev":2,"name":"Faixa"}`, nil); rec.Code != http.StatusUnprocessableEntity || decodeError(t, rec).Code != "snippets_full" {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestStartersPointAtTheirPublicPreviewImage(t *testing.T) {
	r, _ := router(newFake())
	rec := call(r, http.MethodGet, "/sites/starters", "", nil)
	var body []StarterResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body) == 0 {
		t.Fatalf("got %s", rec.Body.String())
	}
	want := "https://api.vozko.test/public/sites/starters/" + body[0].Stack + "/" + body[0].Key + "/preview.jpg"
	if body[0].PreviewURL != want {
		t.Fatalf("preview %q want %q", body[0].PreviewURL, want)
	}
	public := mux.NewRouter()
	RegisterPublicRoutes(public, NewHandler(newFake(), &fakeInspector{}, testSections(), fakeAccess{}, "https://api.vozko.test"))
	img := httptest.NewRecorder()
	public.ServeHTTP(img, httptest.NewRequest(http.MethodGet, "/public/sites/starters/"+body[0].Stack+"/"+body[0].Key+"/preview.jpg", nil))
	if img.Code != http.StatusOK || img.Header().Get("Content-Type") != "image/jpeg" || img.Body.Len() == 0 || img.Header().Get("Cache-Control") == "" {
		t.Fatalf("got %d %v", img.Code, img.Header())
	}
	missing := httptest.NewRecorder()
	public.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/public/sites/starters/html/nada/preview.jpg", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("got %d", missing.Code)
	}
}

func TestOutlineSaysWhyWithAStableCode(t *testing.T) {
	inspector := &fakeInspector{report: siteinspect.Report{Reason: "texto para a Elo", ReasonCode: siteinspect.CodeRouteUnreadable, ReasonRoute: "/obrigado"}}
	r, _ := routerWith(newFake(), inspector)
	rec := call(r, http.MethodGet, "/sites/projects/p-1/outline", "", nil)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["reasonCode"] != "route_unreadable" || body["reasonRoute"] != "/obrigado" {
		t.Fatalf("got %s", rec.Body.String())
	}
	if _, ok := body["reason"]; ok {
		t.Fatal("the outline never sends display text")
	}
}
