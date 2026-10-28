package sitehost

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	siteserve_usecase "vozko/usecases/siteserve"
)

type fakePages struct {
	claims   map[string]bool
	claimErr error
	outcome  siteserve_usecase.Outcome
	got      siteserve_usecase.Request
}

func (p *fakePages) Claims(_ context.Context, host string) (bool, error) {
	return p.claims[host], p.claimErr
}

func (p *fakePages) Serve(_ context.Context, req siteserve_usecase.Request) siteserve_usecase.Outcome {
	p.got = req
	return p.outcome
}

func serve(t *testing.T, pages *fakePages, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	handler, err := NewHandler(pages)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestDispatchSendsClaimedHostsToPagesAndTheRestToTheAPI(t *testing.T) {
	pages := &fakePages{claims: map[string]bool{"sorriso.paginas.test": true}}
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	page := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) })
	handler := Dispatch(pages, page, api)
	for host, want := range map[string]int{"sorriso.paginas.test": http.StatusAccepted, "api.vozkoia.com": http.StatusTeapot} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://"+host+"/", nil))
		if w.Code != want {
			t.Fatalf("%s: got %d", host, w.Code)
		}
	}
	pages.claimErr = errors.New("db down")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://www.sorriso.com.br/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("an unknown answer never guesses, got %d", w.Code)
	}
}

