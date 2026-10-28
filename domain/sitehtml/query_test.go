package sitehtml

import (
	"errors"
	"testing"
)

func TestStripRemovesOnlyTheAnnotationBytes(t *testing.T) {
	cases := []struct {
		name     string
		in, want string
	}{
		{"one element", `<h1 class="a" data-vz-node="pages/index.html#0">Oi</h1>`, `<h1 class="a">Oi</h1>`},
		{"first attribute", `<h1 data-vz-node="pages/index.html#0" class="a">Oi</h1>`, `<h1 class="a">Oi</h1>`},
		{"self closing", `<img src="/a.png" data-vz-node="pages/index.html#3" />`, `<img src="/a.png" />`},
		{"nested and raw text untouched", "<div data-vz-node=\"src/App.jsx#1\">\n  <script>var s = 'data-vz-node=\"x#1\"'</script>\n\t<p  data-vz-node='src/App.jsx#9'  >a</p></div>", "<div>\n  <script>var s = 'data-vz-node=\"x#1\"'</script>\n\t<p  >a</p></div>"},
		{"nothing to strip", "<div class=\"x\"  id=y>\r\n<br/>  </div>", "<div class=\"x\"  id=y>\r\n<br/>  </div>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := strip(c.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestStripRefusesWhatItCannotParse(t *testing.T) {
	if _, err := strip(`<div data-vz-node="a#1">`); !errors.Is(err, ErrMalformed) {
		t.Fatalf("got %v", err)
	}
}

func TestLinksReportHrefAndAccessibleName(t *testing.T) {
	doc := mustParse(t, `<nav>
<a href="/obrigado" data-vz-node="pages/index.html#5">Quero <b>agora</b></a>
<a>sem destino</a>
<a href="  ">em branco</a>
<a href="https://wa.me/55"><img src="/w.svg" alt="WhatsApp"></a>
<a href="#topo" aria-label="Voltar ao topo"></a>
</nav>`)
	links := doc.Links()
	if len(links) != 5 {
		t.Fatalf("got %+v", links)
	}
	want := []Link{
		{Href: "/obrigado", HasHref: true, Name: "Quero agora", Node: "pages/index.html#5"},
		{Name: "sem destino"},
		{Href: "  ", HasHref: true, Name: "em branco"},
		{Href: "https://wa.me/55", HasHref: true, Name: "WhatsApp"},
		{Href: "#topo", HasHref: true, Name: "Voltar ao topo"},
	}
	for i, w := range want {
		got := links[i]
		got.Element = 0
		if got != w {
			t.Fatalf("link %d got %+v want %+v", i, got, w)
		}
	}
}

func TestImagesReportAltPresence(t *testing.T) {
	doc := mustParse(t, `<div><img src="/a.webp" alt="Sorriso"><img src="/b.webp" alt=""><img src="/c.webp"></div>`)
	images := doc.Images()
	if len(images) != 3 {
		t.Fatalf("got %+v", images)
	}
	if !images[0].HasAlt || images[0].Alt != "Sorriso" || !images[1].HasAlt || images[1].Alt != "" || images[2].HasAlt || images[2].Src != "/c.webp" {
		t.Fatalf("got %+v", images)
	}
}

func TestScriptsReportSourceTypeAndBody(t *testing.T) {
	doc := mustParse(t, `<script type="module" src="/assets/index-a1.js"></script><script>alert(1)</script><script type="application/ld+json">{"@type":"FAQPage"}</script>`)
	scripts := doc.scripts()
	if len(scripts) != 3 {
		t.Fatalf("got %+v", scripts)
	}
	if !scripts[0].HasSrc || scripts[0].Src != "/assets/index-a1.js" || scripts[0].Type != "module" || scripts[0].Body != "" {
		t.Fatalf("got %+v", scripts[0])
	}
	if scripts[1].HasSrc || scripts[1].Body != "alert(1)" || scripts[1].Type != "" {
		t.Fatalf("got %+v", scripts[1])
	}
	if scripts[2].Type != "application/ld+json" || scripts[2].Body != `{"@type":"FAQPage"}` {
		t.Fatalf("got %+v", scripts[2])
	}
}

func TestHeadingsAndMetaAreListedInOrder(t *testing.T) {
	doc := mustParse(t, `<html><head><meta charset="utf-8"><meta name="description" content="Clínica"><meta property="og:title" content="Sorriso"><title>Sorriso</title></head><body><h1>Oi <em>você</em></h1><h3>Sub</h3><h2>Dois</h2></body></html>`)
	headings := doc.Headings()
	if len(headings) != 3 || headings[0].Level != 1 || headings[0].Text != "Oi você" || headings[1].Level != 3 || headings[2].Text != "Dois" {
		t.Fatalf("got %+v", headings)
	}
	metas := doc.Metas()
	if len(metas) != 3 || metas[0].Charset != "utf-8" || metas[1].Name != "description" || metas[1].Content != "Clínica" || metas[2].Property != "og:title" {
		t.Fatalf("got %+v", metas)
	}
	if title, ok := doc.Title(); !ok || title != "Sorriso" {
		t.Fatalf("got %q %v", title, ok)
	}
}
