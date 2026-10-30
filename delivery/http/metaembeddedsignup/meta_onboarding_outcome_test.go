package metaembeddedsignup

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAMovingNumberIsRegisteredInTheAccountBeingConnected(t *testing.T) {
	uc := &fakeOnboardUC{register: true}
	h, transport := newQuotaHandler(uc)

	rec := runMetaOnboarding(h)

	if rec.Code != http.StatusOK || !transport.hit("/meta-1/register") {
		t.Fatalf("code = %d, calls = %v", rec.Code, transport.urls)
	}
	if len(uc.asked) != 1 || uc.asked[0] != "meta-1@waba-1" {
		t.Fatalf("asked = %v", uc.asked)
	}
}

func TestANumberAlreadyInTheAccountIsNotRegisteredAgain(t *testing.T) {
	uc := &fakeOnboardUC{register: false}
	h, transport := newQuotaHandler(uc)

	runMetaOnboarding(h)

	if transport.hit("/register") {
		t.Fatalf("registered a number already in this account: %v", transport.urls)
	}
}

func TestAnUnreadableNumberStopsTheOnboardingBeforeAnythingIsSaved(t *testing.T) {
	uc := &fakeOnboardUC{registrationErr: errors.New("database unavailable")}
	h, transport := newQuotaHandler(uc)

	rec := runMetaOnboarding(h)

	if rec.Code < http.StatusInternalServerError || uc.executed != 0 || transport.hit("/register") {
		t.Fatalf("code = %d, executed = %d, calls = %v", rec.Code, uc.executed, transport.urls)
	}
}

func TestAFailedSaveReachesTheScreenAsAnError(t *testing.T) {
	uc := &fakeOnboardUC{executeErr: errors.New("database unavailable")}
	h, _ := newQuotaHandler(uc)

	rec := runMetaOnboarding(h)

	if rec.Code < http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"error":true`) {
		t.Fatalf("a failed save answered %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAnAccountWithoutANumberReachesTheScreenAsAnError(t *testing.T) {
	uc := &fakeOnboardUC{}
	h, _ := newQuotaHandler(uc)
	rec := httptest.NewRecorder()

	h.handleMetaOnboarding(rec, httptest.NewRequest(http.MethodPost, "/oauth/meta/embedded?workspace_id=ws-1", nil),
		EmbeddedSignupCallbackRequest{AccessToken: "token", WABAID: "waba-1"}, "ws-1", "user-1")

	if rec.Code < http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"error":true`) || uc.executed != 0 {
		t.Fatalf("an account with no number answered %d: %s", rec.Code, rec.Body.String())
	}
}
