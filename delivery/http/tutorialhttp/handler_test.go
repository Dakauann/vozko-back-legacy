package tutorialhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/domain/auth"
	"vozko/domain/tutorial"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
	tutorial_usecase "vozko/usecases/tutorial"
)

var at = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

type fakeService struct {
	progress   []tutorial.Progress
	listErr    error
	saved      tutorial_usecase.Change
	savedBy    string
	saveErr    error
	reset      string
	resetErr   error
	checklist  tutorial.Checklist
	listIn     string
	checkErr   error
	guardAsked []string
}

func (f *fakeService) Progress(_ context.Context, userID string) ([]tutorial.Progress, error) {
	if err := tutorial.RequireUser(userID); err != nil {
		return nil, err
	}
	return f.progress, f.listErr
}

func (f *fakeService) SaveProgress(_ context.Context, userID string, change tutorial_usecase.Change) (tutorial.Progress, error) {
	f.saved, f.savedBy = change, userID
	if f.saveErr != nil {
		return tutorial.Progress{}, f.saveErr
	}
	return tutorial.NewProgress(change.TutorialID, change.Status, change.Step, at)
}

func (f *fakeService) ResetProgress(_ context.Context, userID, tutorialID string) error {
	f.reset = userID + "/" + tutorialID
	return f.resetErr
}

func (f *fakeService) SitesChecklist(_ context.Context, workspaceID string) (tutorial.Checklist, error) {
	f.listIn = workspaceID
	return f.checklist, f.checkErr
}

func call(t *testing.T, svc *fakeService, method, path, body string, withClaims bool) *httptest.ResponseRecorder {
	t.Helper()
	h, err := NewHandler(svc)
	if err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()
	RegisterProtectedRoutes(router, h, func(resource workspace_domain.Resource, action workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		svc.guardAsked = append(svc.guardAsked, string(resource)+":"+string(action))
		return next
	})
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "ws-1")
	if withClaims {
		ctx = context.WithValue(ctx, middleware.ClaimsContextKey, &auth.Claims{UserID: "user-1", Role: "user"})
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

func codeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body response.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return body.Code
}

func TestTheHandlerNeedsItsService(t *testing.T) {
	if _, err := NewHandler(nil); err == nil {
		t.Fatal("a nil service must fail")
	}
}

func TestRegisteringWithoutAHandlerAddsNoRoutes(t *testing.T) {
	router := mux.NewRouter()
	RegisterProtectedRoutes(router, nil, nil)
	count := 0
	_ = router.Walk(func(*mux.Route, *mux.Router, []*mux.Route) error {
		count++
		return nil
	})
	if count != 0 {
		t.Fatalf("routes = %d, want none", count)
	}
}

func TestListingProgressAnswersEveryStoredTutorial(t *testing.T) {
	svc := &fakeService{progress: []tutorial.Progress{{TutorialID: "sites.editor", Status: tutorial.StatusCompleted, Step: 7, UpdatedAt: at}}}
	rec := call(t, svc, http.MethodGet, "/tutorials/progress", "", true)
	var body ProgressListResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if len(body.Items) != 1 || body.Items[0] != (ProgressResponse{TutorialID: "sites.editor", Status: "completed", Step: 7, UpdatedAt: at}) {
		t.Fatalf("items = %+v", body.Items)
	}
	empty := call(t, &fakeService{}, http.MethodGet, "/tutorials/progress", "", true)
	if !strings.Contains(empty.Body.String(), `"items":[]`) {
		t.Fatalf("an empty list must be an empty array: %s", empty.Body.String())
	}
}

func TestProgressNeedsALoggedPerson(t *testing.T) {
	rec := call(t, &fakeService{}, http.MethodGet, "/tutorials/progress", "", false)
	if rec.Code != http.StatusUnauthorized || codeOf(t, rec) != "unauthorized" {
		t.Fatalf("status %d code %s", rec.Code, rec.Body.String())
	}
}

