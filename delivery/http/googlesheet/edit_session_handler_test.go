package googlesheet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"vozko/domain/auth"
	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/infra/http/middleware"
	gsuc "vozko/usecases/googlesheet"
)

type stubEdits struct {
	start   gsuc.StartEditInput
	beat    gsuc.EditInput
	end     gsuc.EndEditInput
	file    string
	view    *gsuc.EditView
	editor  *gsuc.EditorView
	history []gsuc.EditView
	err     error
}

func (s *stubEdits) Editor(_ context.Context, workspaceID, spreadsheetID string) (*gsuc.EditorView, error) {
	s.file = workspaceID + "/" + spreadsheetID
	return s.editor, s.err
}

func (s *stubEdits) Start(_ context.Context, in gsuc.StartEditInput) (*gsuc.EditView, error) {
	s.start = in
	return s.view, s.err
}

func (s *stubEdits) Heartbeat(_ context.Context, in gsuc.EditInput) (*gsuc.EditView, error) {
	s.beat = in
	return s.view, s.err
}

func (s *stubEdits) End(_ context.Context, in gsuc.EndEditInput) (*gsuc.EditView, error) {
	s.end = in
	return s.view, s.err
}

func (s *stubEdits) History(_ context.Context, workspaceID, spreadsheetID string, p shared.Pagination) (*shared.PaginatedResult[gsuc.EditView], error) {
	s.file = workspaceID + "/" + spreadsheetID
	if s.err != nil {
		return nil, s.err
	}
	return &shared.PaginatedResult[gsuc.EditView]{Items: s.history, Page: p.Page, PageSize: p.PageSize, TotalItems: int64(len(s.history)), TotalPages: 1}, nil
}

