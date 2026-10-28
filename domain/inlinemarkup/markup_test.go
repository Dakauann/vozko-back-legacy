package inlinemarkup

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseReadsParagraphsMarksAndLinks(t *testing.T) {
	got, err := Parse("Olá, **{{first_name|cliente}}**!\nVeja *as* __novidades__.\n\n[Compre agora](https://loja.example.com/a?b=1) e ganhe ***frete***")
	if err != nil {
		t.Fatal(err)
	}
	want := []Paragraph{
		{Runs: []Run{
			{Text: "Olá, "},
			{Text: "{{first_name|cliente}}", Bold: true},
			{Text: "!\nVeja "},
			{Text: "as", Italic: true},
			{Text: " "},
			{Text: "novidades", Underline: true},
			{Text: "."},
		}},
		{Runs: []Run{
			{Text: "Compre agora", Link: &Link{Href: "https://loja.example.com/a?b=1"}},
			{Text: " e ganhe "},
			{Text: "frete", Bold: true, Italic: true},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestParseKeepsEscapesAndSingleUnderscores(t *testing.T) {
	got, err := Parse(`2 \* 3 = 6, snake_case, \[nota\], \\ e \__dois`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Paragraph{{Runs: []Run{{Text: `2 * 3 = 6, snake_case, [nota], \ e __dois`}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestParseTogglesAdjacentMarksWithoutAmbiguity(t *testing.T) {
	got, err := Parse("*a***b**")
	if err != nil {
		t.Fatal(err)
	}
	want := []Paragraph{{Runs: []Run{{Text: "a", Italic: true}, {Text: "b", Bold: true}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestParseRefusesBrokenMarkup(t *testing.T) {
	cases := map[string]string{
		"unclosed bold":   "**oi",
		"unclosed link":   "[oi](https://x.com",
		"label only":      "[oi] solto",
		"too many stars":  "****oi****",
		"unclosed italic": "oi *mundo",
		"empty":           "  \n\n ",
		"nested link":     "[a [b](https://x.com)](https://y.com)",
	}
	for name, markup := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(markup); !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected ErrInvalid, got %v", err)
			}
		})
	}
}

func TestFormatRoundTrips(t *testing.T) {
	paragraphs := []Paragraph{
		{Runs: []Run{
			{Text: "Preço: 2 * 3 [promo] a__b snake_case \\ "},
			{Text: "negrito", Bold: true},
			{Text: "itálico", Italic: true},
			{Text: "ambos", Bold: true, Italic: true},
			{Text: "sublinhado", Underline: true},
			{Text: "{{first_name|cliente}}", Bold: true, Link: &Link{Href: "https://x.com/p(1)"}},
			{Text: " fim"},
		}},
		{Runs: []Run{{Text: "linha 1\nlinha 2"}}},
	}
	markup := Format(paragraphs)
	back, err := Parse(markup)
	if err != nil {
		t.Fatalf("%q: %v", markup, err)
	}
	want := []Paragraph{
		paragraphs[0],
		paragraphs[1],
	}
	want[0].Runs[5].Link = &Link{Href: "https://x.com/p(1%29"}
	if !reflect.DeepEqual(back, want) {
		t.Fatalf("markup %q\ngot  %+v\nwant %+v", markup, back, want)
	}
}

func TestFormatMergesEqualNeighbours(t *testing.T) {
	got := Format([]Paragraph{{Runs: []Run{{Text: "a", Bold: true}, {Text: "b", Bold: true}, {Text: "c"}}}})
	if got != "**ab**c" {
		t.Fatalf("got %q", got)
	}
}

func TestParseErrorsCarryTheReasonForTheMessage(t *testing.T) {
	_, err := Parse("**oi")
	var invalid *Error
	if !errors.As(err, &invalid) || invalid.Reason != "a bold, italic or underline mark is not closed" {
		t.Fatalf("got %v", err)
	}
	if err.Error() != "inline markup is invalid: a bold, italic or underline mark is not closed" {
		t.Fatalf("got %q", err.Error())
	}
}
