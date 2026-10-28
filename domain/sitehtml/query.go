package sitehtml

import (
	"strings"

	"golang.org/x/net/html"
)

type Link struct {
	Element int
	Href    string
	HasHref bool
	Name    string
	Node    string
}

type Image struct {
	Element int
	Src     string
	Alt     string
	HasAlt  bool
	Node    string
}

type Script struct {
	Element int
	Src     string
	HasSrc  bool
	Type    string
	Body    string
	Node    string
}

type Heading struct {
	Element int
	Level   int
	Text    string
	Node    string
}

type Meta struct {
	Element   int
	Name      string
	Property  string
	HTTPEquiv string
	Content   string
	Charset   string
}

func (d *Document) AccessibleName(i int) string {
	if label, ok := d.AttrValue(i, "aria-label"); ok && strings.TrimSpace(label) != "" {
		return strings.TrimSpace(label)
	}
	if text := d.Text(i); text != "" {
		return text
	}
	var alts []string
	for _, img := range d.Descendants(i, "img") {
		if alt, ok := d.AttrValue(img, "alt"); ok && strings.TrimSpace(alt) != "" {
			alts = append(alts, strings.TrimSpace(alt))
		}
	}
	if len(alts) > 0 {
		return strings.Join(alts, " ")
	}
	title, _ := d.AttrValue(i, "title")
	return strings.TrimSpace(title)
}

func (d *Document) Links() []Link {
	var out []Link
	for _, i := range d.ByTag("a") {
		href, has := d.AttrValue(i, "href")
		out = append(out, Link{Element: i, Href: href, HasHref: has, Name: d.AccessibleName(i), Node: d.Elements[i].Node})
	}
	return out
}

func (d *Document) Images() []Image {
	var out []Image
	for _, i := range d.ByTag("img") {
		src, _ := d.AttrValue(i, "src")
		alt, has := d.AttrValue(i, "alt")
		out = append(out, Image{Element: i, Src: src, Alt: alt, HasAlt: has, Node: d.Elements[i].Node})
	}
	return out
}

func (d *Document) scripts() []Script {
	var out []Script
	for _, i := range d.ByTag("script") {
		el := d.Elements[i]
		src, has := d.AttrValue(i, "src")
		kind, _ := d.AttrValue(i, "type")
		out = append(out, Script{Element: i, Src: src, HasSrc: has, Type: strings.ToLower(strings.TrimSpace(kind)), Body: d.Source[el.InnerStart:el.InnerEnd], Node: el.Node})
	}
	return out
}

func (d *Document) Headings() []Heading {
	var out []Heading
	for _, i := range d.ByTag("h1", "h2", "h3", "h4", "h5", "h6") {
		el := d.Elements[i]
		out = append(out, Heading{Element: i, Level: int(el.Tag[1] - '0'), Text: d.Text(i), Node: el.Node})
	}
	return out
}

func (d *Document) Metas() []Meta {
	var out []Meta
	for _, i := range d.ByTag("meta") {
		m := Meta{Element: i}
		m.Name, _ = d.AttrValue(i, "name")
		m.Property, _ = d.AttrValue(i, "property")
		m.HTTPEquiv, _ = d.AttrValue(i, "http-equiv")
		m.Content, _ = d.AttrValue(i, "content")
		m.Charset, _ = d.AttrValue(i, "charset")
		out = append(out, m)
	}
	return out
}

func (d *Document) Title() (string, bool) {
	titles := d.ByTag("title")
	if len(titles) == 0 {
		return "", false
	}
	if _, inSVG := d.Ancestor(titles[0], "svg"); inSVG {
		return "", false
	}
	el := d.Elements[titles[0]]
	return strings.Join(strings.Fields(html.UnescapeString(d.Source[el.InnerStart:el.InnerEnd])), " "), true
}

func strip(source string) (string, error) {
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.Grow(len(source))
	last := 0
	for _, el := range doc.Elements {
		for _, attr := range el.Attrs {
			if attr.Name != AnnotationAttr {
				continue
			}
			from := attr.Start
			for from > el.Start && isSpace(source[from-1]) {
				from--
			}
			b.WriteString(source[last:from])
			last = attr.End
		}
	}
	b.WriteString(source[last:])
	return b.String(), nil
}
