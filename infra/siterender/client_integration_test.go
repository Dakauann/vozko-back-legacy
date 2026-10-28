package siterender

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vozko/infra/siterendergate"
	renderuc "vozko/usecases/siterender"
)

const probePage = `<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Probe</title>
<style>
body { margin: 0; font-family: Arial, sans-serif; background: #ffffff; color: #111111 }
section { height: 1000px; padding: 24px }
.low { color: #bbbbbb }
.wide { width: 600px; height: 40px; background: #eeeeee }
</style></head>
<body>
<header data-vz-node="index.html#10"><h1 data-vz-node="index.html#20">Titulo da pagina</h1></header>
<section data-vz-node="index.html#30">
  <p class="low" data-vz-node="index.html#40">Texto claro demais</p>
  <button data-vz-node="index.html#50"></button>
  <div class="wide" data-vz-node="index.html#60"></div>
</section>
<section data-vz-node="index.html#70">
  <img data-vz-node="index.html#80" src="/missing.png" alt="foto" width="200" height="100">
  <script src="http://blocked.invalid/blocked.js"></script>
</section>
<section data-vz-node="index.html#90"><p data-vz-node="index.html#95">Fim</p></section>
<script>console.error("probe console error")</script>
</body></html>`

const prerenderPage = `<!doctype html><html><head><meta charset="utf-8"></head><body>
<script>window.__vzPrerender = async (route) => "<main>" + route + "</main>";</script>
</body></html>`

const referencePage = `<!doctype html><html><head><meta charset="utf-8"><title>Referencia</title>
<style>
body { margin: 0; background: #fafafa; font-family: Georgia, serif; color: #222222 }
section { background: #1d4ed8; color: #ffffff; padding: 32px 24px; margin-bottom: 16px; height: 600px }
p { margin: 8px 0; padding: 8px }
</style></head><body>
<section><p>Primeira dobra com bastante texto para pesar</p><p>Mais texto</p></section>
<section><p>Segunda parte</p></section>
</body></html>`

type hostPolicy struct{ hosts map[string]bool }

func (p hostPolicy) Allow(_ context.Context, host string) bool {
	return p.hosts[host]
}

type rig struct {
	client  *Client
	baseURL string
	policy  hostPolicy
}

