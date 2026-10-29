package sitehttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/domain/auth"
	"vozko/domain/shared"
	"vozko/domain/site"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
	"vozko/pkg/codegolden"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
)

const (
	testWS   = "ws-1"
	testUser = "u-1"
)

type fakeService struct {
	err         error
	view        *site_usecase.View
	settings    *site_usecase.StoredSettings
	design      *site_usecase.DesignSystemRecord
	edit        *site_usecase.EditResult
	saveInput   site_usecase.SaveFilesInput
	editInput   site_usecase.EditInput
	createIn    site_usecase.CreateInput
	duplicateIn site_usecase.DuplicateInput
	settingsIn  []byte
	gets        int
	snippetIn   site_usecase.SaveSnippetInput
	previews    int
	cached      int
	cachedErr   error
}

func (f *fakeService) CachedPreview(context.Context, string, string) (site_usecase.PreviewResult, error) {
	f.cached++
	if f.cachedErr != nil {
		return site_usecase.PreviewResult{}, f.cachedErr
	}
	return site_usecase.PreviewResult{URL: "https://t.preview.example/", RevisionID: "r-1"}, nil
}

type fakeAccess struct {
	denied bool
}

func (f fakeAccess) Execute(userID, workspaceID string, resource workspace_domain.Resource, action workspace_domain.Action) error {
	if f.denied || userID != testUser || workspaceID != testWS || resource != workspace_domain.ResourceSites || action != workspace_domain.ActionUpdate {
		return errors.New("denied")
	}
	return nil
}

func (f *fakeService) SaveSnippet(_ context.Context, in site_usecase.SaveSnippetInput) (*site_usecase.SavedSnippet, error) {
	f.snippetIn = in
	if f.err != nil {
		return nil, f.err
	}
	return &site_usecase.SavedSnippet{DesignSystem: *f.design, Snippet: site.DesignSnippet{Key: "snippet-faixa", Name: in.Name, Stack: site.StackHTML, Content: "<section />"}}, nil
}

type snippetSource []site.DesignSnippet

func (s snippetSource) Snippets(context.Context, string) ([]site.DesignSnippet, error) { return s, nil }

func testSections() *sitebuild.WorkspaceSections {
	lib, err := sitebuild.Sections()
	if err != nil {
		panic(err)
	}
	return sitebuild.NewWorkspaceSections(lib, snippetSource{
		{Key: "snippet-faixa", Name: "Faixa", Stack: site.StackHTML, Content: "<section />"},
		{Key: "snippet-card", Name: "Card", Stack: site.StackReact, Content: "<div />"},
	})
}

func sampleView() *site_usecase.View {
	return &site_usecase.View{
		Project:  site_usecase.Project{ID: "p-1", WorkspaceID: testWS, Name: "Página", Slug: "pagina", Stack: site.StackHTML, Status: site_usecase.StatusDraft, WorkingRevisionID: "r-1", Version: 3},
		Files:    map[string]site.File{"pages/index.html": {Content: "<h1>Oi</h1>", Rev: 2}},
		Settings: &site_usecase.StoredSettings{Version: 1},
		Revision: site_usecase.Revision{ID: "r-1", Kind: site_usecase.RevisionAutosave, Author: "person:u-1"},
	}
}

