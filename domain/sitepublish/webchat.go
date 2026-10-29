package sitepublish

import (
	"errors"
	"fmt"
	"html"
	"sort"
	"strings"

	"vozko/domain/site"
	"vozko/domain/sitehtml"
)

const (
	WebchatTag     = site.WebchatTag
	WebchatKeyAttr = "data-vz-webchat-key"
)

var ErrWebchatKeyUnsafe = errors.New("sitepublish: a chat widget key has characters a page cannot carry")

type WebchatResolver func(ref string) (string, bool, error)

func LinkWebchat(source string, resolve WebchatResolver) (string, error) {
	if !strings.Contains(source, "<"+WebchatTag) {
		return source, nil
	}
	doc, err := sitehtml.Parse(source)
	if err != nil {
		return "", err
	}
	var edits []splice
	for _, i := range doc.ByTag(WebchatTag) {
		if existing, ok := doc.Attr(i, WebchatKeyAttr); ok {
			edits = append(edits, splice{start: existing.Start, end: existing.End})
		}
		ref, _ := doc.AttrValue(i, site.WebchatAttr)
		if ref = strings.TrimSpace(ref); ref == "" {
			continue
		}
		value, found, err := resolve(ref)
		if err != nil {
			return "", err
		}
		if !found {
			continue
		}
		if !site.ValidWebchatKey(value) {
			return "", fmt.Errorf("%w: %s", ErrWebchatKeyUnsafe, ref)
		}
		at := doc.Elements[i].Start + 1 + len(WebchatTag)
		edits = append(edits, splice{start: at, end: at, text: " " + WebchatKeyAttr + `="` + html.EscapeString(value) + `"`})
	}
	return applySplices(source, edits), nil
}

func applySplices(source string, edits []splice) string {
	sort.SliceStable(edits, func(a, b int) bool { return edits[a].start > edits[b].start })
	out := source
	for _, e := range edits {
		out = out[:e.start] + e.text + out[e.end:]
	}
	return out
}
