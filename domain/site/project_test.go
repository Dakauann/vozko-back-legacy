package site

import (
	"errors"
	"strings"
	"testing"

	"vozko/domain/studio"
)

const htmlSiteJSON = `{"name":"Clínica Sorriso","lang":"pt-BR","routes":[{"path":"/","page":"pages/index.html"},{"path":"/obrigado","page":"pages/obrigado.html"}]}`

const reactSiteJSON = `{"name":"Clínica Sorriso","lang":"pt-BR","routes":[{"path":"/","page":"src/pages/Home.jsx"}]}`

func htmlProject() Project {
	return NewProject(StackHTML, map[string]string{
		"site.json":            htmlSiteJSON,
		"theme.css":            "@import \"tailwindcss\";\n@theme {\n  --color-brand: oklch(0.6 0.2 250);\n  --hero-gap: 4rem;\n}\n",
		"pages/index.html":     "<section class=\"hero\">\n  <h1>Oi</h1>\n</section>\n",
		"pages/obrigado.html":  "<h1>Obrigado</h1>\n",
		"partials/header.html": "<header>Logo</header>\n",
		"scripts/motion.js":    "import { animate } from \"motion\";\n",
		"knobs.json":           `{"knobs":[{"name":"Espaço do topo","variable":"--hero-gap","kind":"range","min":0,"max":8,"step":0.5,"unit":"rem"},{"name":"Cor da marca","variable":"--color-brand","kind":"color"}]}`,
	})
}

func reactProject() Project {
	return NewProject(StackReact, map[string]string{
		"site.json":                    reactSiteJSON,
		"src/main.jsx":                 "import { createRoot } from \"react-dom/client\";\n",
		"src/index.css":                "@import \"tailwindcss\";\n@import \"./fonts.css\";\n",
		"src/fonts.css":                "@theme { --font-display: \"Inter\"; }\n",
		"src/pages/Home.jsx":           "export default function Home() { return <h1>Oi</h1> }\n",
		"src/components/ui/button.tsx": "export function Button() { return null }\n",
		"src/lib/utils.ts":             "export const cn = () => \"\"\n",
	})
}

func codes(issues []Issue) []string {
	out := []string{}
	for _, issue := range issues {
		out = append(out, issue.Code+"@"+issue.Path)
	}
	return out
}

func TestStacksAreFixedAndKnown(t *testing.T) {
	if len(Stacks()) != 2 || !StackReact.Valid() || !StackHTML.Valid() || Stack("vue").Valid() || Stack("").Valid() {
		t.Fatal("stacks")
	}
	if DefaultStack != StackReact {
		t.Fatal("react is the default stack")
	}
}

func TestPathsFollowTheLayoutOfEachStack(t *testing.T) {
	cases := []struct {
		stack Stack
		path  string
		ok    bool
	}{
		{StackHTML, "site.json", true},
		{StackHTML, "theme.css", true},
		{StackHTML, "knobs.json", true},
		{StackHTML, "pages/index.html", true},
		{StackHTML, "pages/obrigado-2.html", true},
		{StackHTML, "partials/header.html", true},
		{StackHTML, "scripts/motion.js", true},
		{StackHTML, "pages/Index.html", false},
		{StackHTML, "pages/blog/post.html", false},
		{StackHTML, "pages/index.htm", false},
		{StackHTML, "scripts/a.ts", false},
		{StackHTML, "styles/theme.css", false},
		{StackHTML, "src/main.jsx", false},
		{StackHTML, "index.html", false},
		{StackReact, "site.json", true},
		{StackReact, "knobs.json", true},
		{StackReact, "src/main.jsx", true},
		{StackReact, "src/components/ui/button.tsx", true},
		{StackReact, "src/components/Hero.jsx", true},
		{StackReact, "src/lib/utils.ts", true},
		{StackReact, "src/index.css", true},
		{StackReact, "src/data.json", false},
		{StackReact, "index.html", false},
		{StackReact, "public/logo.png", false},
		{StackReact, "theme.css", false},
		{StackReact, "src/.hidden.js", false},
		{StackReact, "src/../site.json", false},
		{StackReact, "src//a.js", false},
		{StackReact, "/src/a.js", false},
		{StackReact, "src\\a.js", false},
		{StackReact, "src/a.js/", false},
		{StackReact, "src/ação.js", false},
		{StackReact, "src/a b.js", false},
		{StackReact, "src/" + strings.Repeat("a", 300) + ".js", false},
		{Stack("vue"), "site.json", false},
	}
	for _, c := range cases {
		err := ValidatePath(c.stack, c.path)
		if (err == nil) != c.ok {
			t.Fatalf("%s %q: got %v", c.stack, c.path, err)
		}
		if err != nil && !errors.Is(err, ErrPathInvalid) {
			t.Fatalf("%s %q: got %v", c.stack, c.path, err)
		}
	}
}

