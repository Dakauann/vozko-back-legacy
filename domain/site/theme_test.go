package site

import (
	"errors"
	"testing"
)

func TestThemeFileFollowsTheBuilderChoice(t *testing.T) {
	if path, ok := ThemeFile(htmlProject()); !ok || path != ThemePath {
		t.Fatalf("got %q %v", path, ok)
	}
	if path, ok := ThemeFile(reactProject()); !ok || path != "src/fonts.css" {
		t.Fatalf("a react page keeps its theme where the @theme block is: %q %v", path, ok)
	}
	p := reactProject()
	p.Files["src/fonts.css"] = File{Content: ".a { color: red }\n", Rev: 1}
	if path, ok := ThemeFile(p); !ok || path != "src/index.css" {
		t.Fatalf("without a block the tailwind entry holds the theme: %q %v", path, ok)
	}
	delete(p.Files, "src/index.css")
	if _, ok := ThemeFile(p); ok {
		t.Fatal("no theme file")
	}
}

func TestThemeTokensReadTheThemeBlocks(t *testing.T) {
	p := htmlProject()
	p.Files[ThemePath] = File{Rev: 3, Content: `@import "tailwindcss";
/* @theme { --ignored: 1; } */
@theme inline {
  --color-brand: oklch(0.6 0.2 250);
  --font-sans: "Inter; Sans", ui-sans-serif;
  @keyframes pulse { 50% { --inner: 1; } }
  --radius-card: 1.25rem
}
:root { --outside: 2px; }
@theme { --color-brand: red; --hero-gap: 4rem; }
`}
	got := ThemeTokens(p)
	want := []Token{
		{Name: "--color-brand", Value: "oklch(0.6 0.2 250)", Path: ThemePath},
		{Name: "--font-sans", Value: `"Inter; Sans", ui-sans-serif`, Path: ThemePath},
		{Name: "--radius-card", Value: "1.25rem", Path: ThemePath},
		{Name: "--hero-gap", Value: "4rem", Path: ThemePath},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d: got %+v want %+v", i, got[i], want[i])
		}
	}
	if tokens := ThemeTokens(Project{Stack: StackHTML, Files: map[string]File{}}); tokens == nil || len(tokens) != 0 {
		t.Fatal("no theme reads as an empty list")
	}
}

func TestKnobStatesCarryTheCurrentValues(t *testing.T) {
	p := htmlProject()
	p.Files[ThemePath] = File{Rev: 2, Content: "@theme {\n  --color-brand: #ff0066;\n}\n:root { --hero-gap: 2.5rem; }\n"}
	states, err := KnobStates(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[0].Name != "Espaço do topo" || states[0].Number == nil || *states[0].Number != 2.5 || states[0].Color != nil {
		t.Fatalf("got %+v", states)
	}
	if states[1].Color == nil || *states[1].Color != "#ff0066" || states[1].Number != nil {
		t.Fatalf("got %+v", states[1])
	}
	p.Files[ThemePath] = File{Rev: 3, Content: ":root { --hero-gap: 3px; }\n"}
	states, err = KnobStates(p)
	if err != nil || states[0].Number != nil || states[1].Color != nil {
		t.Fatalf("a value outside the knob schema is unknown: %+v %v", states, err)
	}
	delete(p.Files, KnobsPath)
	if states, err := KnobStates(p); err != nil || states == nil || len(states) != 0 {
		t.Fatalf("got %+v %v", states, err)
	}
	p.Files[KnobsPath] = File{Rev: 1, Content: "{"}
	if _, err := KnobStates(p); !errors.Is(err, ErrKnobsInvalid) {
		t.Fatalf("got %v", err)
	}
}
