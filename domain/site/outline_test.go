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