func TestTheProjectBoundIsTheStudioDocumentBound(t *testing.T) {
	if MaxProjectBytes != studio.MaxDocumentBytes {
		t.Fatalf("got %d", MaxProjectBytes)
	}
}

func TestValidProjectsOfBothStacksHaveNoIssues(t *testing.T) {
	for name, p := range map[string]Project{"html": htmlProject(), "react": reactProject()} {
		if issues := Validate(p); len(issues) != 0 {
			t.Fatalf("%s: %v", name, codes(issues))
		}
	}
}

func TestValidateReportsEveryStructuralIssueAtOnce(t *testing.T) {
	p := htmlProject()
	p.Files["pages/Bad.html"] = File{Content: "<p>x</p>", Rev: 1}
	p.Files["theme.css"] = File{Content: "@import \"tailwindcss\";\n@plugin \"../server.js\";\n", Rev: 1}
	p.Files["site.json"] = File{Content: `{"name":"x","routes":[{"path":"/","page":"pages/index.html"},{"path":"/sumiu","page":"pages/sumiu.html"}]}`, Rev: 1}
	p.Files["knobs.json"] = File{Content: `{"knobs":[{"name":"a","variable":"--nao-existe","kind":"color"}]}`, Rev: 1}
	got := strings.Join(codes(Validate(p)), ",")
	for _, want := range []string{"path_invalid@pages/Bad.html", "css_directive_forbidden@theme.css", "route_page_missing@site.json", "knob_variable_unknown@knobs.json"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
}

func TestValidateRequiresTheFilesEachStackNeeds(t *testing.T) {
	html := htmlProject()
	delete(html.Files, "theme.css")
	delete(html.Files, "knobs.json")
	if got := codes(Validate(html)); len(got) != 1 || got[0] != "file_required@theme.css" {
		t.Fatalf("got %v", got)
	}
	react := reactProject()
	delete(react.Files, "src/main.jsx")
	if got := codes(Validate(react)); len(got) != 1 || got[0] != "file_required@src/main.jsx" {
		t.Fatalf("got %v", got)
	}
	tsx := reactProject()
	delete(tsx.Files, "src/main.jsx")
	tsx.Files["src/main.tsx"] = File{Content: "export {}\n", Rev: 1}
	if got := codes(Validate(tsx)); len(got) != 0 {
		t.Fatalf("a typescript entry is an entry: %v", got)
	}
	both := reactProject()
	both.Files["src/main.tsx"] = File{Content: "export {}\n", Rev: 1}
	if got := codes(Validate(both)); len(got) != 1 || got[0] != "entry_ambiguous@src/main.tsx" {
		t.Fatalf("got %v", got)
	}
	none := NewProject(StackHTML, map[string]string{})
	if got := codes(Validate(none)); len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	unknown := NewProject("vue", map[string]string{})
	if got := codes(Validate(unknown)); len(got) != 1 || got[0] != "stack_invalid@" {
		t.Fatalf("got %v", got)
	}
}

func TestValidateRefusesPathsThatCollideOnCaseInsensitiveDisks(t *testing.T) {
	p := reactProject()
	p.Files["src/components/hero.jsx"] = File{Content: "export {}", Rev: 1}
	p.Files["src/components/Hero.jsx"] = File{Content: "export {}", Rev: 1}
	if got := codes(Validate(p)); len(got) != 1 || got[0] != "path_duplicate@src/components/hero.jsx" {
		t.Fatalf("got %v", got)
	}
}

func TestValidateBoundsTheWholeProjectNotTheCount(t *testing.T) {
	p := reactProject()
	for i := 0; i < 300; i++ {
		p.Files["src/components/C"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+strings.Repeat("y", i/26)+".jsx"] = File{Content: "export {}", Rev: 1}
	}
	if got := codes(Validate(p)); len(got) != 0 {
		t.Fatalf("no count caps: %v", got)
	}
	big := reactProject()
	big.Files["src/a.js"] = File{Content: strings.Repeat("a", MaxProjectBytes/2), Rev: 1}
	big.Files["src/b.js"] = File{Content: strings.Repeat("b", MaxProjectBytes/2), Rev: 1}
	if got := codes(Validate(big)); len(got) != 1 || got[0] != "project_too_large@" {
		t.Fatalf("got %v", got)
	}
	huge := reactProject()
	huge.Files["src/a.js"] = File{Content: strings.Repeat("a", MaxProjectBytes+1), Rev: 1}
	if got := codes(Validate(huge)); len(got) != 2 || got[0] != "file_too_large@src/a.js" {
		t.Fatalf("got %v", got)
	}
	if reactProject().Size() <= 0 {
		t.Fatal("size")
	}
}

func TestPathsAreListedInOrder(t *testing.T) {
	got := strings.Join(htmlProject().Paths(), ",")
	if got != "knobs.json,pages/index.html,pages/obrigado.html,partials/header.html,scripts/motion.js,site.json,theme.css" {
		t.Fatalf("got %s", got)
	}
}
