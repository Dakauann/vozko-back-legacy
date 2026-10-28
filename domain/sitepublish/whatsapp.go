package sitepublish

import (
	"errors"
	"fmt"
	"html"
	"sort"
	"strings"

	"vozko/domain/sitehtml"
)

var ErrWhatsAppLinkUnsafe = errors.New("sitepublish: a WhatsApp link must be an https address")

type WhatsAppAnchor struct {
	NumberRef string
	Text      string
}

type WhatsAppResolver func(WhatsAppAnchor) (string, bool, error)

type splice struct {
	start, end int
	text       string
}

func whatsAppAnchor(doc *sitehtml.Document, i int) (WhatsAppAnchor, bool) {
	switch doc.Elements[i].Tag {
	case "a":
		ref, ok := doc.AttrValue(i, "data-vz-whatsapp")
		text, _ := doc.AttrValue(i, "data-vz-text")
		return WhatsAppAnchor{NumberRef: strings.TrimSpace(ref), Text: text}, ok
	case "vz-whatsapp":
		ref, _ := doc.AttrValue(i, "channel")
		text, _ := doc.AttrValue(i, "text")
		return WhatsAppAnchor{NumberRef: strings.TrimSpace(ref), Text: text}, true
	}
	return WhatsAppAnchor{}, false
}

func LinkWhatsApp(source string, resolve WhatsAppResolver) (string, error) {
	if !strings.Contains(source, "data-vz-whatsapp") && !strings.Contains(source, "<vz-whatsapp") {
		return source, nil
	}
	doc, err := sitehtml.Parse(source)
	if err != nil {
		return "", err
	}
	var edits []splice
	for _, i := range doc.ByTag("a", "vz-whatsapp") {
		anchor, ok := whatsAppAnchor(doc, i)
		if !ok || anchor.NumberRef == "" {
			continue
		}
		link, found, err := resolve(anchor)
		if err != nil {
			return "", err
		}
		if !found {
			continue
		}
		if !strings.HasPrefix(link, "https://") {
			return "", fmt.Errorf("%w: %s", ErrWhatsAppLinkUnsafe, link)
		}
		attr := `href="` + html.EscapeString(link) + `"`
		el := doc.Elements[i]
		if href, ok := doc.Attr(i, "href"); ok {
			edits = append(edits, splice{start: href.Start, end: href.End, text: attr})
			continue
		}
		at := el.Start + 1 + len(el.Tag)
		edits = append(edits, splice{start: at, end: at, text: " " + attr})
	}
	sort.Slice(edits, func(a, b int) bool { return edits[a].start > edits[b].start })
	out := source
	for _, e := range edits {
		out = out[:e.start] + e.text + out[e.end:]
	}
	return out, nil
}
