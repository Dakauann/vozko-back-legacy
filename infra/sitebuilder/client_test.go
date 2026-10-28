package sitebuilder

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vozko/domain/inlinemarkup"
	"vozko/domain/site"
	"vozko/usecases/sitebuild"
)

const testToken = "0123456789abcdef0123456789abcdef-token"

type recorded struct {
	path   string
	auth   string
	ctype  string
	body   map[string]any
	method string
}

func server(t *testing.T, status int, reply string) (*Client, *recorded) {
	t.Helper()
	seen := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path, seen.auth, seen.ctype, seen.method = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &seen.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	client, err := New(Config{BaseURL: srv.URL, Token: testToken})
	if err != nil {
		t.Fatal(err)
	}
	return client, seen
}

func project() site.Project {
	return site.NewProject(site.StackHTML, map[string]string{"site.json": "{}", "pages/index.html": "<h1>Oi</h1>\n"})
}

func executeRequest() sitebuild.ExecuteRequest {
	text := "Olá"
	return sitebuild.ExecuteRequest{
		Project:    project(),
		Operations: []site.Operation{{Op: site.OpSetText, Node: "pages/index.html#0", Rev: 1, Text: &text}},
		Texts:      map[int][]inlinemarkup.Run{0: {{Text: "Olá"}}},
	}
}

func TestNewRefusesAMissingURLOrToken(t *testing.T) {
	for _, cfg := range []Config{{Token: testToken}, {BaseURL: "http://x"}, {BaseURL: "ftp://x", Token: testToken}} {
		if _, err := New(cfg); err == nil {
			t.Fatalf("%+v: expected a refusal", cfg)
		}
	}
}

func TestExecuteSendsTheWireContractAndDecodesSteps(t *testing.T) {
	client, seen := server(t, 200, `{"steps":[{"op":"set_text","index":0,"focus":"pages/index.html#0","files":{"pages/index.html":"<h1>Olá</h1>\n"}}],"splices":[{"index":0,"path":"pages/index.html","start":4,"end":6,"length":3}]}`)
	out, err := client.Execute(context.Background(), executeRequest())
	if err != nil {
		t.Fatal(err)
	}
	if seen.path != "/execute" || seen.method != http.MethodPost || seen.auth != "Bearer "+testToken || !strings.HasPrefix(seen.ctype, "application/json") {
		t.Fatalf("got %+v", seen)
	}
	for _, key := range []string{"project", "operations", "texts", "media", "sections", "bases"} {
		if _, ok := seen.body[key]; !ok {
			t.Fatalf("missing %s in %+v", key, seen.body)
		}
	}
	if seen.body["media"] == nil || seen.body["bases"] == nil {
		t.Fatal("empty maps must be sent as objects")
	}
	texts := seen.body["texts"].(map[string]any)["0"].([]any)
	if texts[0].(map[string]any)["text"] != "Olá" {
		t.Fatalf("got %+v", seen.body["texts"])
	}
	if len(out.Steps) != 1 || out.Steps[0].Files["pages/index.html"] != "<h1>Olá</h1>\n" || out.Splices[0].Length != 3 {
		t.Fatalf("got %+v", out)
	}
}

func TestExecuteDecodesRefusals(t *testing.T) {
	client, _ := server(t, 200, `{"failures":[{"index":0,"code":"edit_not_found","field":"old","detail":"nearest match at line 1"}],"skipped":[]}`)
	out, err := client.Execute(context.Background(), executeRequest())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Failures) != 1 || out.Failures[0].Detail != "nearest match at line 1" || len(out.SiteFailures()) != 1 || out.SiteFailures()[0].Detail != "nearest match at line 1" {
		t.Fatalf("got %+v", out)
	}
}

