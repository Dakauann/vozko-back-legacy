package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/delivery/http/response/responsetest"
	"vozko/domain/auth"
	"vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/workflow"
	workflow_usecase "vozko/usecases/workflow"
)

type testValuesEditorStub struct {
	userID  string
	saved   map[string]string
	cleared bool
	err     error
}

func (s *testValuesEditorStub) form() *workflow_usecase.TestValuesForm {
	link := automationneeds.Link{Automation: conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "w1"}, Name: "Fluxo"}
	return &workflow_usecase.TestValuesForm{Needs: automationneeds.Needs{{Name: "matricula", Via: []automationneeds.Link{link}}}, Values: s.saved}
}

func (s *testValuesEditorStub) Form(_ *workflow.Workflow, userID string) (*workflow_usecase.TestValuesForm, error) {
	s.userID = userID
	return s.form(), s.err
}

func (s *testValuesEditorStub) Save(_ *workflow.Workflow, userID string, values map[string]string) (*workflow_usecase.TestValuesForm, error) {
	s.userID = userID
	if s.err != nil {
		return nil, s.err
	}
	s.saved = values
	return s.form(), nil
}

func (s *testValuesEditorStub) Clear(_ *workflow.Workflow, userID string) error {
	s.userID = userID
	s.cleared = true
	return s.err
}

func testValuesHandler(editor TestValuesEditor) *WorkflowHandler {
	return &WorkflowHandler{workflows: &versionReadingWorkflows{}, testValues: editor}
}

func TestTheTestValuesAnswerTheNeedsAndTheViewersValues(t *testing.T) {
	editor := &testValuesEditorStub{saved: map[string]string{"matricula": "123"}}
	rec := httptest.NewRecorder()
	testValuesHandler(editor).TestValues(rec, workflowRequestAs(http.MethodGet, "/workflows/w1/test-values", nil, &auth.Claims{UserID: "u1"}))

	if rec.Code != http.StatusOK || editor.userID != "u1" {
		t.Fatalf("status = %d user = %q body = %s", rec.Code, editor.userID, rec.Body.String())
	}
	var body testValuesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Variables) != 1 || body.Variables[0].Name != "matricula" || body.Variables[0].Via[0].Name != "Fluxo" || body.Values["matricula"] != "123" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestSavingAndClearingTestValuesActAsTheViewer(t *testing.T) {
	editor := &testValuesEditorStub{}
	rec := httptest.NewRecorder()
	testValuesHandler(editor).SaveTestValues(rec, workflowRequestAs(http.MethodPut, "/workflows/w1/test-values", []byte(`{"values":{"matricula":"9"}}`), &auth.Claims{UserID: "u1"}))
	if rec.Code != http.StatusOK || editor.saved["matricula"] != "9" || editor.userID != "u1" {
		t.Fatalf("save status = %d saved = %v body = %s", rec.Code, editor.saved, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	testValuesHandler(editor).ClearTestValues(rec, workflowRequestAs(http.MethodDelete, "/workflows/w1/test-values", nil, &auth.Claims{UserID: "u1"}))
	if rec.Code != http.StatusNoContent || !editor.cleared {
		t.Fatalf("clear status = %d cleared = %v", rec.Code, editor.cleared)
	}
}

func TestTestValuesRefusalsCarryTheirCodes(t *testing.T) {
	cases := []struct {
		name   string
		editor TestValuesEditor
		body   string
		status int
		code   string
	}{
		{"a variable the workflow does not use", &testValuesEditorStub{err: fmt.Errorf("%w: cpf", workflow.ErrTestValueNotNeeded)}, `{"values":{"cpf":"1"}}`, http.StatusUnprocessableEntity, "test_value_not_needed"},
		{"a value past the limit", &testValuesEditorStub{err: fmt.Errorf("%w: matricula", workflow.ErrTestValueInvalid)}, `{"values":{"matricula":"x"}}`, http.StatusBadRequest, "test_value_invalid"},
		{"a store that is down", &testValuesEditorStub{err: workflow.ErrTestValuesUnavailable}, `{"values":{}}`, http.StatusServiceUnavailable, "test_values_unavailable"},
		{"a body that is not json", &testValuesEditorStub{}, `nope`, http.StatusBadRequest, "invalid_body"},
		{"no editor wired", nil, `{"values":{}}`, http.StatusNotImplemented, "test_values_unwired"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			testValuesHandler(tc.editor).SaveTestValues(rec, workflowRequestAs(http.MethodPut, "/workflows/w1/test-values", []byte(tc.body), &auth.Claims{UserID: "u1"}))
			if rec.Code != tc.status || responsetest.CodeOf(t, rec) != tc.code {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}
