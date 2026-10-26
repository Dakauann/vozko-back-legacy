package crmboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/crmfilter"
)

func TestAnUnknownConversationOriginAnswersACoded400(t *testing.T) {
	rec := httptest.NewRecorder()
	(&CRMBoardHandler{}).handleError(rec, fmt.Errorf("compile: %w: %w", crmfilter.ErrInvalidValue, conversation.ErrOriginInvalid))

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest || body.Code != CodeOriginInvalid {
		t.Fatalf("status %d code %q, want 400 %s", rec.Code, body.Code, CodeOriginInvalid)
	}
}

func TestEveryInvalidFilterValueIsABadRequestNotAServerError(t *testing.T) {
	for _, err := range []error{crmfilter.ErrInvalidValue, crmfilter.ErrTooManyValues, crmfilter.ErrNotApplicable} {
		rec := httptest.NewRecorder()
		(&CRMBoardHandler{}).handleError(rec, fmt.Errorf("compile: %w", err))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%v answered %d, want 400", err, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	(&CRMBoardHandler{}).handleError(rec, errors.New("boom"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("an unknown failure answered %d, want 500", rec.Code)
	}
}
