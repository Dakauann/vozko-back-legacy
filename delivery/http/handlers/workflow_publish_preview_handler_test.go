package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/domain/auth"
	"vozko/domain/workflow"
)

type previewingWorkflows struct {
	workflow.ScopedWorkflowsUseCase
	err   error
	scope workflow.Scope
	id    string
}

func (f *previewingWorkflows) PublishPreview(scope workflow.Scope, id string) (*workflow.RunSummary, error) {
	f.scope, f.id = scope, id
	if f.err != nil {
		return nil, f.err
	}
	return &workflow.RunSummary{
		State:      workflow.RunningActive,
		Conditions: []workflow.RunCondition{{Kind: workflow.EntryBirthday, Scope: workflow.TriggerScopeGlobal, NodeID: "b"}},
		Overlaps:   []workflow.RunOverlap{{Kind: workflow.EntryBirthday, Workflows: []workflow.WorkflowRef{{ID: "w9", Name: "Cupom"}}}},
	}, nil
}

func TestThePublishPreviewAnswersTheRunSummaryOfTheStoredDraft(t *testing.T) {
	workflows := &previewingWorkflows{}
	rec := httptest.NewRecorder()
	(&WorkflowHandler{workflows: workflows}).PublishPreview(rec, workflowRequestAs(http.MethodGet, "/workflows/w1/publish-preview", nil, &auth.Claims{UserID: "u1"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body workflow.RunSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.State != workflow.RunningActive || len(body.Conditions) != 1 || len(body.Overlaps) != 1 || body.Overlaps[0].Workflows[0].Name != "Cupom" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if workflows.id != "w1" || workflows.scope.WorkspaceID != "ws1" || workflows.scope.Requester.UserID != "u1" {
		t.Fatalf("asked %q in %+v", workflows.id, workflows.scope)
	}
}

func TestThePublishPreviewRefusesWithTheWorkflowCodes(t *testing.T) {
	cases := map[error]int{
		workflow.ErrWorkflowNotFound:       http.StatusNotFound,
		workflow.ErrWorkflowAccessDenied:   http.StatusForbidden,
		errors.New("entry points offline"): http.StatusInternalServerError,
	}
	for err, want := range cases {
		rec := httptest.NewRecorder()
		(&WorkflowHandler{workflows: &previewingWorkflows{err: err}}).PublishPreview(rec, workflowRequestAs(http.MethodGet, "/workflows/w1/publish-preview", nil, &auth.Claims{UserID: "u1"}))
		var body struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != want || body.Code == "" {
			t.Errorf("%v: status = %d body = %s, want %d with a code", err, rec.Code, rec.Body.String(), want)
		}
	}
}
