package site

import (
	"errors"
	"strings"
	"testing"
)

func validDesignSystem() DesignSystem {
	return DesignSystem{
		Name:        "Clínica Sorriso",
		Tokens:      map[string]string{"--color-brand": "oklch(0.6 0.2 250)", "--radius-card": "1rem"},
		Fonts:       []DesignFont{{Family: "Inter", Role: FontBody, Weights: []int{400, 600}}, {Family: "Playfair Display", Role: FontHeading, Weights: []int{700}}},
		LogoMediaID: "med_01",
		Voice:       "Próximo, direto e sem jargões.",
		Snippets:    []DesignSnippet{{Key: "hero-split", Name: "Hero dividido", Stack: StackHTML, Content: "<section class=\"py-20\"></section>"}},
	}
}

func TestADesignSystemWithTokensFontsAndSnippetsIsValid(t *testing.T) {
	if err := validDesignSystem().Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (DesignSystem{Name: "Vazio"}).Validate(); err != nil {
		t.Fatalf("only the name is required: %v", err)
	}
}

func TestDesignSystemRefusals(t *testing.T) {
	cases := map[string]func(*DesignSystem){
		"empty name":  func(d *DesignSystem) { d.Name = " " },
		"padded name": func(d *DesignSystem) { d.Name = " Marca" },
		"long name":   func(d *DesignSystem) { d.Name = strings.Repeat("a", 121) },
		"token name":  func(d *DesignSystem) { d.Tokens = map[string]string{"color": "red"} },
		"token value": func(d *DesignSystem) { d.Tokens = map[string]string{"--a": "red; background: url(x)"} },
		"font family": func(d *DesignSystem) { d.Fonts = []DesignFont{{Family: "<b>", Role: FontBody}} },
		"font role":   func(d *DesignSystem) { d.Fonts = []DesignFont{{Family: "Inter", Role: "title"}} },
		"font weight": func(d *DesignSystem) { d.Fonts = []DesignFont{{Family: "Inter", Role: FontBody, Weights: []int{450}}} },
		"repeated role": func(d *DesignSystem) {
			d.Fonts = []DesignFont{{Family: "Inter", Role: FontBody}, {Family: "Lora", Role: FontBody}}
		},
		"logo id":    func(d *DesignSystem) { d.LogoMediaID = "../x" },
		"long voice": func(d *DesignSystem) { d.Voice = strings.Repeat("a", 2001) },
		"snippet key": func(d *DesignSystem) {
			d.Snippets = []DesignSnippet{{Key: "Hero", Name: "x", Stack: StackHTML, Content: "x"}}
		},
		"snippet stack": func(d *DesignSystem) { d.Snippets = []DesignSnippet{{Key: "a", Name: "x", Stack: "vue", Content: "x"}} },
		"snippet empty": func(d *DesignSystem) { d.Snippets = []DesignSnippet{{Key: "a", Name: "x", Stack: StackHTML}} },
		"snippet repeated": func(d *DesignSystem) {
			d.Snippets = []DesignSnippet{{Key: "a", Name: "x", Stack: StackHTML, Content: "x"}, {Key: "a", Name: "y", Stack: StackReact, Content: "y"}}
		},
		"snippet name": func(d *DesignSystem) { d.Snippets = []DesignSnippet{{Key: "a", Stack: StackHTML, Content: "x"}} },
		"snippet too large": func(d *DesignSystem) {
			d.Snippets = []DesignSnippet{{Key: "a", Name: "x", Stack: StackHTML, Content: strings.Repeat("a", MaxProjectBytes+1)}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := validDesignSystem()
			mutate(&d)
			err := d.Validate()
			if !errors.Is(err, ErrDesignSystemInvalid) || ErrorCode(err) != CodeDesignSystemInvalid {
				t.Fatalf("got %v", err)
			}
		})
	}
}
