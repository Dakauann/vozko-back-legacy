package sitebuilder

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"vozko/domain/site"
	"vozko/usecases/sitebuild"
)

func TestExtractSendsTheNodeAndDecodesTheSource(t *testing.T) {
	client, seen := server(t, 200, `{"source":"<h1>Oi</h1>"}`)
	out, err := client.Extract(context.Background(), sitebuild.ExtractRequest{Project: project(), Node: "pages/index.html#0", Rev: 1, Bases: map[string][]site.File{"pages/index.html": {{Content: "<h1>Antes</h1>", Rev: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != "<h1>Oi</h1>" || out.Failure != nil {
		t.Fatalf("got %+v", out)
	}
	bases, ok := seen.body["bases"].(map[string]any)
	if !ok || len(bases["pages/index.html"].([]any)) != 1 {
		t.Fatalf("the older revisions travel with the extraction, got %+v", seen.body)
	}
	if seen.path != "/extract" || seen.method != http.MethodPost || seen.body["node"] != "pages/index.html#0" || seen.body["rev"] != float64(1) || seen.body["project"] == nil {
		t.Fatalf("got %+v", seen)
	}
}

func TestExtractDecodesAFailureWithAKnownCode(t *testing.T) {
	client, _ := server(t, 200, `{"failure":{"code":"node_stale","field":"node"}}`)
	out, err := client.Extract(context.Background(), sitebuild.ExtractRequest{Project: project(), Node: "pages/index.html#9", Rev: 1})
	if err != nil {
		t.Fatal(err)
	}
	if out.Failure == nil || out.Failure.Code != site.CodeNodeStale || out.Failure.Field != "node" || out.Source != "" {
		t.Fatalf("got %+v", out)
	}
}

func TestExtractRefusesMalformedReplies(t *testing.T) {
	for _, reply := range []string{
		`{}`,
		`{"source":""}`,
		`{"source":"<h1>Oi</h1>","failure":{"code":"node_stale"}}`,
		`{"failure":{"code":"made_up"}}`,
		`{"source":"<h1>Oi</h1>","extra":1}`,
	} {
		client, _ := server(t, 200, reply)
		if _, err := client.Extract(context.Background(), sitebuild.ExtractRequest{Project: project(), Node: "pages/index.html#0", Rev: 1}); !errors.Is(err, sitebuild.ErrBuilderReply) {
			t.Fatalf("%s: got %v", reply, err)
		}
	}
}

func TestExecuteSendsJSXSections(t *testing.T) {
	client, seen := server(t, 200, `{"steps":[{"op":"set_text","index":0}],"splices":[]}`)
	req := executeRequest()
	req.JSXSections = map[string]string{"snippet-card": "<div />"}
	if _, err := client.Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	jsx, ok := seen.body["jsx_sections"].(map[string]any)
	if !ok || jsx["snippet-card"] != "<div />" {
		t.Fatalf("got %+v", seen.body)
	}
}
