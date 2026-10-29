package sitepreview

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vozko/domain/sitepublish"
	"vozko/usecases/sitebuild"
)

type fakeServer struct {
	host, path string
	served     sitebuild.Served
	err        error
}

func (f *fakeServer) Serve(_ context.Context, host, path string) (sitebuild.Served, error) {
	f.host, f.path = host, path
	return f.served, f.err
}

func newHandler(t *testing.T, server Server) *Handler {
	t.Helper()
	h, err := NewHandler(server, Origins{Editors: []string{"https://app.vozkoia.com", "http://localhost:3000"}, API: "https://api.vozkoia.com", Media: "https://cdn.vozkoia.com"})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestNewHandlerRefusesMissingOrigins(t *testing.T) {
	good := Origins{Editors: []string{"https://app.vozkoia.com"}, API: "https://api.vozkoia.com", Media: "https://cdn.vozkoia.com"}
	withEditors := func(editors ...string) Origins { o := good; o.Editors = editors; return o }
	if _, err := NewHandler(&fakeServer{}, withEditors()); err == nil {
		t.Fatal("frame ancestors are required")
	}
	for _, bad := range []string{"app.vozkoia.com", "https://app.vozkoia.com/editor", "ftp://app.vozkoia.com", "https://x.com;script-src *"} {
		if _, err := NewHandler(&fakeServer{}, withEditors(bad)); err == nil {
			t.Fatalf("%q is not an editor origin", bad)
		}
	}
	noAPI := good
	noAPI.API = "javascript:x"
	if _, err := NewHandler(&fakeServer{}, noAPI); err == nil {
		t.Fatal("the api origin must be an origin")
	}
	for _, bad := range []string{"", "cdn.vozkoia.com", "https://cdn.vozkoia.com/media", "https://x.com;connect-src *"} {
		media := good
		media.Media = bad
		if _, err := NewHandler(&fakeServer{}, media); err == nil {
			t.Fatalf("%q is not a media origin", bad)
		}
	}
	if _, err := NewHandler(nil, good); err == nil {
		t.Fatal("the server is required")
	}
}

func TestServesThePreviewWithSecurityHeaders(t *testing.T) {
	server := &fakeServer{served: sitebuild.Served{ContentType: "text/html; charset=utf-8", Body: []byte("<html></html>")}}
	h := newHandler(t, server)
	req := httptest.NewRequest(http.MethodGet, "https://preview--tok.paginas.test/obrigado", nil)
	req.Header.Set("Cookie", "session=secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "<html></html>" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	if server.host != "preview--tok.paginas.test" || server.path != "/obrigado" {
		t.Fatalf("got %+v", server)
	}
	headers := rec.Header()
	csp := headers.Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "script-src 'self'", "frame-ancestors https://app.vozkoia.com http://localhost:3000", "connect-src 'self' https://api.vozkoia.com https://cdn.vozkoia.com", "form-action 'self' https://api.vozkoia.com", "object-src 'none'", "base-uri 'none'"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("csp %q misses %q", csp, want)
		}
	}
	if strings.Contains(csp, "unsafe-eval") || strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("csp too loose: %q", csp)
	}
	checks := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Content-Type":           "text/html; charset=utf-8",
		"Cache-Control":          "no-store",
		"Referrer-Policy":        "no-referrer",
	}
	for name, want := range checks {
		if headers.Get(name) != want {
			t.Fatalf("%s: got %q", name, headers.Get(name))
		}
	}
	if headers.Get("Set-Cookie") != "" || headers.Get("X-Frame-Options") != "" {
		t.Fatalf("got %+v", headers)
	}
}

func TestHeadAnswersWithoutABody(t *testing.T) {
	h := newHandler(t, &fakeServer{served: sitebuild.Served{ContentType: "text/css", Body: []byte("a{}")}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "https://preview--tok.paginas.test/a.css", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestRefusalsMapToStatuses(t *testing.T) {
	cases := []struct {
		err    error
		method string
		want   int
	}{
		{sitebuild.ErrPreviewNotFound, http.MethodGet, http.StatusNotFound},
		{sitebuild.ErrPreviewUnavailable, http.MethodGet, http.StatusServiceUnavailable},
		{errors.New("other"), http.MethodGet, http.StatusServiceUnavailable},
		{nil, http.MethodPost, http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		h := newHandler(t, &fakeServer{err: c.err})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, "https://preview--tok.paginas.test/", nil))
		if rec.Code != c.want {
			t.Fatalf("%v %s: got %d", c.err, c.method, rec.Code)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("refusals keep the security headers: %+v", rec.Header())
		}
	}
}

func TestDispatchRoutesPreviewHostsBeforeTheAPI(t *testing.T) {
	preview := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(299) })
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(298) })
	handler := Dispatch(sitepublish.Addresses{Domain: "paginas.test"}, preview, api)
	cases := map[string]int{
		"preview--tok.paginas.test":      299,
		"PREVIEW--TOK.Paginas.Test:8443": 299,
		"preview--tok.paginas.test.":     299,
		"tok.paginas.test":               298,
		"deploy--tok.paginas.test":       298,
		"preview--.paginas.test":         298,
		"paginas.test":                   298,
		"api.vozkoia.com":                298,
		"preview--tok.paginas.test.evil": 298,
		"preview--tok.notpaginas.test":   298,
		"a.preview--tok.paginas.test":    298,
		"":                               298,
	}
	for host, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s: got %d want %d", host, rec.Code, want)
		}
	}
}

func TestThePreviewMayLoadOnlyTheSiteChatLoaderAndFrame(t *testing.T) {
	server := &fakeServer{served: sitebuild.Served{ContentType: "text/html; charset=utf-8", Body: []byte("<html></html>")}}
	rec := httptest.NewRecorder()
	newHandler(t, server).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "https://preview--tok.paginas.test/", nil))
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"script-src 'self' https://api.vozkoia.com/public/webchat/loader.js;", "frame-src https://api.vozkoia.com/public/webchat/;"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("csp %q misses %q", csp, want)
		}
	}
}
