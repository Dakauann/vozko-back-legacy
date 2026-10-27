package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/delivery/http/response/responsetest"
	"vozko/domain/auth"
	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/workflow"
	"vozko/infra/http/middleware"
	"vozko/pkg/codegolden"
	conversation_usecase "vozko/usecases/conversation"
)

const responsibleWorkflowErrorCodesAdvice = "update testdata/responsible_workflow_error_codes.golden, and also translate crmConversationHeader.responsibleWorkflow.errors.<code> in the vozko-front pt, en, es and de catalogs"

type stubResponsibleWorkflow struct {
	viewer                          shared.Person
	workspaceID, entryID, entryType string
	found                           *conversation_usecase.ResponsibleWorkflow
	err                             error
}

func (s *stubResponsibleWorkflow) Execute(viewer shared.Person, workspaceID, entryID, entryType string) (*conversation_usecase.ResponsibleWorkflow, error) {
	s.viewer, s.workspaceID, s.entryID, s.entryType = viewer, workspaceID, entryID, entryType
	return s.found, s.err
}

func responsibleWorkflowRequest(claims *auth.Claims) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/conversations/instagram/entry-1/responsible-workflow", nil)
	r = mux.SetURLVars(r, map[string]string{"entryType": "instagram", "entryId": "entry-1"})
	ctx := context.WithValue(r.Context(), middleware.WorkspaceIDContextKey, "ws-1")
	if claims != nil {
		ctx = context.WithValue(ctx, middleware.ClaimsContextKey, claims)
	}
	return r.WithContext(ctx)
}

func TestTheConversationAnswersItsResponsibleWorkflow(t *testing.T) {
	stub := &stubResponsibleWorkflow{found: &conversation_usecase.ResponsibleWorkflow{ID: "wf-1", Name: "Atendimento", State: workflow.RunningActive}}
	rec := httptest.NewRecorder()
	(&ConversationHandler{responsibleWorkflow: stub}).GetResponsibleWorkflow(rec, responsibleWorkflowRequest(&auth.Claims{UserID: "bob", Role: "user"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Workflow *struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"workflow"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Workflow == nil || body.Workflow.ID != "wf-1" || body.Workflow.Name != "Atendimento" || body.Workflow.State != "active" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if stub.viewer.UserID != "bob" || stub.workspaceID != "ws-1" || stub.entryID != "entry-1" || stub.entryType != "instagram" {
		t.Fatalf("use case got %+v", stub)
	}
}

func TestAConversationWithNoResponsibleWorkflowAnswersNull(t *testing.T) {
	rec := httptest.NewRecorder()
	(&ConversationHandler{responsibleWorkflow: &stubResponsibleWorkflow{}}).GetResponsibleWorkflow(rec, responsibleWorkflowRequest(&auth.Claims{UserID: "bob"}))
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
	}
	if value, present := body["workflow"]; !present || value != nil {
		t.Fatalf("body = %s, want workflow null", rec.Body.String())
	}
}

func TestEveryResponsibleWorkflowRefusalWritesItsStableCode(t *testing.T) {
	responsetest.RequireStableCodes(t, responsibleWorkflowRefusals)
	cases := []struct {
		name   string
		err    error
		claims *auth.Claims
		want   response.Refusal
	}{
		{"a hidden conversation", conversation.ErrNotVisible, &auth.Claims{UserID: "bob"}, refusalResponsibleConversationHidden},
		{"an unreadable responsibility", errors.New("db down"), &auth.Claims{UserID: "bob"}, refusalResponsibleWorkflowFailed},
		{"no caller", nil, nil, refusalResponsibleUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			(&ConversationHandler{responsibleWorkflow: &stubResponsibleWorkflow{err: tc.err}}).GetResponsibleWorkflow(rec, responsibleWorkflowRequest(tc.claims))
			if rec.Code != tc.want.Status || responsetest.CodeOf(t, rec) != tc.want.Code {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestResponsibleWorkflowErrorCodesMatchTheGoldenSoTheFrontTranslatesThem(t *testing.T) {
	codegolden.Compare(t, filepath.Join("testdata", "responsible_workflow_error_codes.golden"), response.RefusalCodes(responsibleWorkflowRefusals), responsibleWorkflowErrorCodesAdvice)
}
