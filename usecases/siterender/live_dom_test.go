package siterender

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLiveDOMOpensEveryRouteOnTheCandidateHostOnly(t *testing.T) {
	b := &fakeBrowser{live: map[string]string{"/": "<!doctype html><html><body>home</body></html>", "/precos": "<!doctype html><html><body>p</body></html>"}}
	got, err := newTestService(b, &fakeHosts{}).LiveDOM(context.Background(), LiveDOMRequest{URL: "https://r1abc.paginas.test/?t=sig", Routes: []string{"/", "/precos", "/"}})
	if err != nil {
		t.Fatalf("live dom: %v", err)
	}
	if b.liveJob.URL != "https://r1abc.paginas.test/?t=sig" || len(b.liveJob.Routes) != 2 {
		t.Fatalf("job %+v", b.liveJob)
	}
	ctx := context.Background()
	if !b.liveJob.Policy.Allow(ctx, "r1abc.paginas.test") || b.liveJob.Policy.Allow(ctx, "other.paginas.test") || b.liveJob.Policy.Allow(ctx, "cdn.example.com") {
		t.Fatalf("the live scan policy is not the candidate host only")
	}
	if got["/precos"] != "<!doctype html><html><body>p</body></html>" || len(got) != 2 {
		t.Fatalf("result %v", got)
	}
}

func TestLiveDOMRouteURLJoinsThePathAndKeepsTheQuery(t *testing.T) {
	cases := []struct {
		base, route, want string
	}{
		{"https://r1.paginas.test/?t=sig", "/", "https://r1.paginas.test/?t=sig"},
		{"https://r1.paginas.test/?t=sig", "/precos", "https://r1.paginas.test/precos?t=sig"},
		{"https://r1.paginas.test/c/abc/", "/precos/mensal", "https://r1.paginas.test/c/abc/precos/mensal"},
		{"https://r1.paginas.test", "/", "https://r1.paginas.test/"},
	}
	for _, c := range cases {
		got, err := LiveRouteURL(c.base, c.route)
		if err != nil || got != c.want {
			t.Fatalf("%s %s: got %q %v, want %q", c.base, c.route, got, err, c.want)
		}
	}
}

func TestLiveDOMFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		routes []string
		result map[string]string
		err    error
		want   error
	}{
		{"foreign host", "https://example.com/", []string{"/"}, nil, nil, ErrURLRefused},
		{"no routes", "https://r1.paginas.test/", nil, nil, nil, ErrRequestInvalid},
		{"bad route", "https://r1.paginas.test/", []string{"/../etc"}, nil, nil, ErrRequestInvalid},
		{"missing route in the result", "https://r1.paginas.test/", []string{"/", "/a"}, map[string]string{"/": "<html></html>"}, nil, ErrLiveScanFailed},
		{"empty document", "https://r1.paginas.test/", []string{"/"}, map[string]string{"/": "  "}, nil, ErrLiveScanFailed},
		{"oversized document", "https://r1.paginas.test/", []string{"/"}, map[string]string{"/": strings.Repeat("a", maxPrerenderHTMLBytes+1)}, nil, ErrLiveScanFailed},
		{"browser failure", "https://r1.paginas.test/", []string{"/"}, nil, errors.New("tab crashed"), ErrLiveScanFailed},
		{"renderer down", "https://r1.paginas.test/", []string{"/"}, nil, ErrUnavailable, ErrUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &fakeBrowser{live: c.result, liveErr: c.err}
			_, err := newTestService(b, &fakeHosts{}).LiveDOM(context.Background(), LiveDOMRequest{URL: c.url, Routes: c.routes})
			if !errors.Is(err, c.want) {
				t.Fatalf("err %v, want %v", err, c.want)
			}
		})
	}
}

func TestLiveScanFailureHasItsOwnCode(t *testing.T) {
	if got := ErrorCode(ErrLiveScanFailed); got != CodeLiveScanFailed || CodeLiveScanFailed != "live_scan_failed" {
		t.Fatalf("code %q", got)
	}
}
