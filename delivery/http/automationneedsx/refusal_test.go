package automationneedsx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
)

func TestEveryCodeHasAStatusAndAMessage(t *testing.T) {
	if err := CheckCodes(); err != nil {
		t.Fatal(err)
	}
}

func TestARefusalIsWrittenWithItsDetails(t *testing.T) {
	link := domain.Link{Automation: conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "wf-1"}, Name: "Cobrança"}
	err := fmt.Errorf("save: %w", &domain.Refusal{Cause: domain.ErrReceptiveUnmet, Automation: link, Needs: domain.Needs{{Name: "valor", Via: []domain.Link{link}}}})
	rec := httptest.NewRecorder()
	if !WriteRefusal(rec, err) {
		t.Fatal("a refusal must be written")
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Code    string         `json:"code"`
		Details RefusalDetails `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "automation_variables_unmet" || body.Details.Automation.Name != "Cobrança" || body.Details.Variables[0].Name != "valor" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestOtherErrorsAreLeftToTheCaller(t *testing.T) {
	if WriteRefusal(httptest.NewRecorder(), fmt.Errorf("other")) {
		t.Fatal("a plain error is not a refusal")
	}
}