func TestAnUnreadableProgressIsAFailureNotAnEmptyList(t *testing.T) {
	rec := call(t, &fakeService{listErr: errors.New("db down")}, http.MethodGet, "/tutorials/progress", "", true)
	if rec.Code != http.StatusInternalServerError || codeOf(t, rec) != codeProgressFailed {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestSavingProgress(t *testing.T) {
	svc := &fakeService{}
	rec := call(t, svc, http.MethodPut, "/tutorials/progress/sites.editor", `{"status":"dismissed","step":3}`, true)
	var body ProgressResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if svc.savedBy != "user-1" || svc.saved != (tutorial_usecase.Change{TutorialID: "sites.editor", Status: tutorial.StatusDismissed, Step: 3}) || body.Status != "dismissed" {
		t.Fatalf("saved %+v by %q, body %+v", svc.saved, svc.savedBy, body)
	}
}

func TestSavingRefusesWhatTheDomainRefuses(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{tutorial.ErrInvalidID, http.StatusBadRequest, codeIDInvalid},
		{tutorial.ErrInvalidProgress, http.StatusBadRequest, codeProgressInvalid},
		{tutorial.ErrUserRequired, http.StatusUnauthorized, "unauthorized"},
		{errors.New("db down"), http.StatusInternalServerError, codeProgressFailed},
	}
	for _, tc := range cases {
		rec := call(t, &fakeService{saveErr: tc.err}, http.MethodPut, "/tutorials/progress/sites.editor", `{"status":"completed","step":0}`, true)
		if rec.Code != tc.status || codeOf(t, rec) != tc.code {
			t.Errorf("%v: status %d body %s", tc.err, rec.Code, rec.Body.String())
		}
	}
	missing := call(t, &fakeService{}, http.MethodPut, "/tutorials/progress/sites.editor", `{"step":1}`, true)
	if missing.Code != http.StatusBadRequest || codeOf(t, missing) != codeProgressInvalid {
		t.Fatalf("a missing status: %d %s", missing.Code, missing.Body.String())
	}
}

func TestResettingProgress(t *testing.T) {
	svc := &fakeService{}
	rec := call(t, svc, http.MethodDelete, "/tutorials/progress/sites.editor", "", true)
	if rec.Code != http.StatusNoContent || svc.reset != "user-1/sites.editor" {
		t.Fatalf("status %d reset %q", rec.Code, svc.reset)
	}
	bad := call(t, &fakeService{resetErr: tutorial.ErrInvalidID}, http.MethodDelete, "/tutorials/progress/nope", "", true)
	if bad.Code != http.StatusBadRequest || codeOf(t, bad) != codeIDInvalid {
		t.Fatalf("status %d body %s", bad.Code, bad.Body.String())
	}
}

func TestTheSitesChecklistNeedsSitesReadAndCountsWhatIsDone(t *testing.T) {
	svc := &fakeService{checklist: tutorial.SitesChecklist(tutorial.SiteFacts{HasPage: true, Published: true})}
	rec := call(t, svc, http.MethodGet, "/tutorials/checklists/sites", "", true)
	var body ChecklistResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if body.Area != "sites" || body.Done != 2 || body.Total != 7 || len(body.Items) != 7 || body.Items[0] != (ChecklistItemResponse{ID: "create_page", Done: true}) || svc.listIn != "ws-1" {
		t.Fatalf("body %+v asked %q", body, svc.listIn)
	}
	guarded := false
	for _, asked := range svc.guardAsked {
		if asked == string(workspace_domain.ResourceSites)+":"+string(workspace_domain.ActionRead) {
			guarded = true
		}
	}
	if !guarded {
		t.Fatalf("guards asked %v, want sites read", svc.guardAsked)
	}
}

func TestAnUnreadableChecklistIsAFailure(t *testing.T) {
	rec := call(t, &fakeService{checkErr: errors.New("db down")}, http.MethodGet, "/tutorials/checklists/sites", "", true)
	if rec.Code != http.StatusInternalServerError || codeOf(t, rec) != codeChecklistFailed {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestRefusalCodesListEveryCode(t *testing.T) {
	want := map[string]bool{"unauthorized": true, codeIDInvalid: true, codeProgressInvalid: true, codeProgressFailed: true, codeChecklistFailed: true}
	codes := RefusalCodes()
	if len(codes) != len(want) {
		t.Fatalf("codes = %v", codes)
	}
	for _, code := range codes {
		if !want[code] {
			t.Errorf("unexpected code %q", code)
		}
	}
}
