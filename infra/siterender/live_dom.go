package siterender

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"

	renderuc "vozko/usecases/siterender"
)

const (
	networkQuiet     = 500 * time.Millisecond
	networkMaxWait   = 10 * time.Second
	scrollQuietLimit = 3 * time.Second
	liveSettle       = 300 * time.Millisecond
)

var rawTextElements = map[string]bool{"script": true, "style": true, "xmp": true, "noembed": true, "noframes": true, "plaintext": true}

var documentWrappers = map[string]bool{"html": true, "head": true, "body": true}

var htmlVoidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true,
}

func (s *session) waitNetworkIdle(quiet, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	var calmSince time.Time
	for {
		s.mu.Lock()
		busy := len(s.inflight)
		s.mu.Unlock()
		now := time.Now()
		if busy > 0 {
			calmSince = time.Time{}
		} else if calmSince.IsZero() {
			calmSince = now
		}
		if busy == 0 && now.Sub(calmSince) >= quiet {
			return nil
		}
		if now.After(deadline) {
			return nil
		}
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (s *session) liveDocument() (string, error) {
	var root *cdp.Node
	err := s.run(func(ctx context.Context) error {
		var err error
		root, err = dom.GetDocument().WithDepth(-1).WithPierce(true).Do(ctx)
		return err
	})
	if err != nil {
		return "", err
	}
	if root == nil {
		return "", fmt.Errorf("siterender: the page has no document")
	}
	return serializeDocument(root), nil
}

func (c *Client) LiveDOM(ctx context.Context, job renderuc.LiveDOMJob) (map[string]string, error) {
	s, done, err := c.open(ctx, "", job.Policy, renderuc.ErrURLRefused)
	if err != nil {
		return nil, err
	}
	defer done()
	out := make(map[string]string, len(job.Routes))
	for _, route := range job.Routes {
		target, err := renderuc.LiveRouteURL(job.URL, route)
		if err != nil {
			return nil, err
		}
		if err := s.navigate(target, renderuc.Desktop); err != nil {
			return nil, fmt.Errorf("route %s: %w", route, err)
		}
		if err := s.waitNetworkIdle(networkQuiet, networkMaxWait); err != nil {
			return nil, err
		}
		if err := s.settle(liveSettle); err != nil {
			return nil, fmt.Errorf("route %s: %w", route, err)
		}
		if err := s.probe("scrollOnce()", nil); err != nil {
			return nil, fmt.Errorf("route %s: %w", route, err)
		}
		if err := s.waitNetworkIdle(networkQuiet, scrollQuietLimit); err != nil {
			return nil, err
		}
		document, err := s.liveDocument()
		if err != nil {
			return nil, fmt.Errorf("route %s: %w", route, err)
		}
		out[route] = document
	}
	return out, nil
}

func serializeDocument(root *cdp.Node) string {
	var b strings.Builder
	b.WriteString("<!doctype html>")
	for _, child := range root.Children {
		writeNode(&b, child, false, false)
	}
	return b.String()
}

func elementName(n *cdp.Node) string {
	if n.LocalName != "" {
		return n.LocalName
	}
	return strings.ToLower(n.NodeName)
}

func writeNode(b *strings.Builder, n *cdp.Node, raw, unwrap bool) {
	switch n.NodeType {
	case cdp.NodeTypeText, cdp.NodeTypeCDATA:
		if raw {
			b.WriteString(n.NodeValue)
			return
		}
		b.WriteString(escapeText(n.NodeValue))
	case cdp.NodeTypeElement:
		writeElement(b, n, unwrap)
	case cdp.NodeTypeDocument, cdp.NodeTypeDocumentFragment:
		for _, child := range n.Children {
			writeNode(b, child, false, unwrap)
		}
	}
}

func writeElement(b *strings.Builder, n *cdp.Node, unwrap bool) {
	name := elementName(n)
	if unwrap && documentWrappers[name] {
		for _, child := range n.Children {
			writeNode(b, child, false, true)
		}
		return
	}
	b.WriteString("<" + name)
	for i := 0; i+1 < len(n.Attributes); i += 2 {
		b.WriteString(" " + n.Attributes[i] + `="` + escapeAttr(n.Attributes[i+1]) + `"`)
	}
	b.WriteString(">")
	if htmlVoidElements[name] {
		return
	}
	for _, shadow := range n.ShadowRoots {
		if shadow.ShadowRootType == cdp.ShadowRootTypeUserAgent {
			continue
		}
		mode := "closed"
		if shadow.ShadowRootType == cdp.ShadowRootTypeOpen {
			mode = "open"
		}
		b.WriteString(`<template shadowrootmode="` + mode + `">`)
		for _, child := range shadow.Children {
			writeNode(b, child, false, false)
		}
		b.WriteString("</template>")
	}
	if n.TemplateContent != nil {
		for _, child := range n.TemplateContent.Children {
			writeNode(b, child, false, false)
		}
	}
	raw := rawTextElements[name]
	for _, child := range n.Children {
		writeNode(b, child, raw, false)
	}
	b.WriteString("</" + name + ">")
	if n.ContentDocument != nil {
		b.WriteString(`<template data-vz-frame="">`)
		writeNode(b, n.ContentDocument, false, true)
		b.WriteString("</template>")
	}
}

func escapeText(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(value)
}

func escapeAttr(value string) string {
	return strings.NewReplacer("&", "&amp;", `"`, "&quot;").Replace(value)
}