func TestContentCarriesThePolicyCachingAndCookies(t *testing.T) {
	pages := &fakePages{outcome: siteserve_usecase.Outcome{
		Kind: siteserve_usecase.KindContent,
		Content: siteserve_usecase.Content{
			Body: []byte("<html>oi</html>"), ContentType: "text/html; charset=utf-8", CacheControl: "private, no-store",
			ETag: `"abc"`, Policy: "default-src 'self'", NoIndex: true,
		},
		Cookies: []siteserve_usecase.Cookie{{Name: siteserve_usecase.VariantCookie, Value: "b", Path: "/", MaxAge: 30 * 24 * time.Hour}},
	}}
	r := httptest.NewRequest(http.MethodGet, "http://sorriso.paginas.test/?utm_source=x", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0")
	r.Header.Set("Referer", "https://www.google.com/")
	r.Header.Set("CF-Connecting-IP", "203.0.113.9")
	r.AddCookie(&http.Cookie{Name: siteserve_usecase.VariantCookie, Value: "control"})
	w := serve(t, pages, r)
	h := w.Result().Header
	if w.Code != http.StatusOK || w.Body.String() != "<html>oi</html>" {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	for key, want := range map[string]string{
		"Content-Security-Policy": "default-src 'self'", "Cache-Control": "private, no-store", "Etag": `"abc"`, "X-Robots-Tag": "noindex, nofollow",
		"X-Content-Type-Options": "nosniff", "Strict-Transport-Security": "max-age=31536000", "X-Frame-Options": "SAMEORIGIN",
	} {
		if h.Get(key) != want {
			t.Fatalf("%s: got %q", key, h.Get(key))
		}
	}
	if cookie := h.Get("Set-Cookie"); !strings.Contains(cookie, "vz_variant=b") || !strings.Contains(cookie, "Secure") || !strings.Contains(cookie, "SameSite=Lax") || strings.Contains(cookie, "HttpOnly") {
		t.Fatalf("got %s", cookie)
	}
	got := pages.got
	if got.Host != "sorriso.paginas.test" || got.Path != "/" || got.RawQuery != "utm_source=x" || got.ClientIP != "203.0.113.9" || got.UserAgent != "Mozilla/5.0" || got.Referer != "https://www.google.com/" || got.Cookie(siteserve_usecase.VariantCookie) != "control" {
		t.Fatalf("got %+v", got)
	}
}

func TestHeadAndMissingPagesKeepTheirStatus(t *testing.T) {
	pages := &fakePages{outcome: siteserve_usecase.Outcome{Kind: siteserve_usecase.KindContent, Content: siteserve_usecase.Content{Body: []byte("404"), ContentType: "text/html", Missing: true}}}
	if w := serve(t, pages, httptest.NewRequest(http.MethodGet, "http://a.paginas.test/nada", nil)); w.Code != http.StatusNotFound || w.Body.String() != "404" {
		t.Fatalf("got %d", w.Code)
	}
	pages.outcome.Content.Missing = false
	if w := serve(t, pages, httptest.NewRequest(http.MethodHead, "http://a.paginas.test/", nil)); w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
}

func TestBeaconAndPasswordBodiesReachTheUseCase(t *testing.T) {
	pages := &fakePages{outcome: siteserve_usecase.Outcome{Kind: siteserve_usecase.KindAccepted}}
	r := httptest.NewRequest(http.MethodPost, "http://a.paginas.test"+siteserve_usecase.BeaconPath, strings.NewReader(`{"event":"view"}`+strings.Repeat(" ", 2*siteserve_usecase.MaxBeaconBytes)))
	if w := serve(t, pages, r); w.Code != http.StatusNoContent {
		t.Fatalf("got %d", w.Code)
	}
	if len(pages.got.Body) != siteserve_usecase.MaxBeaconBytes+1 || !strings.HasPrefix(string(pages.got.Body), `{"event":"view"}`) {
		t.Fatalf("the body is read up to one byte past the limit, got %d", len(pages.got.Body))
	}
	form := url.Values{"password": {"sorriso"}, "next": {"/promo"}}
	r = httptest.NewRequest(http.MethodPost, "http://a.paginas.test"+siteserve_usecase.PasswordPath, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	pages.outcome = siteserve_usecase.Outcome{Kind: siteserve_usecase.KindRedirect, Location: "/promo", Status: http.StatusSeeOther, Cookies: []siteserve_usecase.Cookie{{Name: siteserve_usecase.PassCookie, Value: "1.ab", Path: "/", MaxAge: time.Hour, HTTPOnly: true}}}
	w := serve(t, pages, r)
	if pages.got.Password != "sorriso" || pages.got.Next != "/promo" || w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/promo" || !strings.Contains(w.Header().Get("Set-Cookie"), "HttpOnly") {
		t.Fatalf("got %+v %d %v", pages.got, w.Code, w.Header())
	}
}

func TestGeneratedPagesAreLockedDown(t *testing.T) {
	cases := []struct {
		outcome siteserve_usecase.Outcome
		status  int
		text    string
	}{
		{siteserve_usecase.Outcome{Kind: siteserve_usecase.KindNotFound}, http.StatusNotFound, "Página não encontrada"},
		{siteserve_usecase.Outcome{Kind: siteserve_usecase.KindUnavailable}, http.StatusServiceUnavailable, "Serviço indisponível"},
		{siteserve_usecase.Outcome{Kind: siteserve_usecase.KindSuspended}, http.StatusServiceUnavailable, "Página indisponível"},
		{siteserve_usecase.Outcome{Kind: siteserve_usecase.KindPasswordPrompt, Prompt: siteserve_usecase.Prompt{Next: `/a"><script>`, Failed: true}}, http.StatusUnauthorized, "Senha incorreta"},
		{siteserve_usecase.Outcome{Kind: siteserve_usecase.KindTooManyRequests}, http.StatusTooManyRequests, "Muitas"},
		{siteserve_usecase.Outcome{Kind: siteserve_usecase.KindBadRequest}, http.StatusBadRequest, "inválid"},
		{siteserve_usecase.Outcome{Kind: siteserve_usecase.KindPayloadTooLarge}, http.StatusRequestEntityTooLarge, "grande"},
		{siteserve_usecase.Outcome{Kind: siteserve_usecase.KindMethodNotAllowed, Allow: []string{"GET", "HEAD"}}, http.StatusMethodNotAllowed, "Método"},
		{siteserve_usecase.Outcome{Kind: siteserve_usecase.KindRedirect, Location: "/", Status: http.StatusMovedPermanently}, http.StatusMovedPermanently, ""},
	}
	for _, c := range cases {
		pages := &fakePages{outcome: c.outcome}
		w := serve(t, pages, httptest.NewRequest(http.MethodGet, "http://a.paginas.test/", nil))
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.text) {
			t.Fatalf("%v: got %d %s", c.outcome.Kind, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "<script>") {
			t.Fatalf("the next path is escaped, got %s", w.Body.String())
		}
		if c.outcome.Kind != siteserve_usecase.KindRedirect && (w.Header().Get("X-Frame-Options") != "DENY" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") || w.Header().Get("Cache-Control") != "no-store") {
			t.Fatalf("%v: got %v", c.outcome.Kind, w.Header())
		}
	}
	w := serve(t, &fakePages{outcome: siteserve_usecase.Outcome{Kind: siteserve_usecase.KindMethodNotAllowed, Allow: []string{"GET", "HEAD"}}}, httptest.NewRequest(http.MethodPut, "http://a.paginas.test/", nil))
	if w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("got %v", w.Header())
	}
}