func (f *fakeService) Duplicate(_ context.Context, in site_usecase.DuplicateInput) (*site_usecase.View, error) {
	f.duplicateIn = in
	return f.view, f.err
}
func (f *fakeService) Create(_ context.Context, in site_usecase.CreateInput) (*site_usecase.View, error) {
	f.createIn = in
	return f.view, f.err
}
func (f *fakeService) List(context.Context, site_usecase.ListQuery) ([]site_usecase.Project, int64, error) {
	return []site_usecase.Project{f.view.Project}, 1, f.err
}
func (f *fakeService) Get(context.Context, string, string) (*site_usecase.View, error) {
	f.gets++
	return f.view, nil
}
func (f *fakeService) Update(context.Context, string, string, int64, site_usecase.Change) (*site_usecase.Project, error) {
	return &f.view.Project, f.err
}
func (f *fakeService) Delete(context.Context, string, string) error { return f.err }
func (f *fakeService) SaveFiles(_ context.Context, in site_usecase.SaveFilesInput) (*site_usecase.View, error) {
	f.saveInput = in
	return f.view, f.err
}
func (f *fakeService) Edit(_ context.Context, in site_usecase.EditInput) (*site_usecase.EditResult, error) {
	f.editInput = in
	return f.edit, f.err
}
func (f *fakeService) ListRevisions(context.Context, string, string, int, int) ([]site_usecase.Revision, int64, error) {
	return []site_usecase.Revision{f.view.Revision}, 1, f.err
}
func (f *fakeService) SaveVersion(context.Context, site_usecase.SaveVersionInput) (*site_usecase.View, error) {
	return f.view, f.err
}
func (f *fakeService) Restore(context.Context, site_usecase.RestoreInput) (*site_usecase.View, error) {
	return f.view, f.err
}
func (f *fakeService) GetSettings(context.Context, string, string) (*site_usecase.StoredSettings, error) {
	return f.settings, nil
}
func (f *fakeService) PutSettings(_ context.Context, _, _ string, _ int64, raw []byte) (*site_usecase.StoredSettings, error) {
	f.settingsIn = raw
	return f.settings, f.err
}
func (f *fakeService) Preview(context.Context, string, string) (site_usecase.PreviewResult, error) {
	f.previews++
	return site_usecase.PreviewResult{URL: "https://t.preview.example/", RevisionID: "r-1"}, f.err
}
func (f *fakeService) Export(context.Context, string, string) (string, []byte, error) {
	return "pagina.zip", []byte("PK"), f.err
}
func (f *fakeService) CreateDesignSystem(context.Context, string, site.DesignSystem, bool) (*site_usecase.DesignSystemRecord, error) {
	return f.design, f.err
}
func (f *fakeService) ListDesignSystems(context.Context, string) ([]site_usecase.DesignSystemRecord, error) {
	return []site_usecase.DesignSystemRecord{*f.design}, f.err
}
func (f *fakeService) GetDesignSystem(context.Context, string, string) (*site_usecase.DesignSystemRecord, error) {
	return f.design, nil
}
func (f *fakeService) UpdateDesignSystem(context.Context, string, string, int64, site.DesignSystem, bool) (*site_usecase.DesignSystemRecord, error) {
	return f.design, f.err
}
func (f *fakeService) DeleteDesignSystem(context.Context, string, string) error { return f.err }

type grant struct {
	resource workspace_domain.Resource
	action   workspace_domain.Action
}

func router(svc *fakeService) (*mux.Router, map[string]grant) {
	return routerWith(svc, &fakeInspector{})
}

func routerWith(svc *fakeService, inspector Outliner) (*mux.Router, map[string]grant) {
	return routerAs(svc, inspector, fakeAccess{})
}

func routerAs(svc *fakeService, inspector Outliner, access Access) (*mux.Router, map[string]grant) {
	r := mux.NewRouter()
	grants := map[string]grant{}
	ac := func(res workspace_domain.Resource, action workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			tmpl, _ := mux.CurrentRoute(req).GetPathTemplate()
			grants[req.Method+" "+tmpl] = grant{res, action}
			next(w, req)
		}
	}
	RegisterProtectedRoutes(r, NewHandler(svc, inspector, testSections(), access, "https://api.vozko.test"), ac)
	return r, grants
}

func call(r *mux.Router, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.ClaimsContextKey, &auth.Claims{UserID: testUser})
	ctx = context.WithValue(ctx, middleware.WorkspaceIDContextKey, testWS)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

var ifMatch = map[string]string{"If-Match": `"3"`}

type errorBody struct {
	Code    string          `json:"code"`
	Details json.RawMessage `json:"details"`
	Current json.RawMessage `json:"current"`
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %s: %v", rec.Body.String(), err)
	}
	return body
}

