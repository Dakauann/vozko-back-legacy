package site

import (
	"regexp"
	"strings"
	"testing"
)

func TestTheLibraryAllowListIsOneSourceWithPinnedVersions(t *testing.T) {
	pinned := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	seen := map[string]bool{}
	for _, lib := range Libraries() {
		if seen[lib.Package] {
			t.Fatalf("%s repeats", lib.Package)
		}
		seen[lib.Package] = true
		if !pinned.MatchString(lib.Version) || len(lib.Stacks) == 0 {
			t.Fatalf("got %+v", lib)
		}
	}
	for _, want := range []string{"react", "react-dom", "framer-motion", "lucide-react", "clsx", "tailwind-merge", "class-variance-authority", "gsap", "motion", "swiper", "lottie-web", "three", "lucide"} {
		if !seen[want] {
			t.Fatalf("%s is missing", want)
		}
	}
	if lib, ok := LibraryFor(StackReact, "react-dom/client"); !ok || lib.Package != "react-dom" {
		t.Fatalf("got %+v %v", lib, ok)
	}
	if _, ok := LibraryFor(StackHTML, "react"); ok {
		t.Fatal("react is not offered to plain html pages")
	}
	if lib, ok := LibraryFor(StackHTML, "gsap/ScrollTrigger"); !ok || lib.Package != "gsap" {
		t.Fatalf("got %+v %v", lib, ok)
	}
}

func TestCheckImportsAllowsTheProjectAndTheAllowList(t *testing.T) {
	imports := []Import{
		{Importer: "src/main.jsx", Specifier: "react"},
		{Importer: "src/main.jsx", Specifier: "react-dom/client"},
		{Importer: "src/main.jsx", Specifier: "./App.jsx"},
		{Importer: "src/components/ui/button.tsx", Specifier: "@/lib/utils"},
		{Importer: "src/components/ui/button.tsx", Specifier: "../../lib/utils.ts"},
		{Importer: "src/components/ui/button.tsx", Specifier: "class-variance-authority"},
		{Importer: "src/pages/Home.jsx", Specifier: "three/addons/controls/OrbitControls.js"},
		{Importer: "src/pages/Home.jsx", Specifier: "framer-motion"},
	}
	if got := CheckImports(StackReact, imports); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestCheckImportsRefusesEverythingElse(t *testing.T) {
	cases := []Import{
		{Importer: "src/main.jsx", Specifier: "axios"},
		{Importer: "src/main.jsx", Specifier: "https://esm.sh/react"},
		{Importer: "src/main.jsx", Specifier: "//cdn.example/x.js"},
		{Importer: "src/main.jsx", Specifier: "node:fs"},
		{Importer: "src/main.jsx", Specifier: "data:text/javascript,alert(1)"},
		{Importer: "src/main.jsx", Specifier: "/etc/passwd"},
		{Importer: "src/main.jsx", Specifier: "../../server/secrets.js"},
		{Importer: "src/main.jsx", Specifier: "react/../../evil"},
		{Importer: "src/main.jsx", Specifier: "@radix-ui/react-icons"},
		{Importer: "src/main.jsx", Specifier: "embla-carousel"},
		{Importer: "src/main.jsx", Specifier: "@/../site.json"},
		{Importer: "src/main.jsx", Specifier: ""},
	}
	for _, c := range cases {
		got := CheckImports(StackReact, []Import{c})
		if len(got) != 1 || got[0].Code != CodeImportForbidden || got[0].Path != "src/main.jsx" || got[0].Detail != c.Specifier {
			t.Fatalf("%q: got %+v", c.Specifier, got)
		}
	}
	html := CheckImports(StackHTML, []Import{{Importer: "scripts/a.js", Specifier: "react"}, {Importer: "scripts/a.js", Specifier: "@/x"}, {Importer: "scripts/a.js", Specifier: "./b.js"}})
	if len(html) != 2 || !strings.Contains(html[1].Detail, "@/x") {
		t.Fatalf("got %+v", html)
	}
}

func TestTheShadcnDependenciesAreReactOnly(t *testing.T) {
	want := map[string]string{
		"@radix-ui/react-slot":            "1.5.0",
		"@radix-ui/react-accordion":       "1.3.1",
		"@radix-ui/react-dialog":          "1.3.1",
		"@radix-ui/react-tabs":            "1.2.1",
		"@radix-ui/react-dropdown-menu":   "2.2.1",
		"@radix-ui/react-popover":         "1.3.1",
		"@radix-ui/react-tooltip":         "1.4.1",
		"@radix-ui/react-select":          "2.4.1",
		"@radix-ui/react-checkbox":        "1.4.1",
		"@radix-ui/react-radio-group":     "1.5.1",
		"@radix-ui/react-switch":          "1.4.1",
		"@radix-ui/react-label":           "2.1.17",
		"@radix-ui/react-separator":       "1.2.0",
		"@radix-ui/react-avatar":          "1.3.1",
		"@radix-ui/react-navigation-menu": "1.4.1",
		"@radix-ui/react-collapsible":     "1.2.1",
		"embla-carousel-react":            "8.6.0",
	}
	found := 0
	for _, lib := range Libraries() {
		version, ok := want[lib.Package]
		if !ok {
			continue
		}
		found++
		if lib.Version != version || len(lib.Stacks) != 1 || lib.Stacks[0] != StackReact {
			t.Fatalf("got %+v", lib)
		}
		if _, ok := LibraryFor(StackHTML, lib.Package); ok {
			t.Fatalf("%s is offered to plain html pages", lib.Package)
		}
	}
	if found != len(want) {
		t.Fatalf("found %d of %d shadcn dependencies", found, len(want))
	}
	imports := []Import{
		{Importer: "src/components/ui/dialog.jsx", Specifier: "@radix-ui/react-dialog"},
		{Importer: "src/components/ui/carousel.jsx", Specifier: "embla-carousel-react"},
	}
	if got := CheckImports(StackReact, imports); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
	if got := CheckImports(StackHTML, []Import{{Importer: "scripts/a.js", Specifier: "@radix-ui/react-slot"}}); len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
}
