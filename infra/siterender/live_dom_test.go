package siterender

import (
	"strings"
	"testing"

	"github.com/chromedp/cdproto/cdp"

	"vozko/domain/site"
	"vozko/domain/sitehtml"
)

func element(name string, attrs []string, children ...*cdp.Node) *cdp.Node {
	return &cdp.Node{NodeType: cdp.NodeTypeElement, NodeName: strings.ToUpper(name), LocalName: name, Attributes: attrs, Children: children}
}

func text(value string) *cdp.Node {
	return &cdp.Node{NodeType: cdp.NodeTypeText, NodeName: "#text", NodeValue: value}
}

func liveTree() *cdp.Node {
	host := element("vz-card", nil)
	host.ShadowRoots = []*cdp.Node{{NodeType: cdp.NodeTypeDocumentFragment, NodeName: "#document-fragment", ShadowRootType: cdp.ShadowRootTypeClosed, Children: []*cdp.Node{
		element("input", []string{"type", "password", "name", "senha"}),
	}}}
	frame := element("iframe", []string{"src", "/x"})
	frame.ContentDocument = &cdp.Node{NodeType: cdp.NodeTypeDocument, Children: []*cdp.Node{element("html", nil, element("body", nil, element("input", []string{"autocomplete", "cc-number"})))}}
	body := element("body", nil,
		element("h1", []string{"class", `a"b`}, text("Bolos & <doces>")),
		&cdp.Node{NodeType: cdp.NodeTypeComment, NodeName: "#comment", NodeValue: "-->escape"},
		element("script", nil, text(`if (a < b && c) {}`)),
		element("br", nil),
		host,
		frame,
	)
	return &cdp.Node{NodeType: cdp.NodeTypeDocument, NodeName: "#document", Children: []*cdp.Node{
		{NodeType: cdp.NodeTypeDocumentType, NodeName: "html"},
		element("html", []string{"lang", "pt-BR"}, element("head", nil, element("title", nil, text("Oi"))), body),
	}}
}

func TestSerializeDocumentWritesParseableHTMLWithHiddenTrees(t *testing.T) {
	got := serializeDocument(liveTree())
	if !strings.HasPrefix(got, "<!doctype html>") {
		t.Fatalf("no doctype: %s", got)
	}
	for _, want := range []string{
		`<h1 class="a&quot;b">Bolos &amp; &lt;doces&gt;</h1>`,
		`<script>if (a < b && c) {}</script>`,
		`<br>`,
		`<template shadowrootmode="closed"><input type="password" name="senha"></template>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, "escape") || strings.Contains(got, "</br>") {
		t.Fatalf("comments or void closers leaked: %s", got)
	}
	doc, err := sitehtml.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, got)
	}
	issues := site.CheckPages([]site.RenderedPage{{Route: "/", Document: doc}}).Issues
	credentials := 0
	for _, issue := range issues {
		if issue.Code == site.CodeCredentialFieldForbidden {
			credentials++
		}
	}
	if credentials != 2 {
		t.Fatalf("the closed shadow root and the frame document must be scanned, issues %+v", issues)
	}
}