func newFake() *fakeService {
	return &fakeService{
		view: sampleView(), settings: &site_usecase.StoredSettings{Version: 2},
		design: &site_usecase.DesignSystemRecord{ID: "d-1", System: site.DesignSystem{Name: "Marca"}, Version: 1},
		edit:   &site_usecase.EditResult{View: *sampleView()},
	}
}

func TestEveryRouteIsGuardedByTheSitesPermission(t *testing.T) {
	svc := newFake()
	r, grants := router(svc)
	cases := map[string]grant{
		"GET /sites/starters":                            {workspace_domain.ResourceSites, workspace_domain.ActionRead},
		"GET /sites/sections":                            {workspace_domain.ResourceSites, workspace_domain.ActionRead},
		"GET /sites/projects":                            {workspace_domain.ResourceSites, workspace_domain.ActionRead},
		"POST /sites/projects":                           {workspace_domain.ResourceSites, workspace_domain.ActionCreate},
		"GET /sites/projects/p-1":                        {workspace_domain.ResourceSites, workspace_domain.ActionRead},
		"PATCH /sites/projects/p-1":                      {workspace_domain.ResourceSites, workspace_domain.ActionUpdate},
		"DELETE /sites/projects/p-1":                     {workspace_domain.ResourceSites, workspace_domain.ActionDelete},
		"GET /sites/projects/p-1/files":                  {workspace_domain.ResourceSites, workspace_domain.ActionRead},
		"PUT /sites/projects/p-1/files":                  {workspace_domain.ResourceSites, workspace_domain.ActionUpdate},
		"POST /sites/projects/p-1/edits":                 {workspace_domain.ResourceSites, workspace_domain.ActionUpdate},
		"GET /sites/projects/p-1/revisions":              {workspace_domain.ResourceSites, workspace_domain.ActionRead},
		"POST /sites/projects/p-1/revisions":             {workspace_domain.ResourceSites, workspace_domain.ActionUpdate},
		"POST /sites/projects/p-1/revisions/r-1/restore": {workspace_domain.ResourceSites, workspace_domain.ActionUpdate},
		"GET /sites/projects/p-1/settings":               {workspace_domain.ResourceSites, workspace_domain.ActionRead},
		"PUT /sites/projects/p-1/settings":               {workspace_domain.ResourceSites, workspace_domain.ActionUpdate},
		"POST /sites/projects/p-1/preview":               {workspace_domain.ResourceSites, workspace_domain.ActionRead},
		"GET /sites/projects/p-1/export":                 {workspace_domain.ResourceSites, workspace_domain.ActionRead},
		"GET /sites/design-systems":                      {workspace_domain.ResourceSites, workspace_domain.ActionRead},
		"POST /sites/design-systems":                     {workspace_domain.ResourceSites, workspace_domain.ActionCreate},
		"GET /sites/design-systems/d-1":                  {workspace_domain.ResourceSites, workspace_domain.ActionRead},
		"PUT /sites/design-systems/d-1":                  {workspace_domain.ResourceSites, workspace_domain.ActionUpdate},
		"DELETE /sites/design-systems/d-1":               {workspace_domain.ResourceSites, workspace_domain.ActionDelete},
		"POST /sites/projects/p-1/snippets":              {workspace_domain.ResourceSites, workspace_domain.ActionUpdate},
	}
	for route, want := range cases {
		method, path, _ := strings.Cut(route, " ")
		call(r, method, path, "{}", ifMatch)
		tmpl := strings.NewReplacer("/p-1", "/{id}", "/d-1", "/{id}", "/r-1", "/{rev}").Replace(path)
		if got := grants[method+" "+tmpl]; got != want {
			t.Errorf("%s guarded by %+v, want %+v", route, got, want)
		}
	}
}

