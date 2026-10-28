package site

import (
	"strings"
	"testing"
)

func cssIssues(stack Stack, path, css string, files map[string]string) []string {
	all := map[string]string{path: css}
	for k, v := range files {
		all[k] = v
	}
	p := NewProject(stack, all)
	out := []string{}
	for _, issue := range CheckCSS(p, path) {
		out = append(out, issue.Code)
	}
	return out
}

func TestCSSAcceptsTailwindThemeWork(t *testing.T) {
	cases := map[string]string{
		"tailwind import":             `@import "tailwindcss";`,
		"tailwind url import":         `@import url("tailwindcss");`,
		"tailwind layers":             "@import \"tailwindcss/theme.css\" layer(theme);\n@import \"tailwindcss/utilities.css\" layer(utilities);",
		"theme and utilities":         "@theme { --color-brand: #123456; }\n@utility glass { backdrop-filter: blur(8px); }\n@custom-variant dark (&:where(.dark, .dark *));",
		"media and keyframes":         "@media (min-width: 40rem) { .a { color: red } }\n@keyframes pulse { from { opacity: 0 } }\n@font-face { font-family: X; }",
		"inline source":               `@source inline("bg-red-500 {sm:,md:,}text-xl");`,
		"negated source":              `@source not inline("container");`,
		"relative source":             `@source "./pages";`,
		"directive in string":         `.a::after { content: "@plugin 'x'"; }`,
		"directive in comment":        "/* @config \"../x.js\" */ .a { color: red }",
		"apply and layer":             "@layer components { .btn { @apply px-4 py-2; } }",
		"supports and starting style": "@supports (display: grid) { .a { display: grid } }\n@starting-style { .a { opacity: 0 } }",
	}
	for name, css := range cases {
		t.Run(name, func(t *testing.T) {
			if got := cssIssues(StackHTML, "theme.css", css, nil); len(got) != 0 {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestCSSRefusesDirectivesThatReachOutsideTheProject(t *testing.T) {
	cases := map[string]string{
		"plugin":                   `@plugin "@tailwindcss/typography";`,
		"config":                   `@config "../../tailwind.config.js";`,
		"source outside":           `@source "../../../etc";`,
		"source absolute":          `@source "/etc";`,
		"import remote":            `@import "https://evil.example/x.css";`,
		"import protocol relative": `@import url(//evil.example/x.css);`,
		"import missing file":      `@import "./nao-existe.css";`,
		"import outside":           `@import "../x.css";`,
		"import package":           `@import "bootstrap";`,
		"reference outside":        `@reference "../../app.css";`,
		"escaped keyword":          `@\70 lugin "x";`,
		"unknown at rule":          `@tailwind base;`,
		"import self":              `@import "./theme.css";`,
		"unterminated":             `@import "tailwindcss`,
	}
	for name, css := range cases {
		t.Run(name, func(t *testing.T) {
			got := cssIssues(StackHTML, "theme.css", css, nil)
			if len(got) == 0 || got[0] != CodeCSSDirectiveForbidden {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestCSSRelativeImportsResolveToProjectStylesheets(t *testing.T) {
	got := cssIssues(StackReact, "src/styles/index.css", `@import "../fonts.css"; @reference "./tokens.css";`, map[string]string{"src/fonts.css": "", "src/styles/tokens.css": ""})
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	got = cssIssues(StackReact, "src/index.css", `@import "./main.jsx";`, map[string]string{"src/main.jsx": ""})
	if len(got) != 1 {
		t.Fatalf("only stylesheets can be imported: %v", got)
	}
}

func TestCSSReportsEachForbiddenDirective(t *testing.T) {
	got := cssIssues(StackHTML, "theme.css", "@plugin \"a\";\n.a{}\n@config \"b\";", nil)
	if strings.Join(got, ",") != "css_directive_forbidden,css_directive_forbidden" {
		t.Fatalf("got %v", got)
	}
}