func serveEdits(h *EditSessionHandler, method, path, body string) *httptest.ResponseRecorder {
	router := mux.NewRouter()
	router.HandleFunc("/google-sheets/files/{spreadsheetId}/edit-session", h.Editor).Methods(http.MethodGet)
	router.HandleFunc("/google-sheets/files/{spreadsheetId}/edit-session", h.Start).Methods(http.MethodPost)
	router.HandleFunc("/google-sheets/files/{spreadsheetId}/edit-sessions", h.History).Methods(http.MethodGet)
	router.HandleFunc("/google-sheets/edit-sessions/{sessionId}/heartbeat", h.Heartbeat).Methods(http.MethodPost)
	router.HandleFunc("/google-sheets/edit-sessions/{sessionId}/end", h.End).Methods(http.MethodPost)
	router.HandleFunc("/google-sheets/edit-sessions/{sessionId}/force-end", h.ForceEnd).Methods(http.MethodPost)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "ws-1")
	ctx = context.WithValue(ctx, middleware.ClaimsContextKey, &auth.Claims{UserID: "user-1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

var handlerEditStart = time.Date(2026, 10, 27, 14, 2, 0, 0, time.UTC)

func heldView(holder string) *gsuc.EditView {
	session, _ := googlesheet.NewEditSession("ws-1", "sp-1", holder, handlerEditStart)
	session.ID = "es-1"
	session.Settle(handlerEditStart)
	return &gsuc.EditView{Session: session, StartedBy: &gsuc.NamedRef{ID: holder, Name: "Ana Souza"}}
}

type sessionBody struct {
	Session struct {
		ID         string    `json:"id"`
		Mine       bool      `json:"mine"`
		Settled    bool      `json:"settled"`
		StartedAt  time.Time `json:"startedAt"`
		ExpiresAt  time.Time `json:"expiresAt"`
		Deadline   time.Time `json:"deadline"`
		EndReason  string    `json:"endReason"`
		WaitedRows int       `json:"waitedRows"`
		StartedBy  struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"startedBy"`
		Check *struct {
			Status string `json:"status"`
			Tabs   []struct {
				SheetID     string `json:"sheetId"`
				Reason      string `json:"reason"`
				Repairable  bool   `json:"repairable"`
				RenamedFrom string `json:"renamedFrom"`
				RefWarnings []int  `json:"refWarnings"`
			} `json:"tabs"`
		} `json:"check"`
	} `json:"session"`
}

func TestStartingAnEditIsForThePersonAndTheFileInThePath(t *testing.T) {
	stub := &stubEdits{view: heldView("user-1")}
	rec := serveEdits(NewEditSessionHandler(stub), http.MethodPost, "/google-sheets/files/sp-1/edit-session", "")
	if rec.Code != http.StatusOK || stub.start != (gsuc.StartEditInput{WorkspaceID: "ws-1", SpreadsheetID: "sp-1", UserID: "user-1"}) {
		t.Fatalf("status %d input %+v: %s", rec.Code, stub.start, rec.Body.String())
	}
	var body sessionBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Session.ID != "es-1" || !body.Session.Mine || !body.Session.Settled || body.Session.StartedBy.Name != "Ana Souza" ||
		!body.Session.Deadline.Equal(handlerEditStart.Add(googlesheet.EditSessionMaxLength)) || body.Session.Check != nil {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestAnotherPersonsSessionIsNotMine(t *testing.T) {
	stub := &stubEdits{view: heldView("user-2")}
	rec := serveEdits(NewEditSessionHandler(stub), http.MethodPost, "/google-sheets/edit-sessions/es-1/heartbeat", "")
	var body sessionBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK || body.Session.Mine {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if stub.beat != (gsuc.EditInput{WorkspaceID: "ws-1", SessionID: "es-1", UserID: "user-1"}) {
		t.Fatalf("beat = %+v", stub.beat)
	}
}

func TestTheFileBeingEditedNamesTheHolderAndSince(t *testing.T) {
	session, _ := googlesheet.NewEditSession("ws-1", "sp-1", "user-2", handlerEditStart)
	stub := &stubEdits{err: &googlesheet.EditTakenError{Session: session, HolderName: "Bruno Lima"}}
	rec := serveEdits(NewEditSessionHandler(stub), http.MethodPost, "/google-sheets/files/sp-1/edit-session", "")
	var body struct {
		Code    string `json:"code"`
		Details struct {
			EditedBy     string    `json:"editedBy"`
			EditedByName string    `json:"editedByName"`
			Since        time.Time `json:"since"`
		} `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusConflict || body.Code != googlesheet.CodeFileBeingEdited ||
		body.Details.EditedBy != "user-2" || body.Details.EditedByName != "Bruno Lima" || !body.Details.Since.Equal(handlerEditStart) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestEndingTakesOnlyFinishedOrLeftFromTheClientAndForceEndIsTheAdminEnd(t *testing.T) {
	view := heldView("user-1")
	end := handlerEditStart.Add(time.Minute)
	_ = view.Session.End(googlesheet.EditEnd{Reason: googlesheet.EditFinished, By: "user-1", At: end})
	view.Session.WaitedRows = 3
	view.Session.Check = googlesheet.NewEditCheck([]googlesheet.TabCheck{
		{SheetID: "s1", TabID: 0, TabName: "Pagamentos", Reason: googlesheet.ReasonHeaderMismatch, Detail: "coluna C"},
		{SheetID: "s2", TabID: 4, TabName: "Pagos", RenamedFrom: "Negociações", RefWarnings: []int{7}},
	})
	stub := &stubEdits{view: view}
	rec := serveEdits(NewEditSessionHandler(stub), http.MethodPost, "/google-sheets/edit-sessions/es-1/end", `{"reason":"left"}`)
	if rec.Code != http.StatusOK || stub.end != (gsuc.EndEditInput{WorkspaceID: "ws-1", SessionID: "es-1", UserID: "user-1", Reason: googlesheet.EditLeft}) {
		t.Fatalf("status %d end %+v: %s", rec.Code, stub.end, rec.Body.String())
	}
	var body sessionBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Session.EndReason != "finished" || body.Session.WaitedRows != 3 || body.Session.Check == nil ||
		body.Session.Check.Status != "broken" || !body.Session.Check.Tabs[0].Repairable || body.Session.Check.Tabs[1].RenamedFrom != "Negociações" || body.Session.Check.Tabs[1].RefWarnings[0] != 7 {
		t.Fatalf("body = %s", rec.Body.String())
	}
	for _, bad := range []string{`{"reason":"expired"}`, `{"reason":"ended_by_admin"}`, `{}`} {
		rec := serveEdits(NewEditSessionHandler(&stubEdits{view: view}), http.MethodPost, "/google-sheets/edit-sessions/es-1/end", bad)
		if code, _ := codeOf(t, rec); rec.Code != http.StatusBadRequest || code != googlesheet.CodeEditEndReasonUnknown {
			t.Errorf("%s: status %d code %s", bad, rec.Code, code)
		}
	}
	force := &stubEdits{view: view}
	serveEdits(NewEditSessionHandler(force), http.MethodPost, "/google-sheets/edit-sessions/es-1/force-end", "")
	if force.end.Reason != googlesheet.EditEndedByAdmin || force.end.UserID != "user-1" {
		t.Fatalf("force end = %+v", force.end)
	}
}

func TestEditRefusalsKeepTheirCodes(t *testing.T) {
	cases := map[error]struct {
		status int
		code   string
	}{
		googlesheet.ErrEditSessionNotFound: {http.StatusNotFound, googlesheet.CodeEditSessionNotFound},
		googlesheet.ErrEditSessionEnded:    {http.StatusConflict, googlesheet.CodeEditSessionEnded},
		googlesheet.ErrEditSessionNotYours: {http.StatusForbidden, googlesheet.CodeEditSessionNotYours},
		googlesheet.ErrFileBeingEdited:     {http.StatusConflict, googlesheet.CodeFileBeingEdited},
		googlesheet.ErrSpreadsheetNotFound: {http.StatusNotFound, googlesheet.CodeSpreadsheetNotFound},
	}
	for err, want := range cases {
		rec := serveEdits(NewEditSessionHandler(&stubEdits{err: err}), http.MethodPost, "/google-sheets/edit-sessions/es-1/heartbeat", "")
		if code, _ := codeOf(t, rec); rec.Code != want.status || code != want.code {
			t.Errorf("%v: status %d code %s, want %d %s", err, rec.Code, code, want.status, want.code)
		}
	}
}

func TestTheEditorAnswersTheHolderTheAccountAndTheTimings(t *testing.T) {
	stub := &stubEdits{editor: &gsuc.EditorView{Editing: heldView("user-2"), AccountEmail: "ops@example.com"}}
	rec := serveEdits(NewEditSessionHandler(stub), http.MethodGet, "/google-sheets/files/sp-1/edit-session", "")
	var body struct {
		Editing          *struct{ ID string } `json:"editing"`
		AccountEmail     string               `json:"accountEmail"`
		HeartbeatSeconds int                  `json:"heartbeatSeconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK || stub.file != "ws-1/sp-1" || body.Editing == nil || body.Editing.ID != "es-1" ||
		body.AccountEmail != "ops@example.com" || body.HeartbeatSeconds != 30 {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	free := serveEdits(NewEditSessionHandler(&stubEdits{editor: &gsuc.EditorView{}}), http.MethodGet, "/google-sheets/files/sp-1/edit-session", "")
	if !strings.Contains(free.Body.String(), `"editing":null`) {
		t.Fatalf("free body = %s", free.Body.String())
	}
}

func TestHistoryListsTheFileSessions(t *testing.T) {
	stub := &stubEdits{history: []gsuc.EditView{*heldView("user-1")}}
	rec := serveEdits(NewEditSessionHandler(stub), http.MethodGet, "/google-sheets/files/sp-1/edit-sessions?page=1&pageSize=10", "")
	var body struct {
		Items      []struct{ ID string } `json:"items"`
		TotalItems int64                 `json:"total_items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK || stub.file != "ws-1/sp-1" || len(body.Items) != 1 || body.Items[0].ID != "es-1" || body.TotalItems != 1 {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestEverySheetCarriesItsEmbeddedEditorLink(t *testing.T) {
	sheet := &googlesheet.Sheet{ID: "s1", SpreadsheetID: "sp-1", TabID: 9}
	if got := toSheetResponse(sheet); got.EmbedURL != sheet.EmbedURL() || got.URL != sheet.URL() {
		t.Fatalf("embed = %s url = %s", got.EmbedURL, got.URL)
	}
}
