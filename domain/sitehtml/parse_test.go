package sitehtml

import (
	"errors"
	"strings"
	"testing"
)

const rendered = `<main>
<section class="hero py-20" id="topo" data-vz-node="src/pages/Home.jsx#120">
  <h1 class="text-4xl" data-vz-node="src/pages/Home.jsx#188">Olá <strong>mundo</strong></h1>
  <p>
    Texto de apoio
  </p>
  <img src="/media/abc.webp" alt="Foto">
  <a href="/obrigado" class="btn" data-vz-node="src/components/Cta.jsx#40">Quero</a>
  <a href="/obrigado" class="btn" data-vz-node="src/components/Cta.jsx#40">Quero</a>
</section>
</main>
`

func mustParse(t *testing.T, source string) *Document {
	t.Helper()
	doc, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func find(t *testing.T, doc *Document, tag string, nth int) int {
	t.Helper()
	found := doc.ByTag(tag)
	if nth >= len(found) {
		t.Fatalf("no %s number %d", tag, nth)
	}
	return found[nth]
}

func TestParseRecordsEveryElementWithExactByteRanges(t *testing.T) {
	doc := mustParse(t, rendered)
	tags := []string{}
	for _, el := range doc.Elements {
		tags = append(tags, el.Tag)
	}
	if strings.Join(tags, ",") != "main,section,h1,strong,p,img,a,a" {
		t.Fatalf("got %v", tags)
	}
	for _, el := range doc.Elements {
		start := rendered[el.Start:el.StartTagEnd]
		if !strings.HasPrefix(start, "<"+el.Tag) || !strings.HasSuffix(start, ">") {
			t.Fatalf("%s start tag %q", el.Tag, start)
		}
		if el.Void {
			if el.InnerStart != el.StartTagEnd || el.InnerEnd != el.StartTagEnd || el.End != el.StartTagEnd {
				t.Fatalf("void %s has %+v", el.Tag, el)
			}
			continue
		}
		if rendered[el.InnerEnd:el.End] != "</"+el.Tag+">" {
			t.Fatalf("%s end tag %q", el.Tag, rendered[el.InnerEnd:el.End])
		}
	}
	h1 := doc.Elements[find(t, doc, "h1", 0)]
	if rendered[h1.InnerStart:h1.InnerEnd] != "Olá <strong>mundo</strong>" {
		t.Fatalf("got inner %q", rendered[h1.InnerStart:h1.InnerEnd])
	}
}

func TestParseBuildsTheTreeAndElementPaths(t *testing.T) {
	doc := mustParse(t, rendered)
	want := []struct {
		tag  string
		nth  int
		path string
	}{
		{"main", 0, "main[0]"},
		{"section", 0, "main[0]/section[0]"},
		{"h1", 0, "main[0]/section[0]/h1[0]"},
		{"strong", 0, "main[0]/section[0]/h1[0]/strong[0]"},
		{"a", 0, "main[0]/section[0]/a[0]"},
		{"a", 1, "main[0]/section[0]/a[1]"},
	}
	for _, w := range want {
		i := find(t, doc, w.tag, w.nth)
		if doc.Elements[i].Path != w.path {
			t.Fatalf("%s %d path %q, want %q", w.tag, w.nth, doc.Elements[i].Path, w.path)
		}
		found, ok := doc.ByPath(w.path)
		if !ok || found != i {
			t.Fatalf("ByPath(%q) = %d, %v", w.path, found, ok)
		}
	}
	section := find(t, doc, "section", 0)
	if len(doc.Elements[section].Children) != 5 || doc.Elements[section].Parent != find(t, doc, "main", 0) {
		t.Fatalf("got %+v", doc.Elements[section])
	}
	if doc.Elements[find(t, doc, "main", 0)].Parent != -1 {
		t.Fatal("main is a root")
	}
	strong := find(t, doc, "strong", 0)
	if ancestor, ok := doc.Ancestor(strong, "section", "form"); !ok || ancestor != section {
		t.Fatalf("got %d %v", ancestor, ok)
	}
	if _, ok := doc.Ancestor(strong, "form"); ok {
		t.Fatal("no form around")
	}
	if got := doc.Descendants(section, "a", "img"); len(got) != 3 {
		t.Fatalf("got %v", got)
	}
}

func TestParseReadsTheSourceNodeAnnotations(t *testing.T) {
	doc := mustParse(t, rendered)
	h1 := find(t, doc, "h1", 0)
	if doc.Elements[h1].Node != "src/pages/Home.jsx#188" {
		t.Fatalf("got %q", doc.Elements[h1].Node)
	}
	if doc.Elements[find(t, doc, "p", 0)].Node != "" {
		t.Fatal("an element without annotation has no node")
	}
	links := doc.byNode("src/components/Cta.jsx#40")
	if len(links) != 2 || links[0] != find(t, doc, "a", 0) || links[1] != find(t, doc, "a", 1) {
		t.Fatalf("a component rendered twice keeps one source node, got %v", links)
	}
}

func TestParseRefusesMalformedAnnotations(t *testing.T) {
	_, err := Parse(`<div data-vz-node="sem-offset"></div>`)
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("got %v", err)
	}
}

