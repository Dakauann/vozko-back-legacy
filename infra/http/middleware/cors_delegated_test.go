package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSLeavesDelegatedRoutesToTheirOwnRule(t *testing.T) {
	cors := NewCORSMiddleware([]string{"https://app.vozko.com.br"}).Delegating("/public/sites/forms/")
	reached := false
	handler := cors.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodOptions, "/public/sites/forms/key-1/submissions", nil)
	req.Header.Set("Origin", "https://evil.example")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !reached || rr.Code != http.StatusTeapot || rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("delegated preflight %d %v", rr.Code, rr.Header())
	}
	other := httptest.NewRequest(http.MethodOptions, "/public/webchat/session", nil)
	other.Header.Set("Origin", "https://evil.example")
	rr = httptest.NewRecorder()
	reached = false
	handler.ServeHTTP(rr, other)
	if reached || rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("other routes keep the shared rule: %d %v", rr.Code, rr.Header())
	}
}
