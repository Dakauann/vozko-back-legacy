package httpx

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAStartRunsThroughItsGuard(t *testing.T) {
	reached, guarded := false, false
	guard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			guarded = true
			next(w, r)
		}
	}
	WithStart(guard, httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), func(http.ResponseWriter, *http.Request) { reached = true })
	if !guarded || !reached {
		t.Fatalf("guarded %v reached %v", guarded, reached)
	}
}

func TestAStartWithoutAGuardIsRefused(t *testing.T) {
	reached := false
	rec := httptest.NewRecorder()
	WithStart(nil, rec, httptest.NewRequest(http.MethodPost, "/", nil), func(http.ResponseWriter, *http.Request) { reached = true })
	if reached || rec.Code != http.StatusForbidden || !bytes.Contains(rec.Body.Bytes(), []byte(`"code":"start_permission_required"`)) {
		t.Fatalf("reached %v, %d %s", reached, rec.Code, rec.Body.String())
	}
}
