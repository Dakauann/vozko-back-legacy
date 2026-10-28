package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	authdomain "vozko/domain/auth"
)

type countingLimiter struct {
	counted int
	allow   bool
}

func (l *countingLimiter) Validate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.counted++
		if !l.allow {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func publicRouter(uc authdomain.ResetPasswordUseCase, reset *countingLimiter) *mux.Router {
	open := func() *countingLimiter { return &countingLimiter{allow: true} }
	router := mux.NewRouter()
	RegisterPublicRoutes(router, resetHandler(uc), RateLimiters{
		Register:          open(),
		Login:             open(),
		ForgotPassword:    open(),
		ResetPassword:     reset,
		EmailVerification: open(),
	})
	return router
}

func postReset(router http.Handler, body interface{}) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewReader(data))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestResetRoute_RuleRefusalsNeverSpendTheResetLimit(t *testing.T) {
	uc := &recordingResetPassword{}
	limiter := &countingLimiter{allow: true}
	router := publicRouter(uc, limiter)

	for i := 0; i < 10; i++ {
		rr := postReset(router, map[string]string{"email": "user@test.com", "token": "123456", "newPassword": "12345678901!"})
		body := decodeRefusal(t, rr)
		if rr.Code != http.StatusBadRequest || body.Code != "password_missing_uppercase" {
			t.Fatalf("attempt %d got %d %s", i, rr.Code, rr.Body.String())
		}
	}
	if limiter.counted != 0 || uc.calls != 0 {
		t.Fatalf("rule refusals counted %d times and reached the code check %d times", limiter.counted, uc.calls)
	}
}

func TestResetRoute_IncompleteRequestsNeverSpendTheResetLimit(t *testing.T) {
	limiter := &countingLimiter{allow: true}
	router := publicRouter(&recordingResetPassword{}, limiter)

	for _, body := range []map[string]string{
		{"token": "123456", "newPassword": "NovaSenha1"},
		{"email": "user@test.com", "newPassword": "NovaSenha1"},
		{"email": "user@test.com", "token": "123456"},
	} {
		if rr := postReset(router, body); rr.Code != http.StatusBadRequest {
			t.Fatalf("got %d %s", rr.Code, rr.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewReader([]byte("bad")))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if decodeRefusal(t, rr).Code != "invalid_body" || limiter.counted != 0 {
		t.Fatalf("got %d %s, counted %d", rr.Code, rr.Body.String(), limiter.counted)
	}
}

func TestResetRoute_EveryAttemptThatReachesTheCodeIsCounted(t *testing.T) {
	uc := &recordingResetPassword{err: authdomain.ErrInvalidResetToken}
	limiter := &countingLimiter{allow: true}
	router := publicRouter(uc, limiter)

	rr := postReset(router, map[string]string{"email": "user@test.com", "token": "000000", "newPassword": " NovaSenha1 "})

	if decodeRefusal(t, rr).Code != "invalid_reset_code" || limiter.counted != 1 || uc.calls != 1 {
		t.Fatalf("got %s, counted %d, calls %d", rr.Body.String(), limiter.counted, uc.calls)
	}
	if uc.input != (authdomain.ResetPasswordInput{Email: "user@test.com", Token: "000000", NewPassword: " NovaSenha1 "}) {
		t.Fatalf("the use case received %+v", uc.input)
	}
}

func TestResetRoute_ASuccessfulResetIsCounted(t *testing.T) {
	limiter := &countingLimiter{allow: true}
	router := publicRouter(&recordingResetPassword{}, limiter)

	if rr := postReset(router, validReset()); rr.Code != http.StatusOK || limiter.counted != 1 {
		t.Fatalf("got %d, counted %d", rr.Code, limiter.counted)
	}
}

func TestResetRoute_AnExhaustedLimitStillStopsCodeGuesses(t *testing.T) {
	uc := &recordingResetPassword{}
	router := publicRouter(uc, &countingLimiter{allow: false})

	if rr := postReset(router, validReset()); rr.Code != http.StatusTooManyRequests || uc.calls != 0 {
		t.Fatalf("got %d, calls %d", rr.Code, uc.calls)
	}
}

func TestResetRoute_ARuleRefusalSaysNothingAboutTheAccount(t *testing.T) {
	router := publicRouter(&recordingResetPassword{}, &countingLimiter{allow: true})

	known := postReset(router, map[string]string{"email": "user@test.com", "token": "123456", "newPassword": "abc"})
	unknown := postReset(router, map[string]string{"email": "nobody@test.com", "token": "999999", "newPassword": "abc"})

	if known.Code != unknown.Code || known.Body.String() != unknown.Body.String() {
		t.Fatalf("rule refusals differ: %d %s vs %d %s", known.Code, known.Body.String(), unknown.Code, unknown.Body.String())
	}
}