func TestExecuteFailsClosedOnMalformedReplies(t *testing.T) {
	replies := []string{
		`{}`,
		`not json`,
		`{"steps":[],"failures":[]}`,
		`{"steps":[{"op":"paint","index":0}]}`,
		`{"steps":[{"op":"set_text","index":3}]}`,
		`{"steps":[{"op":"set_text","index":0,"extra":1}]}`,
		`{"failures":[{"index":0,"code":"made_up"}]}`,
		`{"failures":[{"index":9,"code":"node_stale"}]}`,
		`{"steps":[{"op":"set_text","index":0}],"failures":[{"index":0,"code":"node_stale"}]}`,
		`{"steps":[{"op":"set_text","index":0}]} {}`,
	}
	for _, reply := range replies {
		client, _ := server(t, 200, reply)
		if _, err := client.Execute(context.Background(), executeRequest()); !errors.Is(err, sitebuild.ErrBuilderReply) {
			t.Fatalf("%s: got %v", reply, err)
		}
	}
}

func TestStatusesMapToTypedErrors(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{400, sitebuild.ErrBuilderRefused},
		{422, sitebuild.ErrBuilderRefused},
		{401, sitebuild.ErrBuilderUnavailable},
		{500, sitebuild.ErrBuilderUnavailable},
		{503, sitebuild.ErrBuilderBusy},
		{504, sitebuild.ErrBuilderTimeout},
	}
	for _, c := range cases {
		client, _ := server(t, c.status, `{"code":"x","message":"why"}`)
		_, err := client.Build(context.Background(), sitebuild.BuildRequest{Mode: sitebuild.ModePreview, Project: project()})
		var typed *sitebuild.BuilderError
		if !errors.Is(err, c.want) || !errors.As(err, &typed) || typed.Status != c.status || typed.Message != "why" {
			t.Fatalf("%d: got %v", c.status, err)
		}
	}
}

