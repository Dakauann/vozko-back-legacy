package site

import (
	"encoding/json"
	"strings"

	"vozko/domain/shared"
	"vozko/domain/siteform"
	"vozko/domain/sitehtml"
)

type RenderedPage struct {
	Route    string
	Document *sitehtml.Document
}

var urlAttrs = map[string]bool{"href": true, "src": true, "action": true, "formaction": true, "poster": true, "xlink:href": true, "data": true, "cite": true, "background": true}

var dataScriptTypes = map[string]bool{"application/ld+json": true, "application/json": true}

var credentialAutocomplete = map[string]bool{"current-password": true, "new-password": true, "one-time-code": true}

var privacyWords = []string{"privacidade", "privacy", "privacidad", "datenschutz"}

type CheckedPages struct {
	Issues    []Issue
	Manifests []siteform.Manifest
	routes    []string
}

func CheckPages(pages []RenderedPage) CheckedPages {
	var out CheckedPages
	firstRoute := map[string]int{}
	for _, p := range pages {
		out.Issues = append(out.Issues, checkElements(p)...)
		found, problems := siteform.Extract(p.Document)
		for _, problem := range problems {
			out.Issues = append(out.Issues, Issue{Code: problem.Code, Route: p.Route, Node: problem.Node, Detail: strings.TrimSpace(problem.Form + " " + problem.Field)})
		}
		for _, m := range found {
			at, seen := firstRoute[m.ID]
			if !seen {
				firstRoute[m.ID] = len(out.Manifests)
				out.Manifests = append(out.Manifests, m)
				out.routes = append(out.routes, p.Route)
				continue
			}
			if !sameContract(out.Manifests[at], m) {
				out.Issues = append(out.Issues, Issue{Code: CodeFormIDDuplicate, Route: p.Route, Node: m.Node, Detail: m.ID})
			}
		}
	}
	return out
}

func sameContract(a, b siteform.Manifest) bool {
	a.Node, b.Node = "", ""
	left, errA := json.Marshal(a)
	right, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(left) == string(right)
}

func checkElements(p RenderedPage) []Issue {
	doc := p.Document
	var issues []Issue
	add := func(code string, i int, detail string) {
		issues = append(issues, Issue{Code: code, Route: p.Route, Node: doc.Elements[i].Node, Detail: detail})
	}
	for i, el := range doc.Elements {
		for _, attr := range el.Attrs {
			if strings.HasPrefix(attr.Name, "on") {
				add(CodeEventHandlerForbidden, i, attr.Name)
				break
			}
		}
		for _, attr := range el.Attrs {
			if urlAttrs[attr.Name] && dangerousURL(el.Tag, attr.Value) {
				add(CodeURLForbidden, i, attr.Name)
			}
		}
		switch el.Tag {
		case "base":
			add(CodeURLForbidden, i, "base")
		case "meta":
			if equiv, _ := doc.AttrValue(i, "http-equiv"); strings.EqualFold(strings.TrimSpace(equiv), "refresh") {
				add(CodeURLForbidden, i, "refresh")
			}
		case "input", "select", "textarea":
			if credentialField(doc, i) {
				add(CodeCredentialFieldForbidden, i, el.Tag)
			}
		case "form":
			if action, ok := doc.AttrValue(i, "action"); ok && externalURL(action) {
				add(CodeFormActionForbidden, i, action)
			}
		case "script":
			issues = append(issues, checkScript(p, i)...)
		case "iframe", "frame":
			if detail, refused := refusedFrame(doc, i); refused {
				add(CodeFrameSourceForbidden, i, detail)
			}
		}
		if action, ok := doc.AttrValue(i, "formaction"); ok && externalURL(action) {
			add(CodeFormActionForbidden, i, action)
		}
	}
	return issues
}

func checkScript(p RenderedPage, i int) []Issue {
	doc := p.Document
	el := doc.Elements[i]
	src, hasSrc := doc.AttrValue(i, "src")
	kind, _ := doc.AttrValue(i, "type")
	kind = strings.ToLower(strings.TrimSpace(kind))
	body := strings.TrimSpace(doc.Source[el.InnerStart:el.InnerEnd])
	issue := func(code, detail string) []Issue {
		return []Issue{{Code: code, Route: p.Route, Node: el.Node, Detail: detail}}
	}
	switch {
	case kind == "importmap":
		return issue(CodeScriptInlineForbidden, kind)
	case hasSrc && (externalURL(src) || strings.TrimSpace(src) == ""):
		return issue(CodeScriptSourceForbidden, src)
	case !hasSrc && body != "" && !dataScriptTypes[kind]:
		return issue(CodeScriptInlineForbidden, kind)
	}
	return nil
}

