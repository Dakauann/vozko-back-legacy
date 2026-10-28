package shortlink

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequiredScannerNeverFallsBackToTheNoop(t *testing.T) {
	for _, key := range []string{"", "   "} {
		if s, err := NewRequiredThreatScanner(key, "", nil); !errors.Is(err, ErrScannerKeyMissing) || s != nil {
			t.Fatalf("key %q: got %v %v", key, s, err)
		}
	}
}

func TestRequiredScannerScansForReal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "key" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"matches":[{"threatType":"SOCIAL_ENGINEERING"}]}`))
	}))
	defer srv.Close()
	s, err := NewRequiredThreatScanner("key", srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := s.Scan(context.Background(), "https://x.com")
	if err != nil || verdict.Safe || verdict.Threats[0] != "SOCIAL_ENGINEERING" {
		t.Fatalf("got %+v %v", verdict, err)
	}
}