func TestUnreachableBuilderIsUnavailable(t *testing.T) {
	client, err := New(Config{BaseURL: "http://127.0.0.1:1", Token: testToken})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Execute(context.Background(), executeRequest()); !errors.Is(err, sitebuild.ErrBuilderUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestTimeoutsAreEnforced(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)
	client, err := New(Config{BaseURL: srv.URL, Token: testToken, ExecuteTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Execute(context.Background(), executeRequest()); !errors.Is(err, sitebuild.ErrBuilderTimeout) {
		t.Fatalf("got %v", err)
	}
}

const goodBuild = `{"assets":[{"path":"index.html","content_type":"text/html; charset=utf-8","body":"PGgxPk9pPC9oMT4="},{"path":"__vz/prerender.html","content_type":"text/html; charset=utf-8","body":""}],"imports":[{"importer":"scripts/a.js","specifier":"gsap"}],"routes":[{"route":"/","html_path":"index.html","interactive":true}],"errors":[],"warnings":[{"path":"src/a.jsx","line":2,"column":3,"code":"class_dynamic","message":"m"}],"prerender_path":"__vz/prerender.html"}`

func TestBuildSendsTheShellAndDecodesAssets(t *testing.T) {
	client, seen := server(t, 200, goodBuild)
	out, err := client.Build(context.Background(), sitebuild.BuildRequest{
		Mode:    sitebuild.ModePublish,
		Project: project(),
		Shell:   sitebuild.Shell{API: "https://api.vozko.test", Pages: map[string]sitebuild.PageMeta{"/": {Title: "x", NoIndex: true}}},
		Runtime: map[string]string{"pow.js": "p"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen.path != "/build" || seen.body["mode"] != "publish" {
		t.Fatalf("got %+v", seen)
	}
	shell := seen.body["shell"].(map[string]any)
	if shell["api"] != "https://api.vozko.test" || shell["forms"] == nil || shell["integrations"] == nil {
		t.Fatalf("got %+v", shell)
	}
	if string(out.Assets[0].Body) != "<h1>Oi</h1>" || out.Routes[0].HTMLPath != "index.html" || !out.Routes[0].Interactive || out.PrerenderPath != "__vz/prerender.html" || out.Warnings[0].Code != "class_dynamic" {
		t.Fatalf("got %+v", out)
	}
}

func TestBuildPassesCompilerErrorsThrough(t *testing.T) {
	client, _ := server(t, 200, `{"assets":[],"imports":[],"routes":[],"errors":[{"path":"src/a.jsx","line":1,"column":2,"message":"boom"}],"warnings":[],"prerender_path":""}`)
	out, err := client.Build(context.Background(), sitebuild.BuildRequest{Mode: sitebuild.ModePreview, Project: project()})
	if err != nil || len(out.Errors) != 1 || out.Errors[0].Message != "boom" {
		t.Fatalf("got %+v %v", out, err)
	}
}

func TestBuildPassesCodedCompilerErrorsThrough(t *testing.T) {
	client, _ := server(t, 200, `{"assets":[],"imports":[],"routes":[],"errors":[{"path":"src/a.jsx","line":1,"column":2,"message":"no","code":"import_dynamic_forbidden"}],"warnings":[],"prerender_path":""}`)
	out, err := client.Build(context.Background(), sitebuild.BuildRequest{Mode: sitebuild.ModePreview, Project: project()})
	if err != nil || len(out.Errors) != 1 || out.Errors[0].Code != site.CodeImportDynamicForbidden {
		t.Fatalf("got %+v %v", out, err)
	}
}

func TestBuildFailsClosedOnMalformedReplies(t *testing.T) {
	replies := []string{
		`{"assets":[],"imports":[],"routes":[],"errors":[],"warnings":[],"prerender_path":""}`,
		`{"assets":[{"path":"../x","content_type":"text/html","body":""}],"imports":[],"routes":[{"route":"/","html_path":"../x","interactive":false}],"errors":[],"warnings":[],"prerender_path":""}`,
		`{"assets":[{"path":"/index.html","content_type":"text/html","body":""}],"imports":[],"routes":[{"route":"/","html_path":"/index.html","interactive":false}],"errors":[],"warnings":[],"prerender_path":""}`,
		`{"assets":[{"path":"index.html","content_type":"","body":""}],"imports":[],"routes":[{"route":"/","html_path":"index.html","interactive":false}],"errors":[],"warnings":[],"prerender_path":""}`,
		`{"assets":[{"path":"index.html","content_type":"text/html","body":""}],"imports":[],"routes":[{"route":"/","html_path":"other.html","interactive":false}],"errors":[],"warnings":[],"prerender_path":""}`,
		`{"assets":[{"path":"index.html","content_type":"text/html","body":""}],"imports":[],"routes":[{"route":"Nope","html_path":"index.html","interactive":false}],"errors":[],"warnings":[],"prerender_path":""}`,
		`{"assets":[{"path":"index.html","content_type":"text/html","body":""}],"imports":[],"routes":[{"route":"/","html_path":"index.html","interactive":false}],"errors":[],"warnings":[],"prerender_path":"missing.html"}`,
		`{"assets":[{"path":"index.html","content_type":"text/html","body":""},{"path":"index.html","content_type":"text/html","body":""}],"imports":[],"routes":[{"route":"/","html_path":"index.html","interactive":false}],"errors":[],"warnings":[],"prerender_path":""}`,
		`{"assets":[{"path":"index.html","content_type":"text/html","body":""}],"imports":[],"routes":[],"errors":[{"message":"x"}],"warnings":[],"prerender_path":""}`,
		`{"assets":[{"path":"index.html","content_type":"text/html","body":"%%%"}],"imports":[],"routes":[{"route":"/","html_path":"index.html","interactive":false}],"errors":[],"warnings":[],"prerender_path":""}`,
		`{"assets":[],"imports":[],"routes":[],"errors":[],"warnings":[],"prerender_path":"","extra":true}`,
		`{"assets":[],"imports":[],"routes":[],"errors":[{"message":"x","code":"not_a_site_code"}],"warnings":[],"prerender_path":""}`,
	}
	for _, reply := range replies {
		client, _ := server(t, 200, reply)
		if _, err := client.Build(context.Background(), sitebuild.BuildRequest{Mode: sitebuild.ModePreview, Project: project()}); !errors.Is(err, sitebuild.ErrBuilderReply) {
			t.Fatalf("%s: got %v", reply, err)
		}
	}
}
