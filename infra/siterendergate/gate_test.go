package siterendergate

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPreviewRuleAllowsOnlyThePreviewDomains(t *testing.T) {
	allow := PreviewRule([]string{"Preview.Test.", "vendor.test"})
	cases := map[string]bool{
		"preview.test":           true,
		"abc.preview.test":       true,
		"a.b.preview.test":       true,
		"ABC.PREVIEW.TEST":       true,
		"cdn.vendor.test":        true,
		"xpreview.test":          false,
		"preview.test.evil.test": false,
		"evil.test":              false,
		"":                       false,
		"127.0.0.1":              false,
	}
	for host, want := range cases {
		if got := allow(host); got != want {
			t.Fatalf("allow(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestPreviewRuleWithoutDomainsAllowsNothing(t *testing.T) {
	if PreviewRule(nil)("preview.test") || PreviewRule([]string{" "})("x") {
		t.Fatalf("an empty rule allowed a host")
	}
}

func upstream(t *testing.T, handler http.Handler) (string, func(ctx context.Context, network, address string) (net.Conn, error)) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	target := strings.TrimPrefix(server.URL, "http://")
	return target, func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, target)
	}
}

func proxyClient(proxyURL string) *http.Client {
	parsed, _ := url.Parse(proxyURL)
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(parsed)}}
}

func TestProxyForwardsPlainRequestsToAllowedHosts(t *testing.T) {
	_, dial := upstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" || r.Header.Get("Proxy-Connection") != "" {
			t.Errorf("hop by hop headers reached the upstream: %v", r.Header)
		}
		fmt.Fprintf(w, "hello %s %s", r.Host, r.URL.Path)
	}))
	gate := httptest.NewServer(&Proxy{Allow: PreviewRule([]string{"preview.test"}), Dial: dial})
	defer gate.Close()
	req, _ := http.NewRequest(http.MethodGet, "http://abc.preview.test/page", nil)
	req.Header.Set("Proxy-Connection", "keep-alive")
	resp, err := proxyClient(gate.URL).Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "hello abc.preview.test /page" {
		t.Fatalf("status %d body %q", resp.StatusCode, body)
	}
}

func TestRefusalHeaderCarriesNoBrand(t *testing.T) {
	if RefusalHeader != "X-Site-Gateway" {
		t.Fatalf("refusal header %q", RefusalHeader)
	}
}

func TestProxyRefusesHostsAndPortsOutsideTheRule(t *testing.T) {
	_, dial := upstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("a refused request reached the upstream")
	}))
	gate := httptest.NewServer(&Proxy{Allow: PreviewRule([]string{"preview.test"}), Dial: dial})
	defer gate.Close()
	for _, target := range []string{"http://evil.test/", "http://abc.preview.test:8080/", "http://169.254.169.254/latest/meta-data"} {
		resp, err := proxyClient(gate.URL).Get(target)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden || resp.Header.Get(RefusalHeader) != "refused" {
			t.Fatalf("%s: status %d header %q, want 403 refused", target, resp.StatusCode, resp.Header.Get(RefusalHeader))
		}
	}
}

func connect(t *testing.T, gate, target string) (net.Conn, *http.Response) {
	t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(gate, "http://"))
	if err != nil {
		t.Fatalf("dial gate: %v", err)
	}
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	return conn, resp
}

func TestProxyTunnelsConnectToAllowedHostsOnly(t *testing.T) {
	_, dial := upstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "tunnelled")
	}))
	gate := httptest.NewServer(&Proxy{Allow: PreviewRule([]string{"preview.test"}), Dial: dial})
	defer gate.Close()

	conn, resp := connect(t, gate.URL, "abc.preview.test:443")
	defer conn.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("connect status %d", resp.StatusCode)
	}
	fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: abc.preview.test\r\nConnection: close\r\n\r\n")
	reply, err := io.ReadAll(conn)
	if err != nil || !strings.Contains(string(reply), "tunnelled") {
		t.Fatalf("tunnel reply %q %v", reply, err)
	}

	for _, target := range []string{"evil.test:443", "abc.preview.test:22"} {
		refused, resp := connect(t, gate.URL, target)
		refused.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s: status %d, want 403", target, resp.StatusCode)
		}
	}
}

func TestPublicProxyRefusesPrivateAddressesWhenDialling(t *testing.T) {
	gate := httptest.NewServer(NewPublic())
	defer gate.Close()
	conn, resp := connect(t, gate.URL, "127.0.0.1:443")
	conn.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", resp.StatusCode)
	}
}

func TestProxyRejectsOriginFormRequests(t *testing.T) {
	gate := httptest.NewServer(NewPreview([]string{"preview.test"}, ""))
	defer gate.Close()
	resp, err := http.Get(gate.URL + "/json/version")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestLoopbackPreviewHostsGoToTheConfiguredAPIOnly(t *testing.T) {
	target, _ := upstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "api %s %s", r.Host, r.URL.Path)
	}))
	never := func(context.Context, string, string) (net.Conn, error) {
		t.Errorf("a loopback preview host was dialled through the guarded dialer")
		return nil, fmt.Errorf("guarded")
	}
	gate := httptest.NewServer(&Proxy{Allow: PreviewRule([]string{"previa.localhost"}), Dial: never, Loopback: target})
	defer gate.Close()
	resp, err := proxyClient(gate.URL).Get("http://abc.previa.localhost:4000/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "api abc.previa.localhost /" {
		t.Fatalf("status %d body %q", resp.StatusCode, body)
	}

	closed := httptest.NewServer(&Proxy{Allow: PreviewRule([]string{"previa.localhost"}), Dial: never})
	defer closed.Close()
	refused, err := proxyClient(closed.URL).Get("http://abc.previa.localhost:4000/")
	if err != nil {
		t.Fatal(err)
	}
	refused.Body.Close()
	if refused.StatusCode != http.StatusForbidden {
		t.Fatalf("without an API address loopback previews are refused, got %d", refused.StatusCode)
	}
}

func TestNewPreviewKeepsTheLoopbackAddress(t *testing.T) {
	if p := NewPreview([]string{"previa.localhost"}, "host.docker.internal:4000"); p.Loopback != "host.docker.internal:4000" || !p.Allow("abc.previa.localhost") {
		t.Fatalf("got %+v", p)
	}
}
