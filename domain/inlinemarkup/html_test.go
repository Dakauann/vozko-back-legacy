package inlinemarkup

import (
	"errors"
	"reflect"
	"testing"
)

func TestFromHTMLKeepsMarksLinksAndParagraphs(t *testing.T) {
	cases := map[string]struct {
		html string
		want []Paragraph
	}{
		"plain": {"Olá mundo", []Paragraph{{Runs: []Run{{Text: "Olá mundo"}}}}},
		"marks": {"<b>Oferta</b> <em>só</em> <u>hoje</u> <strong><i>já</i></strong>", []Paragraph{{Runs: []Run{
			{Text: "Oferta", Bold: true}, {Text: " "}, {Text: "só", Italic: true}, {Text: " "}, {Text: "hoje", Underline: true}, {Text: " "},
			{Text: "já", Bold: true, Italic: true},
		}}}},
		"link": {`Fale <a href="https://wa.me/55" target="_blank" rel="noopener">conosco <b>agora</b></a>.`, []Paragraph{{Runs: []Run{
			{Text: "Fale "}, {Text: "conosco ", Link: &Link{Href: "https://wa.me/55"}}, {Text: "agora", Bold: true, Link: &Link{Href: "https://wa.me/55"}}, {Text: "."},
		}}}},
		"entities and breaks": {"5 &lt; 6 &amp; 7<br>fim", []Paragraph{{Runs: []Run{{Text: "5 < 6 & 7 fim"}}}}},
		"paragraphs":          {"<p>Um</p><p><b>Dois</b></p>", []Paragraph{{Runs: []Run{{Text: "Um"}}}, {Runs: []Run{{Text: "Dois", Bold: true}}}}},
		"divs":                {"Antes<div>Depois</div>", []Paragraph{{Runs: []Run{{Text: "Antes"}}}, {Runs: []Run{{Text: "Depois"}}}}},
		"unknown wrapper":     {`<span class="x">a</span><font>b</font>`, []Paragraph{{Runs: []Run{{Text: "ab"}}}}},
		"dropped content":     {"a<script>alert(1)</script><style>p{}</style>b", []Paragraph{{Runs: []Run{{Text: "ab"}}}}},
		"empty":               {"  ", nil},
		"empty link":          {`<a href="">x</a>`, []Paragraph{{Runs: []Run{{Text: "x"}}}}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := FromHTML(c.html)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestFromHTMLRoundTripsThroughFormat(t *testing.T) {
	got, err := FromHTML(`<b>2 * 3</b> e <a href="https://x.example/a)b">[nota]</a>`)
	if err != nil {
		t.Fatal(err)
	}
	markup := Format(got)
	if markup != `**2 \* 3** e [\[nota\]](https://x.example/a%29b)` {
		t.Fatalf("got %q", markup)
	}
	back, err := Parse(markup)
	if err != nil || len(back) != 1 || back[0].Runs[0].Text != "2 * 3" {
		t.Fatalf("got %+v %v", back, err)
	}
}

func TestFromHTMLClosesNestedLinksAndRefusesScriptLinks(t *testing.T) {
	if _, err := FromHTML(`<a href="/a">x<a href="/b">y</a></a>`); err != nil {
		t.Fatalf("the html parser closes the outer link: %v", err)
	}
	if _, err := FromHTML(`<a href="javascript:alert(1)">x</a>`); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}
