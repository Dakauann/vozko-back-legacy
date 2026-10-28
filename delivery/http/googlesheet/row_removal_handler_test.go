package googlesheet

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

	"vozko/domain/auth"
	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/infra/http/middleware"
	gsuc "vozko/usecases/googlesheet"
)

type stubRemovals struct {
	got     gsuc.RemoveRowInput
	kind    string
	err     error
	removal *googlesheet.RowRemoval
	views   []gsuc.RemovalView
}

func (s *stubRemovals) Cancel(_ context.Context, in gsuc.RemoveRowInput) (*googlesheet.RowRemoval, error) {
	s.got, s.kind = in, "cancel"
	return s.removal, s.err
}

func (s *stubRemovals) Remove(_ context.Context, in gsuc.RemoveRowInput) (*googlesheet.RowRemoval, error) {
	s.got, s.kind = in, "remove"
	return s.removal, s.err
}

func (s *stubRemovals) Removals(_ context.Context, workspaceID, sheetID string, p shared.Pagination) (*shared.PaginatedResult[gsuc.RemovalView], error) {
	s.got = gsuc.RemoveRowInput{WorkspaceID: workspaceID, SheetID: sheetID}
	if s.err != nil {
		return nil, s.err
	}
	return &shared.PaginatedResult[gsuc.RemovalView]{Items: s.views, Page: p.Page, PageSize: p.PageSize, TotalItems: int64(len(s.views)), TotalPages: 1}, nil
}

