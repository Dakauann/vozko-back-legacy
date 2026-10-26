package unofficial_whatsapp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/delivery/http/response"
	ia "vozko/domain/inbox_assignment"
)

func TestAStartTheStarterCannotOwnIsACodedRefusal(t *testing.T) {
	rec := httptest.NewRecorder()

	writeDomainError(rec, ia.StarterNotAssigned(errors.New("db down")))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var body response.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != ia.CodeStarterNotAssigned {
		t.Fatalf("code = %q, want %q so the front can translate it", body.Code, ia.CodeStarterNotAssigned)
	}
}