func chromiumExecutable() string {
	if custom := strings.TrimSpace(os.Getenv("CHROMIUM_PATH")); custom != "" {
		return custom
	}
	for _, candidate := range []string{
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
		"C:/Program Files/Google/Chrome/Application/chrome.exe",
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func localDevTools(t *testing.T) string {
	t.Helper()
	executable := chromiumExecutable()
	if executable == "" {
		t.Skip("no chromium executable on this machine; skipping the renderer integration test")
	}
	profile := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, executable,
		"--headless=new",
		"--remote-debugging-port=0",
		"--user-data-dir="+profile,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-gpu",
		"--hide-scrollbars",
		"about:blank",
	)
	if err := cmd.Start(); err != nil {
		cancel()
		t.Skipf("chromium did not start: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
	portFile := filepath.Join(profile, "DevToolsActivePort")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if file, err := os.Open(portFile); err == nil {
			scanner := bufio.NewScanner(file)
			if scanner.Scan() && strings.TrimSpace(scanner.Text()) != "" {
				port := strings.TrimSpace(scanner.Text())
				_ = file.Close()
				return "http://127.0.0.1:" + port
			}
			_ = file.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Skip("chromium did not open a DevTools port")
	return ""
}

func newRig(t *testing.T) rig {
	t.Helper()
	endpoint := localDevTools(t)
	pageHost := "127.0.0.1"
	listener, err := net.Listen("tcp", pageHost+":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mux := http.NewServeMux()
	serve := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(body))
		}
	}
	mux.HandleFunc("/{$}", serve(probePage))
	mux.HandleFunc("/__vz/prerender.html", serve(prerenderPage))
	mux.HandleFunc("/ref", serve(referencePage))
	mux.HandleFunc("/gone", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "gone", http.StatusNotFound) })
	mux.HandleFunc("/refused", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(siterendergate.RefusalHeader, "refused")
		http.Error(w, "destination not allowed", http.StatusForbidden)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	port := listener.Addr().(*net.TCPAddr).Port
	return rig{
		client:  relayedClient(t, endpoint),
		baseURL: fmt.Sprintf("http://%s:%d", pageHost, port),
		policy:  hostPolicy{hosts: map[string]bool{pageHost: true}},
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestRendererPrerendersEveryRoute(t *testing.T) {
	r := newRig(t)
	got, err := r.client.Prerender(testContext(t), renderuc.PrerenderJob{URL: r.baseURL + "/__vz/prerender.html", Routes: []string{"/", "/precos"}, Policy: r.policy})
	if err != nil {
		t.Fatalf("prerender: %v", err)
	}
	if got["/"] != "<main>/</main>" || got["/precos"] != "<main>/precos</main>" {
		t.Fatalf("prerendered %v", got)
	}
}

func TestRendererRefusesAPageOutsideThePolicy(t *testing.T) {
	r := newRig(t)
	_, err := r.client.Screenshot(testContext(t), renderuc.ScreenshotJob{URL: r.baseURL + "/", Viewport: renderuc.OpenGraph, Policy: hostPolicy{hosts: map[string]bool{}}})
	if !errors.Is(err, renderuc.ErrURLRefused) {
		t.Fatalf("err %v", err)
	}
}

func TestRendererTakesAnOpenGraphScreenshot(t *testing.T) {
	r := newRig(t)
	data, err := r.client.Screenshot(testContext(t), renderuc.ScreenshotJob{URL: r.baseURL + "/", Viewport: renderuc.OpenGraph, Policy: r.policy})
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 630 {
		t.Fatalf("screenshot %v %v", img.Bounds(), err)
	}
}

func TestRendererCapturesAReferenceWithTokens(t *testing.T) {
	r := newRig(t)
	captured, err := r.client.Reference(testContext(t), renderuc.ReferenceJob{
		URL: r.baseURL + "/ref", Policy: r.policy, Viewports: renderuc.DefaultViewports(), MaxSlices: 4,
	})
	if err != nil {
		t.Fatalf("reference: %v", err)
	}
	if captured.Title != "Referencia" || captured.URL != r.baseURL+"/ref" || len(captured.Viewports) != 2 {
		t.Fatalf("reference %q %q %d", captured.Title, captured.URL, len(captured.Viewports))
	}
	tokens := renderuc.ExtractTokens(captured.Samples)
	blue := false
	for _, c := range tokens.Backgrounds {
		if c.Hex == "#1d4ed8" {
			blue = true
		}
	}
	if !blue || len(tokens.Fonts) == 0 || tokens.Fonts[0].Family != "Georgia" || tokens.Spacing.Base != 8 {
		t.Fatalf("tokens %+v", tokens)
	}
}

func TestRendererFailsOnAnErrorPageAndCodesAGatewayRefusal(t *testing.T) {
	r := newRig(t)
	_, err := r.client.Screenshot(testContext(t), renderuc.ScreenshotJob{URL: r.baseURL + "/gone", Viewport: renderuc.OpenGraph, Policy: r.policy})
	if err == nil || errors.Is(err, renderuc.ErrURLRefused) {
		t.Fatalf("error page: %v", err)
	}
	_, err = r.client.Reference(testContext(t), renderuc.ReferenceJob{URL: r.baseURL + "/refused", Policy: r.policy, Viewports: []renderuc.Viewport{renderuc.Desktop}, MaxSlices: 1})
	if !errors.Is(err, renderuc.ErrReferenceRefused) {
		t.Fatalf("gateway refusal: %v", err)
	}
}

const livePage = `<!doctype html><html><head><meta charset="utf-8"><title>Ao vivo</title></head>
<body style="margin:0"><main id="root"><h1>Entrar</h1></main><div style="height:3000px"></div><footer id="end">fim</footer>
<script>
setTimeout(async () => {
  const res = await fetch("/l/data");
  const text = await res.text();
  const field = document.createElement("input");
  field.type = text;
  field.name = "senha";
  document.getElementById("root").appendChild(field);
}, 150);
let sent = false;
addEventListener("scroll", () => {
  if (sent || scrollY < 500) return;
  sent = true;
  setTimeout(() => {
    const form = document.createElement("form");
    form.action = "https://evil.test/coleta";
    document.getElementById("end").appendChild(form);
  }, 100);
});
</script></body></html>`

const liveSecondPage = `<!doctype html><html><head><meta charset="utf-8"><title>Dois</title></head><body><p id="two">segunda</p>
<script>document.getElementById("two").setAttribute("data-ran", "sim");</script></body></html>`

func TestRendererReturnsTheDocumentAfterScriptsNetworkAndOneScroll(t *testing.T) {
	r := newRig(t)
	mux := http.NewServeMux()
	serve := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(body))
		}
	}
	mux.HandleFunc("/l/{$}", serve(livePage))
	mux.HandleFunc("/l/dois", serve(liveSecondPage))
	mux.HandleFunc("/l/data", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte("password"))
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	base := fmt.Sprintf("http://127.0.0.1:%d/l/", listener.Addr().(*net.TCPAddr).Port)
	got, err := r.client.LiveDOM(testContext(t), renderuc.LiveDOMJob{URL: base, Routes: []string{"/", "/dois"}, Policy: r.policy})
	if err != nil {
		t.Fatalf("live dom: %v", err)
	}
	home := got["/"]
	if !strings.HasPrefix(strings.ToLower(home), "<!doctype html>") || !strings.Contains(home, `type="password"`) || !strings.Contains(home, `action="https://evil.test/coleta"`) {
		t.Fatalf("home document misses the late nodes:\n%s", home)
	}
	if !strings.Contains(got["/dois"], `data-ran="sim"`) {
		t.Fatalf("second route %s", got["/dois"])
	}
}

func relayedClient(t *testing.T, devtools string) *Client {
	t.Helper()
	token := strings.Repeat("z", 40)
	relay, err := siterendergate.NewRelay(strings.TrimPrefix(devtools, "http://"), token, 8)
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = relay.Serve(ctx, listener) }()
	client, err := New(Config{Endpoint: "http://" + listener.Addr().String(), Token: token})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return client
}