func serveRemoval(h *RowRemovalHandler, method, path, body string) *httptest.ResponseRecorder {
	router := mux.NewRouter()
	router.HandleFunc("/google-sheets/{id}/rows/{rowId}/cancel", h.Cancel).Methods(http.MethodPost)
	router.HandleFunc("/google-sheets/{id}/rows/{rowId}/remove", h.Remove).Methods(http.MethodPost)
	router.HandleFunc("/google-sheets/{id}/removals", h.List).Methods(http.MethodGet)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "ws-1")
	ctx = context.WithValue(ctx, middleware.ClaimsContextKey, &auth.Claims{UserID: "user-1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

func removedRow() *googlesheet.RowRemoval {
	return &googlesheet.RowRemoval{
		ID: "rm-1", RowID: "row-1", Kind: googlesheet.RemovalDelete, Reason: "duplicada", RemovedBy: "user-1", RowNumber: 4,
		Cells: googlesheet.Cells{"nome": "Ana"}, Origin: googlesheet.Origin{Kind: googlesheet.OriginManual},
		RemovedAt: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
	}
}

func TestCancelAndRemoveCarryTheRowThePersonAndTheReason(t *testing.T) {
	for _, action := range []string{"cancel", "remove"} {
		stub := &stubRemovals{removal: removedRow()}
		rec := serveRemoval(NewRowRemovalHandler(stub), http.MethodPost, "/google-sheets/s1/rows/row-1/"+action, `{"reason":"duplicada"}`)
		if rec.Code != http.StatusOK || stub.kind != action {
			t.Fatalf("%s: status %d kind %s: %s", action, rec.Code, stub.kind, rec.Body.String())
		}
		want := gsuc.RemoveRowInput{WorkspaceID: "ws-1", SheetID: "s1", RowID: "row-1", ActorID: "user-1", Reason: "duplicada"}
		if stub.got != want {
			t.Fatalf("%s: input = %+v", action, stub.got)
		}
		var body struct {
			Removal struct {
				ID        string `json:"id"`
				RowID     string `json:"rowId"`
				Kind      string `json:"kind"`
				RowNumber int    `json:"rowNumber"`
			} `json:"removal"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Removal.ID != "rm-1" || body.Removal.Kind != "remove" || body.Removal.RowNumber != 4 {
			t.Fatalf("%s: body = %s", action, rec.Body.String())
		}
	}
}

func TestRemovalRefusalsKeepTheirCodes(t *testing.T) {
	cases := map[error]struct {
		status int
		code   string
	}{
		googlesheet.ErrRowNotInSheet:               {http.StatusConflict, googlesheet.CodeRowNotInSheet},
		googlesheet.ErrRowRefRepeated:              {http.StatusConflict, googlesheet.CodeRowRefRepeated},
		googlesheet.ErrRowBeingWritten:             {http.StatusConflict, googlesheet.CodeRowBeingWritten},
		googlesheet.ErrRowStatusChanged:            {http.StatusConflict, googlesheet.CodeRowStatusChanged},
		googlesheet.ErrSheetBusy:                   {http.StatusConflict, googlesheet.CodeSheetBusy},
		googlesheet.ErrRowNotFound:                 {http.StatusNotFound, googlesheet.CodeRowNotFound},
		googlesheet.ErrRemovalReasonRequired:       {http.StatusBadRequest, googlesheet.CodeRemovalReasonRequired},
		googlesheet.ErrRemovalReasonTooLong:        {http.StatusBadRequest, googlesheet.CodeRemovalReasonTooLong},
		googlesheet.ErrRemovalActorRequired:        {http.StatusBadRequest, googlesheet.CodeRemovalActorRequired},
		errors.Join(googlesheet.ErrSheetNotActive): {http.StatusConflict, googlesheet.CodeSheetNotActive},
		&gsuc.GoogleRefusal{Failure: googlesheet.Failure{Kind: googlesheet.FailureNotFound}, Err: errors.New("404")}: {http.StatusBadGateway, googlesheet.CodeWriteSpreadsheetMissing},
	}
	for err, want := range cases {
		rec := serveRemoval(NewRowRemovalHandler(&stubRemovals{err: err}), http.MethodPost, "/google-sheets/s1/rows/row-1/remove", `{"reason":"x"}`)
		if code, _ := codeOf(t, rec); rec.Code != want.status || code != want.code {
			t.Errorf("%v: status %d code %s, want %d %s", err, rec.Code, code, want.status, want.code)
		}
	}
	rec := serveRemoval(NewRowRemovalHandler(&stubRemovals{}), http.MethodPost, "/google-sheets/s1/rows/row-1/cancel", `{"reason":`)
	if code, _ := codeOf(t, rec); rec.Code != http.StatusBadRequest || code != "invalid_body" {
		t.Fatalf("broken body: %d %s", rec.Code, code)
	}
}

func TestTheRemovedListNamesWhoRemovedAndWhereTheRowCameFrom(t *testing.T) {
	stub := &stubRemovals{views: []gsuc.RemovalView{{
		Removal:   removedRow(),
		Origin:    gsuc.OriginView{Kind: googlesheet.OriginWorkflow, Workflow: &gsuc.NamedRef{ID: "wf-1", Name: "Cobrança"}, Conversation: &shared.EntryRef{EntryID: "c-1", EntryType: shared.EntryTypeWhatsApp}},
		RemovedBy: &gsuc.NamedRef{ID: "user-1", Name: "Ana Paula"},
	}}}
	rec := serveRemoval(NewRowRemovalHandler(stub), http.MethodGet, "/google-sheets/s1/removals?page=1", "")
	if rec.Code != http.StatusOK || stub.got.SheetID != "s1" || stub.got.WorkspaceID != "ws-1" {
		t.Fatalf("status %d input %+v", rec.Code, stub.got)
	}
	var body struct {
		Items []struct {
			Reason       string            `json:"reason"`
			Values       map[string]any    `json:"values"`
			RemovedBy    map[string]string `json:"removedBy"`
			OriginDetail map[string]any    `json:"originDetail"`
			RemovedAt    time.Time         `json:"removedAt"`
		} `json:"items"`
		TotalItems int `json:"total_items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Items) != 1 || body.TotalItems != 1 {
		t.Fatalf("body = %s", rec.Body.String())
	}
	item := body.Items[0]
	if item.Reason != "duplicada" || item.Values["nome"] != "Ana" || item.RemovedBy["name"] != "Ana Paula" || item.OriginDetail["kind"] != "workflow" || item.RemovedAt.IsZero() {
		t.Fatalf("item = %+v", item)
	}
}

func TestListedRowsSayWhichRemovalTheyOffer(t *testing.T) {
	stub := &stubSheets{rows: []gsuc.RowView{
		{Row: &googlesheet.Row{ID: "a", Status: googlesheet.RowPending}},
		{Row: &googlesheet.Row{ID: "b", Status: googlesheet.RowWritten}},
		{Row: &googlesheet.Row{ID: "c", Status: googlesheet.RowInFlight}},
	}}
	rec := serve(NewHandler(stub, &stubRows{}), http.MethodGet, "/google-sheets/s1/rows", "", nil)
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Items) != 3 {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if body.Items[0]["removal"] != "cancel" || body.Items[1]["removal"] != "remove" {
		t.Fatalf("removals = %v %v", body.Items[0]["removal"], body.Items[1]["removal"])
	}
	if _, offered := body.Items[2]["removal"]; offered {
		t.Fatal("a row being written offers no removal")
	}
}

func TestTheCatalogCarriesTheRemovalReasonLimitTheDomainEnforces(t *testing.T) {
	rec := serve(NewHandler(&stubSheets{}, &stubRows{}), http.MethodGet, "/google-sheets/catalog", "", nil)
	var body struct {
		MaxRemovalReason int `json:"maxRemovalReason"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.MaxRemovalReason != googlesheet.MaxRemovalReasonRunes {
		t.Fatalf("catalog = %s", rec.Body.String())
	}
}