func TestNodeIDsAreSourceFileAndStartOffset(t *testing.T) {
	file, offset, err := ParseNodeID("src/pages/Home.jsx#188")
	if err != nil || file != "src/pages/Home.jsx" || offset != 188 {
		t.Fatalf("got %q %d %v", file, offset, err)
	}
	if NodeID("pages/index.html", 41) != "pages/index.html#41" {
		t.Fatal("NodeID")
	}
	for _, id := range []string{"", "pages/index.html", "#4", "pages/index.html#", "pages/index.html#-1", "pages/index.html#+4", "pages/index.html#4a", "pages/index.html# 4", "pages/index.html#04"} {
		if _, _, err := ParseNodeID(id); !errors.Is(err, ErrNodeID) {
			t.Fatalf("%q: got %v", id, err)
		}
	}
}

func TestParseReadsAttributesInEveryQuotingStyle(t *testing.T) {
	source := `<input type=email name='lead.email' required data-x="a &amp; b" placeholder = "Seu e-mail">`
	doc := mustParse(t, source)
	el := doc.Elements[0]
	want := []Attr{
		{Name: "type", Value: "email", HasValue: true},
		{Name: "name", Value: "lead.email", HasValue: true},
		{Name: "required"},
		{Name: "data-x", Value: "a & b", HasValue: true},
		{Name: "placeholder", Value: "Seu e-mail", HasValue: true},
	}
	if len(el.Attrs) != len(want) {
		t.Fatalf("got %+v", el.Attrs)
	}
	raws := []string{`type=email`, `name='lead.email'`, `required`, `data-x="a &amp; b"`, `placeholder = "Seu e-mail"`}
	for i, attr := range el.Attrs {
		if attr.Name != want[i].Name || attr.Value != want[i].Value || attr.HasValue != want[i].HasValue {
			t.Fatalf("attr %d got %+v", i, attr)
		}
		if source[attr.Start:attr.End] != raws[i] {
			t.Fatalf("attr %d raw %q", i, source[attr.Start:attr.End])
		}
	}
	if value, ok := doc.AttrValue(0, "TYPE"); !ok || value != "email" {
		t.Fatalf("got %q %v", value, ok)
	}
	if !doc.HasAttr(0, "required") || doc.HasAttr(0, "disabled") {
		t.Fatal("HasAttr")
	}
}

func TestParseAcceptsSerializedOutputOfBothStacks(t *testing.T) {
	cases := map[string]string{
		"void with slash":         `<br/><hr />`,
		"svg self closing":        `<svg viewBox="0 0 10 10"><path d="M0 0L10 10"/><circle r="2" /></svg>`,
		"svg serialized closed":   `<svg viewBox="0 0 10 10"><path d="M0 0L10 10"></path></svg>`,
		"script raw text":         `<script type="module">if (a < b && c > d) { x("</div>") }</script>`,
		"style raw text":          `<style>.a > .b { color: red }</style>`,
		"comment with tags":       `<div><!-- <span> --></div>`,
		"react text separators":   `<p>Olá <!-- -->Ana</p>`,
		"uppercase tags":          `<DIV class="x"></div>`,
		"text only":               `só texto`,
		"empty":                   ``,
		"custom element":          `<vz-form id="contato"><input name="lead.name"></vz-form>`,
		"textarea raw text":       `<textarea name="field.obs"><b>não é tag</b></textarea>`,
		"doctype in a full shell": `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Oi</title></head><body></body></html>`,
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(source); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestParseRefusesOutputItCannotReadUnambiguously(t *testing.T) {
	cases := map[string]string{
		"unclosed":            `<div><p>oi</p>`,
		"stray end":           `<p>oi</p></div>`,
		"mismatched":          `<div><span></div></span>`,
		"void end tag":        `<p>a</br></p>`,
		"self closing div":    `<div/>`,
		"duplicate attribute": `<a href="/a" href="/b">x</a>`,
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(source)
			var parseErr *ParseError
			if !errors.Is(err, ErrMalformed) || !errors.As(err, &parseErr) || parseErr.Reason == "" {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestTextReturnsTheDecodedVisibleTextContent(t *testing.T) {
	doc := mustParse(t, `<label><input type="checkbox" name="consent"> Aceito a <a href="/privacidade">Política &amp; Termos</a>
   e o <!-- -->contato.<script>var x = 1</script><style>.a{}</style></label>`)
	if got := doc.Text(0); got != "Aceito a Política & Termos e o contato." {
		t.Fatalf("got %q", got)
	}
}
