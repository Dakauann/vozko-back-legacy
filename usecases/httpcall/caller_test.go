package httpcall_usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vozko/domain/calltrace"
	"vozko/domain/httpcall"
	"vozko/domain/redact"
	"vozko/domain/shared"
	"vozko/domain/workspacesecret"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func respond(status int, body string, headers http.Header) roundTrip {
	return func(r *http.Request) (*http.Response, error) {
		if headers == nil {
			headers = http.Header{}
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: headers, Request: r}, nil
	}
}

func TestDoSendsTheRequestAndRecordsTheExchange(t *testing.T) {
	var seen *http.Request
	var sentBody string
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		seen = r
		b, _ := io.ReadAll(r.Body)
		sentBody = string(b)
		return respond(201, `{"ok":true}`, http.Header{"X-Trace": {"t1"}})(r)
	})}
	out := NewCaller(Egress{Client: client}).Do(context.Background(), Prepared{
		Method: "POST", URL: "https://api.test/x", Body: `{"a":1}`,
		Headers: map[string]string{"X-Source": "vozko"}, Auth: httpcall.Auth{Mode: httpcall.AuthBearer, Token: "tok"},
		Timeout: time.Second, ResponseLimit: 1024,
	})
	if out.Err != nil || out.Status != 201 || string(out.Body) != `{"ok":true}` {
		t.Fatalf("out %+v", out)
	}
	if seen.Header.Get("Authorization") != "Bearer tok" || seen.Header.Get("Content-Type") != "application/json" || seen.Header.Get("X-Source") != "vozko" || sentBody != `{"a":1}` {
		t.Fatalf("request headers %v body %q", seen.Header, sentBody)
	}
	x := out.Exchange
	if x.Method != "POST" || x.URL != "https://api.test/x" || x.Status != 201 || x.ResponseHeaders["X-Trace"] != "t1" || x.RequestBody != `{"a":1}` {
		t.Fatalf("exchange %+v", x)
	}
}

func TestTheResponseIsCappedAndMarkedTruncated(t *testing.T) {
	client := &http.Client{Transport: respond(200, strings.Repeat("a", 100), nil)}
	out := NewCaller(Egress{Client: client}).Do(context.Background(), Prepared{Method: "GET", URL: "https://api.test", Timeout: time.Second, ResponseLimit: 10})
	if len(out.Body) != 10 || !out.Exchange.Truncated {
		t.Fatalf("body %d truncated %v", len(out.Body), out.Exchange.Truncated)
	}
}

func TestATLSHandshakeFailureRetriesOnTheLegacyClient(t *testing.T) {
	modern := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("remote error: tls: handshake failure")
	})}
	legacy := &http.Client{Transport: respond(200, "ok", nil)}
	out := NewCaller(Egress{Client: modern, LegacyTLS: legacy}).Do(context.Background(), Prepared{Method: "GET", URL: "https://old.test", Timeout: time.Second, ResponseLimit: 10})
	if out.Err != nil || !out.Exchange.TLSFallback || out.Status != 200 {
		t.Fatalf("out %+v", out)
	}
}

func TestEgressRefusalAndTimeoutAreClassified(t *testing.T) {
	refused := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial: %w", shared.ErrEgressBlocked)
	})}
	out := NewCaller(Egress{Client: refused}).Do(context.Background(), Prepared{Method: "GET", URL: "https://10.0.0.1", Timeout: time.Second, ResponseLimit: 10})
	if out.Kind != calltrace.ErrorKindTransport || out.Exchange.EgressRefusal == "" {
		t.Fatalf("out %+v", out)
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer slow.Close()
	out = NewCaller(Egress{Client: slow.Client()}).Do(context.Background(), Prepared{Method: "GET", URL: slow.URL, Timeout: 50 * time.Millisecond, ResponseLimit: 10})
	if out.Kind != calltrace.ErrorKindTimeout {
		t.Fatalf("out %+v", out)
	}
}

func TestNoEgressIsAConfigFailure(t *testing.T) {
	out := NewCaller(Egress{}).Do(context.Background(), Prepared{Method: "GET", URL: "https://a.test"})
	if out.Kind != calltrace.ErrorKindConfig || out.Err == nil {
		t.Fatalf("out %+v", out)
	}
}

type resolver map[string]string

func (r resolver) Resolve(_ context.Context, _ string, names []string) (map[string]string, error) {
	out := map[string]string{}
	for _, n := range names {
		if v, ok := r[n]; ok {
			out[n] = v
		}
	}
	return out, nil
}

func TestSecretsResolveFailClosedAndScrub(t *testing.T) {
	got, err := ResolveSecrets(context.Background(), resolver{"k": "s3cr3t"}, "ws", []string{"k"})
	if err != nil || got.Values["k"] != "s3cr3t" || got.Scrub.String("x s3cr3t y") != "x "+redact.Mask+" y" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if _, err := ResolveSecrets(context.Background(), resolver{}, "ws", []string{"absent"}); !errors.Is(err, workspacesecret.ErrMissing) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := ResolveSecrets(context.Background(), nil, "ws", []string{"k"}); !errors.Is(err, workspacesecret.ErrUnavailable) {
		t.Fatalf("unavailable: %v", err)
	}
	if _, err := ResolveSecrets(context.Background(), resolver{}, "", []string{"k"}); !errors.Is(err, workspacesecret.ErrWorkspaceRequired) {
		t.Fatalf("workspace: %v", err)
	}
	if none, err := ResolveSecrets(context.Background(), nil, "", nil); err != nil || len(none.Values) != 0 {
		t.Fatalf("no names: %+v %v", none, err)
	}
}
