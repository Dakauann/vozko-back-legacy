package httpcall

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimeoutDefaultsTo15SecondsAndCapsAt30(t *testing.T) {
	if Timeout(0) != 15*time.Second || Timeout(5) != 5*time.Second || Timeout(90) != 30*time.Second || Timeout(0.5) != 500*time.Millisecond {
		t.Fatal("unexpected timeout")
	}
}

func TestAuthModesSetTheirHeaders(t *testing.T) {
	r := httptest.NewRequest("GET", "https://a.test", nil)
	Auth{Mode: AuthBearer, Token: "Bearer abc"}.Apply(r)
	if r.Header.Get("Authorization") != "Bearer abc" {
		t.Fatalf("bearer %q", r.Header.Get("Authorization"))
	}
	r = httptest.NewRequest("GET", "https://a.test", nil)
	Auth{Mode: AuthAPIKey, Token: "k"}.Apply(r)
	if r.Header.Get("X-API-Key") != "k" {
		t.Fatal("api key")
	}
	r = httptest.NewRequest("GET", "https://a.test", nil)
	Auth{Mode: AuthBasic, User: "u", Token: "p"}.Apply(r)
	if user, pass, ok := r.BasicAuth(); !ok || user != "u" || pass != "p" {
		t.Fatal("basic")
	}
}

func TestOnlyIdempotentMethodsMayBeRetried(t *testing.T) {
	for method, want := range map[string]bool{"GET": true, "head": true, "PUT": true, "DELETE": true, "POST": false, "PATCH": false} {
		if IdempotentMethod(method) != want {
			t.Errorf("%s", method)
		}
	}
}

func TestOnlyGetAndHeadReadWithoutChangingData(t *testing.T) {
	for method, want := range map[string]bool{"GET": true, "head": true, " GET ": true, "PUT": false, "DELETE": false, "POST": false, "PATCH": false, "OPTIONS": false, "": false} {
		if got := ReadOnlyMethod(method); got != want {
			t.Fatalf("%q read only %v, want %v", method, got, want)
		}
	}
}