func refusedFrame(doc *sitehtml.Document, i int) (string, bool) {
	if doc.Elements[i].Tag == "frame" {
		return "frame", true
	}
	if _, inline := doc.AttrValue(i, "srcdoc"); inline {
		return "srcdoc", true
	}
	src, _ := doc.AttrValue(i, "src")
	if !AllowedFrame(src) {
		return src, true
	}
	return "", false
}

func compactURL(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r > ' ' && r != 0x7f {
			b.WriteRune(r)
		}
	}
	return strings.ToLower(b.String())
}

func dangerousURL(tag, raw string) bool {
	value := compactURL(raw)
	switch {
	case strings.HasPrefix(value, "javascript:"), strings.HasPrefix(value, "vbscript:"):
		return true
	case strings.HasPrefix(value, "data:"):
		return !((tag == "img" || tag == "source") && strings.HasPrefix(value, "data:image/"))
	}
	return false
}

func externalURL(raw string) bool {
	value := compactURL(raw)
	if strings.HasPrefix(value, "//") || strings.HasPrefix(value, "\\") {
		return true
	}
	colon := strings.IndexByte(value, ':')
	if colon < 0 {
		return false
	}
	slash := strings.IndexAny(value, "/?#")
	return slash < 0 || colon < slash
}

func credentialField(doc *sitehtml.Document, i int) bool {
	if kind, _ := doc.AttrValue(i, "type"); strings.EqualFold(strings.TrimSpace(kind), "password") {
		return true
	}
	autocomplete, _ := doc.AttrValue(i, "autocomplete")
	for _, token := range strings.Fields(strings.ToLower(autocomplete)) {
		if strings.HasPrefix(token, "cc-") || credentialAutocomplete[token] {
			return true
		}
	}
	return false
}

type gapList struct {
	seen   map[string]bool
	issues []Issue
}

func (g *gapList) add(key string, issue Issue) {
	if g.seen[key] {
		return
	}
	g.seen[key] = true
	g.issues = append(g.issues, issue)
}

func (g *gapList) element(code string, p RenderedPage, i int) {
	el := p.Document.Elements[i]
	key := code + "|" + el.Node
	if el.Node == "" {
		key = code + "|" + p.Route + "|" + el.Path
	}
	g.add(key, Issue{Code: code, Route: p.Route, Node: el.Node, Detail: el.Path})
}

func (c CheckedPages) OpenGaps(pages []RenderedPage, settings Settings) []Issue {
	g := &gapList{seen: map[string]bool{}}
	privacy, booking := false, false
	for _, p := range pages {
		doc := p.Document
		for _, img := range doc.Images() {
			if !img.HasAlt {
				g.element(CodeAltMissing, p, img.Element)
			}
		}
		for _, link := range doc.Links() {
			if !linkHasDestination(doc, link) {
				g.element(CodeLinkEmpty, p, link.Element)
			}
			if privacyLink(link) {
				privacy = true
			}
		}
		for at, m := range c.Manifests {
			if c.routes[at] != p.Route {
				continue
			}
			if strings.TrimSpace(m.ConsentText) == "" {
				g.add(CodeConsentRequired+"|"+m.ID, Issue{Code: CodeConsentRequired, Route: p.Route, Node: m.Node, Detail: m.ID})
			}
			if !m.HasIdentity() {
				g.add(CodeFormIdentityMissing+"|"+m.ID, Issue{Code: CodeFormIdentityMissing, Route: p.Route, Node: m.Node, Detail: m.ID})
			}
			if settings.Forms[m.ID].CheckDealOwner(m.ID) != nil {
				g.add(CodeFormDealOwnerRequired+"|"+m.ID, Issue{Code: CodeFormDealOwnerRequired, Route: p.Route, Node: m.Node, Detail: m.ID})
			}
			if !followUpReachable(m, settings.Forms[m.ID]) {
				g.add(CodeFormFollowUpFieldMissing+"|"+m.ID, Issue{Code: CodeFormFollowUpFieldMissing, Route: p.Route, Node: m.Node, Detail: m.ID})
			}
		}
		if c.bookingGaps(g, p, settings) {
			booking = true
		}
		for _, i := range invisibleAboveTheFold(doc) {
			g.element(CodeInvisibleWithoutJS, p, i)
		}
	}
	if (len(c.Manifests) > 0 || booking) && !privacy {
		g.add(CodePrivacyLinkMissing, Issue{Code: CodePrivacyLinkMissing})
	}
	if len(settings.Integrations.Enabled()) > 0 && !settings.Consent.Banner {
		g.add(CodeConsentBannerMissing, Issue{Code: CodeConsentBannerMissing})
	}
	return g.issues
}

