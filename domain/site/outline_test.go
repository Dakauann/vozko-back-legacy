package site

import (
	"strconv"
	"strings"
	"testing"

	"vozko/domain/sitehtml"
)

func TestNodeOutlineFollowsTheAnnotatedElementsOnly(t *testing.T) {
	doc, err := sitehtml.Parse(`<!DOCTYPE html><html><head><title>x</title></head><body><div id="root">` +
		`<header data-vz-node="src/components/Header.jsx#10"><a data-vz-node="src/components/Header.jsx#40" href="/">Sua Empresa</a></header>` +
		`<main data-vz-node="src/routes/index.jsx#5"><h1 data-vz-node="src/routes/index.jsx#30">Agende sua avaliação gratuita e saia com um plano claro hoje mesmo</h1>` +
		`<img data-vz-node="src/routes/index.jsx#90" src="/a.webp" alt="Foto da clínica"></main></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	got := NodeOutline(doc, 8, 100)
	if len(got) != 2 || got[0].Tag != "header" || got[0].Label != "Header" || got[1].Node != "src/routes/index.jsx#5" {
		t.Fatalf("roots %+v", got)
	}
	main := got[1]
	if len(main.Children) != 2 || !strings.HasPrefix(main.Children[0].Label, "Agende sua avaliação") || len([]rune(main.Children[0].Label)) > maxOutlineLabel+1 || main.Children[1].Label != "Foto da clínica" {
		t.Fatalf("main %+v", main)
	}
	if main.Children[0].Path == "" {
		t.Fatal("every node keeps its element path")
	}
}

func TestNodeOutlineListsAnIconComponentRenderedAsSvgAsItsOwnNode(t *testing.T) {
	doc, err := sitehtml.Parse(`<html><body><span data-vz-node="src/pages/Home.jsx#40"><svg data-vz-inst="src/pages/Home.jsx#60" class="size-6"><path d="M0 0"></path></svg></span>` +
		`<div data-vz-inst="src/pages/Home.jsx#90">componente</div><svg data-vz-inst="nada"></svg></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	got := NodeOutline(doc, 8, 100)
	if len(got) != 1 || len(got[0].Children) != 1 || got[0].Children[0].Node != "src/pages/Home.jsx#60" || got[0].Children[0].Tag != "svg" {
		t.Fatalf("got %+v", got)
	}
}

func TestNodeOutlineIsBoundedInDepthAndSize(t *testing.T) {
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := 0; i < 12; i++ {
		b.WriteString(`<div data-vz-node="pages/index.html#` + strconv.Itoa(i+1) + `">`)
	}
	b.WriteString(strings.Repeat("</div>", 12))
	for i := 0; i < 50; i++ {
		b.WriteString(`<p data-vz-node="pages/index.html#` + strconv.Itoa(1000+i) + `">p</p>`)
	}
	b.WriteString("</body></html>")
	doc, err := sitehtml.Parse(b.String())
	if err != nil {
		t.Fatal(err)
	}
	got := NodeOutline(doc, 3, 20)
	depth, count := 0, 0
	var walk func([]OutlineNode, int)
	walk = func(nodes []OutlineNode, d int) {
		for _, n := range nodes {
			count++
			depth = max(depth, d)
			walk(n.Children, d+1)
		}
	}
	walk(got, 1)
	if depth > 3 || count > 20 {
		t.Fatalf("depth %d count %d", depth, count)
	}
}

func TestNodeOutlineCarriesAnchorsAndTheFormEachNodeBelongsTo(t *testing.T) {
	doc, err := sitehtml.Parse(`<html><body>` +
		`<section data-vz-node="pages/index.html#1" id="planos"><h2 data-vz-node="pages/index.html#2">Planos</h2></section>` +
		`<vz-form id="contato"><form data-vz-node="pages/index.html#3"><button data-vz-node="pages/index.html#4">Enviar</button></form></vz-form>` +
		`<form data-vz-node="pages/index.html#5" data-vz-form="orcamento" id="orcamento-form"><div><button data-vz-node="pages/index.html#6">Pedir</button></div></form>` +
		`</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	got := NodeOutline(doc, 8, 100)
	if len(got) != 3 {
		t.Fatalf("roots %+v", got)
	}
	section, contato, orcamento := got[0], got[1], got[2]
	if section.Anchor != "planos" || section.Form != "" || section.Children[0].Anchor != "" {
		t.Fatalf("section %+v", section)
	}
	if contato.Form != "contato" || contato.Anchor != "" || contato.Children[0].Form != "contato" {
		t.Fatalf("contato %+v", contato)
	}
	if orcamento.Form != "orcamento" || orcamento.Anchor != "orcamento-form" || orcamento.Children[0].Form != "orcamento" {
		t.Fatalf("orcamento %+v", orcamento)
	}
}
