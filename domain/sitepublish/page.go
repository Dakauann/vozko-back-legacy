package sitepublish

import (
	"errors"
	"net/url"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/domain/sitehtml"
)

const (
	PrerenderPlaceholder = "<!--vz-prerender-->"
	OriginPlaceholder    = "__VZ_ORIGIN__"
)

var ErrPrerenderPlaceholder = errors.New("sitepublish: the page needs exactly one prerender placeholder")

var (
	vzFormID    = regexp.MustCompile(`<vz-form\b[^>]*?\sid\s*=\s*["']([^"']*)["']|\sdata-vz-form\s*=\s*["']([^"']*)["']|<vz-booking\b[^>]*?\sbooking\s*=\s*["']([^"']*)["']`)
	formIDShape = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	absoluteURL = regexp.MustCompile(`https?://[^\s"'()<>,]+`)
)

var runtimeTags = []string{"vz-form", "vz-consent", "vz-popup", "vz-countdown", "vz-whatsapp", WebchatTag, site.BookingTag}

var runtimeAttrs = []string{"data-vz-form", "data-vz-cta", "data-vz-popup", "data-vz-countdown", "data-vz-consent", "data-vz-consent-open"}

var runtimeFiles = []string{"vz-runtime", "pow"}

var trustedNames = []string{
	"itau", "bradesco", "santander", "banco do brasil", "caixa economica", "caixa tem", "nubank", "banco inter",
	"c6 bank", "btg pactual", "sicoob", "sicredi", "banrisul", "mercado pago", "picpay", "pagbank", "pagseguro",
	"pix", "gov br", "receita federal", "receita", "correios", "detran", "inss", "serasa", "fgts", "bolsa familia",
}

func FormIDs(p site.Project) []string {
	seen := map[string]bool{}
	for _, file := range p.Paths() {
		content, _ := p.Content(file)
		for _, match := range vzFormID.FindAllStringSubmatch(content, -1) {
			for _, id := range match[1:] {
				if formIDShape.MatchString(id) {
					seen[id] = true
				}
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func FillPrerender(source, markup string) (string, error) {
	if strings.Count(source, PrerenderPlaceholder) != 1 {
		return "", ErrPrerenderPlaceholder
	}
	return strings.Replace(source, PrerenderPlaceholder, markup, 1), nil
}

func NeedsRuntime(doc *sitehtml.Document) bool {
	if len(doc.ByTag(runtimeTags...)) > 0 {
		return true
	}
	for i := range doc.Elements {
		for _, attr := range runtimeAttrs {
			if doc.HasAttr(i, attr) {
				return true
			}
		}
	}
	return false
}

func runtimeFile(ref string) bool {
	base := path.Base(strings.SplitN(ref, "?", 2)[0])
	for _, prefix := range runtimeFiles {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}
	return false
}

func StripAppBundle(source string, keepRuntime bool) (string, error) {
	doc, err := sitehtml.Parse(source)
	if err != nil {
		return "", err
	}
	type span struct{ start, end int }
	var drop []span
	for i, el := range doc.Elements {
		ref := ""
		switch el.Tag {
		case "script":
			ref, _ = doc.AttrValue(i, "src")
		case "link":
			if rel, _ := doc.AttrValue(i, "rel"); strings.EqualFold(strings.TrimSpace(rel), "modulepreload") {
				ref, _ = doc.AttrValue(i, "href")
			}
		}
		if strings.TrimSpace(ref) == "" || (keepRuntime && runtimeFile(ref)) {
			continue
		}
		drop = append(drop, span{el.Start, el.End})
	}
	out := source
	for i := len(drop) - 1; i >= 0; i-- {
		out = out[:drop[i].start] + out[drop[i].end:]
	}
	return out, nil
}

func OutboundURLs(pages []site.RenderedPage) []string {
	seen := map[string]bool{}
	for _, p := range pages {
		doc := p.Document
		for i, el := range doc.Elements {
			attr := ""
			switch el.Tag {
			case "a", "area":
				attr = "href"
			case "iframe":
				attr = "src"
			default:
				continue
			}
			value, _ := doc.AttrValue(i, attr)
			if u, err := url.Parse(strings.TrimSpace(value)); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
				seen[u.String()] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for link := range seen {
		out = append(out, link)
	}
	sort.Strings(out)
	return out
}

func AbsoluteURLs(text string) []string {
	var out []string
	for _, found := range absoluteURL.FindAllString(text, -1) {
		if !slices.Contains(out, found) {
			out = append(out, found)
		}
	}
	return out
}

func searchable(text string) string {
	var b strings.Builder
	b.WriteByte(' ')
	space := true
	for _, r := range shared.FoldForMatch(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	if !space {
		b.WriteByte(' ')
	}
	return b.String()
}

func pageText(doc *sitehtml.Document) string {
	title, _ := doc.Title()
	parts := []string{title}
	bodies := doc.ByTag("body")
	if len(bodies) == 0 {
		for i, el := range doc.Elements {
			if el.Parent < 0 {
				bodies = append(bodies, i)
			}
		}
	}
	for _, i := range bodies {
		parts = append(parts, doc.Text(i))
	}
	return strings.Join(parts, " ")
}

func HoldForReview(pages []site.RenderedPage) (string, bool) {
	for _, p := range pages {
		if len(p.Document.ByTag("input", "select", "textarea")) == 0 {
			continue
		}
		text := searchable(pageText(p.Document))
		for _, name := range trustedNames {
			if strings.Contains(text, " "+name+" ") {
				return p.Route + ": " + name, true
			}
		}
	}
	return "", false
}

func WebchatWidgets(pages []site.RenderedPage) []string {
	seen := map[string]bool{}
	for _, p := range pages {
		for _, i := range p.Document.ByTag(WebchatTag) {
			if id, _ := p.Document.AttrValue(i, site.WebchatAttr); strings.TrimSpace(id) != "" {
				seen[strings.TrimSpace(id)] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func SettingsDestinations(settings site.Settings) []string {
	seen := map[string]bool{}
	for _, r := range settings.Redirects {
		if u, err := url.Parse(strings.TrimSpace(r.To)); err == nil && u.Scheme == "https" && u.Host != "" {
			seen[u.String()] = true
		}
	}
	out := make([]string, 0, len(seen))
	for target := range seen {
		out = append(out, target)
	}
	sort.Strings(out)
	return out
}

func HoldDestinationsForReview(targets []string) (string, bool) {
	for _, target := range targets {
		u, err := url.Parse(strings.TrimSpace(target))
		if err != nil {
			return target + ": unreadable", true
		}
		host := searchable(u.Hostname())
		for _, name := range trustedNames {
			if strings.Contains(host, " "+name+" ") {
				return target + ": " + name, true
			}
		}
	}
	return "", false
}