func TestCreateUsesThePersonAndAnswersTheWholePage(t *testing.T) {
	svc := newFake()
	r, _ := router(svc)
	rec := call(r, http.MethodPost, "/sites/projects", `{"name":"Página","stack":"html","starter":"blank","origin":{"threadId":"t","commandId":"c"}}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if svc.createIn.UserID != testUser || svc.createIn.WorkspaceID != testWS || svc.createIn.Stack != site.StackHTML || svc.createIn.Origin.ThreadID != "t" {
		t.Fatalf("input %+v", svc.createIn)
	}
	var body DetailResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Project.Version != 3 || body.Files["pages/index.html"].Rev != 2 || body.Revision.Kind != "autosave" {
		t.Fatalf("body %s", rec.Body.String())
	}
	if rec := call(r, http.MethodPost, "/sites/projects", `{"name":"x","theme":"dark"}`, nil); rec.Code != http.StatusBadRequest || decodeError(t, rec).Code != "invalid_body" {
		t.Fatalf("strict decoding: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSavesNeedIfMatchAndAnswerConflictsWithTheCurrentPage(t *testing.T) {
	svc := newFake()
	r, _ := router(svc)
	if rec := call(r, http.MethodPut, "/sites/projects/p-1/files", `{"files":{}}`, nil); rec.Code != http.StatusPreconditionRequired || decodeError(t, rec).Code != "version_required" {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	rec := call(r, http.MethodPut, "/sites/projects/p-1/files", `{"files":{"pages/index.html":"<h1>x</h1>"},"deleted":["pages/a.html"]}`, ifMatch)
	if rec.Code != http.StatusOK || svc.saveInput.ExpectedVersion != 3 || svc.saveInput.Author != "person:u-1" || svc.saveInput.Deleted[0] != "pages/a.html" {
		t.Fatalf("got %d %+v", rec.Code, svc.saveInput)
	}
	var saved SavedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &saved)
	if saved.Revs["pages/index.html"] != 2 {
		t.Fatalf("body %s", rec.Body.String())
	}
	svc.err = shared.ErrVersionConflict
	rec = call(r, http.MethodPut, "/sites/projects/p-1/files", `{"files":{}}`, ifMatch)
	body := decodeError(t, rec)
	if rec.Code != http.StatusConflict || body.Code != "version_conflict" || !strings.Contains(string(body.Current), `"version":3`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestRefusalsCarryTheirCodeAndDetails(t *testing.T) {
	svc := newFake()
	r, _ := router(svc)
	svc.err = &site_usecase.IssuesError{Issues: []site.Issue{{Code: site.CodeCSSDirectiveForbidden, Path: "theme.css", Detail: "@plugin"}}}
	rec := call(r, http.MethodPut, "/sites/projects/p-1/files", `{"files":{}}`, ifMatch)
	if body := decodeError(t, rec); rec.Code != http.StatusUnprocessableEntity || body.Code != "css_directive_forbidden" || !strings.Contains(string(body.Details), "theme.css") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	svc.err = &site_usecase.EditRefusal{Failures: []site.Failure{{Index: 0, Code: site.CodeNodeStale}}, Skipped: []site.Skip{{Index: 1, After: 0}}}
	rec = call(r, http.MethodPost, "/sites/projects/p-1/edits", `{"operations":[{"op":"set_text"}]}`, ifMatch)
	if body := decodeError(t, rec); rec.Code != http.StatusUnprocessableEntity || body.Code != "edit_refused" || !strings.Contains(string(body.Details), "node_stale") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if string(svc.editInput.Batch) != `[{"op":"set_text"}]` || svc.editInput.Author != "person:u-1" {
		t.Fatalf("edit input %+v", svc.editInput)
	}
	svc.err = &site_usecase.BuildFailedError{Errors: []site_usecase.BuildIssue{{Path: "pages/index.html", Line: 3, Message: "Unexpected token"}}}
	rec = call(r, http.MethodPost, "/sites/projects/p-1/preview", "", nil)
	if body := decodeError(t, rec); rec.Code != http.StatusUnprocessableEntity || body.Code != "build_failed" || !strings.Contains(string(body.Details), "Unexpected token") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	svc.err = &site_usecase.BuildFailedError{Errors: []site_usecase.BuildIssue{{Path: "src/App.jsx", Line: 2, Message: "a dynamic import must name a fixed file or library", Code: site.CodeImportDynamicForbidden}}}
	rec = call(r, http.MethodPost, "/sites/projects/p-1/preview", "", nil)
	if body := decodeError(t, rec); rec.Code != http.StatusUnprocessableEntity || body.Code != "build_failed" || !strings.Contains(string(body.Details), `"code":"import_dynamic_forbidden"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	svc.err = &site_usecase.FormReferenceError{Form: "contato", Err: site_usecase.ErrFormWorkflowUnknown}
	rec = call(r, http.MethodPut, "/sites/projects/p-1/settings", `{}`, ifMatch)
	if body := decodeError(t, rec); rec.Code != http.StatusUnprocessableEntity || body.Code != "form_workflow_unknown" || !strings.Contains(string(body.Details), "contato") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	for err, want := range map[error]string{
		site_usecase.ErrProjectNotFound:                                   "project_not_found",
		site_usecase.ErrProjectPublished:                                  "project_published",
		site_usecase.ErrBuilderUnavailable:                                "build_unavailable",
		site.ErrBatchMalformed:                                            "batch_malformed",
		errors.Join(site.ErrSettingsInvalid, site.ErrFormChannelRequired): "form_channel_required",
		errors.New("boom"):                                                "internal_error",
	} {
		svc.err = err
		rec := call(r, http.MethodPost, "/sites/projects/p-1/edits", `{"operations":[]}`, ifMatch)
		if got := decodeError(t, rec).Code; got != want {
			t.Errorf("%v: got %s (%d)", err, got, rec.Code)
		}
	}
}

func TestSettingsTakeTheRawDocumentAndAnswerConflictsWithTheCurrentSettings(t *testing.T) {
	svc := newFake()
	r, _ := router(svc)
	rec := call(r, http.MethodPut, "/sites/projects/p-1/settings", `{"consent":{"banner":true}}`, ifMatch)
	if rec.Code != http.StatusOK || string(svc.settingsIn) != `{"consent":{"banner":true}}` {
		t.Fatalf("got %d %s", rec.Code, svc.settingsIn)
	}
	svc.err = shared.ErrVersionConflict
	rec = call(r, http.MethodPut, "/sites/projects/p-1/settings", `{}`, ifMatch)
	if body := decodeError(t, rec); rec.Code != http.StatusConflict || !strings.Contains(string(body.Current), `"version":2`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	svc.err = nil
	rec = call(r, http.MethodPut, "/sites/projects/p-1/settings", `{"x":"`+strings.Repeat("a", settingsBodyBytes)+`"}`, ifMatch)
	if rec.Code != http.StatusRequestEntityTooLarge || decodeError(t, rec).Code != "project_too_large" {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestExportSendsAZipAttachment(t *testing.T) {
	r, _ := router(newFake())
	rec := call(r, http.MethodGet, "/sites/projects/p-1/export", "", nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/zip" || !strings.Contains(rec.Header().Get("Content-Disposition"), "pagina.zip") {
		t.Fatalf("got %d %v", rec.Code, rec.Header())
	}
}

func TestSitesErrorCodesMatchTheGoldenSoTheFrontTranslatesThem(t *testing.T) {
	codes := response.RefusalCodes(response.ErrorRefusalsOf(refusals), response.ErrorRefusalsOf(formRefusals), response.ErrorRefusalsOf(captureRefusals), response.ErrorRefusalsOf(nextStepsRefusals), requestRefusals)
	codes = append(codes, detailCodes()...)
	codegolden.Compare(t, filepath.Join("testdata", "sites_error_codes.golden"), codes,
		"update testdata/sites_error_codes.golden, and translate sites.errors.<code> in the pt, en, es and de catalogs of vozko-front")
}