func privacyLink(link sitehtml.Link) bool {
	if !link.HasHref || strings.TrimSpace(link.Href) == "" {
		return false
	}
	text := shared.FoldForMatch(link.Href + " " + link.Name)
	for _, word := range privacyWords {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func hiddenByOpacity(doc *sitehtml.Document, i int) bool {
	style, ok := doc.AttrValue(i, "style")
	if !ok {
		return false
	}
	for _, declaration := range strings.Split(compactURL(style), ";") {
		value, found := strings.CutPrefix(declaration, "opacity:")
		if !found {
			continue
		}
		value = strings.TrimSuffix(value, "!important")
		value = strings.TrimSuffix(value, "%")
		if strings.Trim(value, "0.") == "" && value != "" {
			return true
		}
	}
	return false
}

func aboveTheFold(doc *sitehtml.Document) (int, int) {
	region, heading := -1, -1
	for i, el := range doc.Elements {
		if region < 0 && (el.Tag == "section" || el.Tag == "header") {
			if _, nested := doc.Ancestor(i, "section", "header"); !nested {
				region = i
			}
		}
		if heading < 0 && el.Tag == "h1" {
			heading = i
		}
	}
	return region, heading
}

func overlaps(doc *sitehtml.Document, i, target int) bool {
	if target < 0 {
		return false
	}
	a, b := doc.Elements[i], doc.Elements[target]
	return (a.Start <= b.Start && b.End <= a.End) || (b.Start <= a.Start && a.End <= b.End)
}

func invisibleAboveTheFold(doc *sitehtml.Document) []int {
	region, heading := aboveTheFold(doc)
	var out []int
	for i := range doc.Elements {
		if !hiddenByOpacity(doc, i) || doc.Text(i) == "" {
			continue
		}
		if overlaps(doc, i, region) || overlaps(doc, i, heading) {
			out = append(out, i)
		}
	}
	return out
}

const (
	CheckHeadingOrder = "heading_order"
	CheckH1Missing    = "h1_missing"
	CheckH1Repeated   = "h1_repeated"
	CheckLinkUnnamed  = "link_unnamed"

	CheckContrastLow        = "contrast_low"
	CheckOverflowHorizontal = "overflow_horizontal"
	CheckImageTooSmall      = "image_too_small"
	CheckConsoleError       = "console_error"
	CheckRequestFailed      = "request_failed"
	CheckAxeViolation       = "axe_violation"
	CheckLCPSlow            = "lcp_slow"
	CheckCLSHigh            = "cls_high"
	CheckTBTHigh            = "tbt_high"
	CheckPageWeightHigh     = "page_weight_high"
)

type Check struct {
	Code     string `json:"code"`
	Route    string `json:"route,omitempty"`
	Node     string `json:"node,omitempty"`
	Viewport int    `json:"viewport,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

func RenderedChecks(pages []RenderedPage) []Check {
	var checks []Check
	for _, p := range pages {
		doc := p.Document
		headings := doc.Headings()
		previous, h1 := 0, 0
		for _, h := range headings {
			if h.Level == 1 {
				h1++
				if h1 == 2 {
					checks = append(checks, Check{Code: CheckH1Repeated, Route: p.Route, Node: h.Node, Detail: h.Text})
				}
			}
			if previous > 0 && h.Level > previous+1 {
				checks = append(checks, Check{Code: CheckHeadingOrder, Route: p.Route, Node: h.Node, Detail: h.Text})
			}
			previous = h.Level
		}
		if h1 == 0 {
			checks = append(checks, Check{Code: CheckH1Missing, Route: p.Route})
		}
		for _, link := range doc.Links() {
			if link.Name == "" {
				checks = append(checks, Check{Code: CheckLinkUnnamed, Route: p.Route, Node: link.Node, Detail: doc.Elements[link.Element].Path})
			}
		}
	}
	return checks
}

func linkHasDestination(doc *sitehtml.Document, link sitehtml.Link) bool {
	if ref, ok := doc.AttrValue(link.Element, WhatsAppNumberAttr); ok && strings.TrimSpace(ref) != "" {
		return true
	}
	href := strings.TrimSpace(link.Href)
	return link.HasHref && href != "" && href != "#"
}
