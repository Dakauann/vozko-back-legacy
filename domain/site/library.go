package site

import (
	"path"
	"slices"
	"strings"
)

type Library struct {
	Package string  `json:"package"`
	Version string  `json:"version"`
	Stacks  []Stack `json:"stacks"`
}

var bothStacks = []Stack{StackReact, StackHTML}

var libraries = []Library{
	{Package: "react", Version: "19.2.0", Stacks: []Stack{StackReact}},
	{Package: "react-dom", Version: "19.2.0", Stacks: []Stack{StackReact}},
	{Package: "framer-motion", Version: "12.23.12", Stacks: []Stack{StackReact}},
	{Package: "lucide-react", Version: "0.544.0", Stacks: []Stack{StackReact}},
	{Package: "clsx", Version: "2.1.1", Stacks: []Stack{StackReact}},
	{Package: "tailwind-merge", Version: "3.3.1", Stacks: []Stack{StackReact}},
	{Package: "class-variance-authority", Version: "0.7.1", Stacks: []Stack{StackReact}},
	{Package: "gsap", Version: "3.13.0", Stacks: bothStacks},
	{Package: "motion", Version: "12.23.12", Stacks: bothStacks},
	{Package: "swiper", Version: "11.2.10", Stacks: bothStacks},
	{Package: "lottie-web", Version: "5.12.2", Stacks: bothStacks},
	{Package: "three", Version: "0.180.0", Stacks: bothStacks},
	{Package: "lucide", Version: "0.544.0", Stacks: bothStacks},
	{Package: "@radix-ui/react-slot", Version: "1.5.0", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-accordion", Version: "1.3.1", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-dialog", Version: "1.3.1", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-tabs", Version: "1.2.1", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-dropdown-menu", Version: "2.2.1", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-popover", Version: "1.3.1", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-tooltip", Version: "1.4.1", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-select", Version: "2.4.1", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-checkbox", Version: "1.4.1", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-radio-group", Version: "1.5.1", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-switch", Version: "1.4.1", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-label", Version: "2.1.17", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-separator", Version: "1.2.0", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-avatar", Version: "1.3.1", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-navigation-menu", Version: "1.4.1", Stacks: []Stack{StackReact}},
	{Package: "@radix-ui/react-collapsible", Version: "1.2.1", Stacks: []Stack{StackReact}},
	{Package: "embla-carousel-react", Version: "8.6.0", Stacks: []Stack{StackReact}},
}

const projectAlias = "@/"

func Libraries() []Library {
	out := make([]Library, len(libraries))
	for i, lib := range libraries {
		lib.Stacks = slices.Clone(lib.Stacks)
		out[i] = lib
	}
	return out
}

func packageOf(specifier string) string {
	parts := strings.SplitN(specifier, "/", 3)
	if strings.HasPrefix(specifier, "@") && len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

func LibraryFor(stack Stack, specifier string) (Library, bool) {
	if specifier == "" || strings.Contains(specifier, "..") || strings.Contains(specifier, ":") || strings.Contains(specifier, "\\") {
		return Library{}, false
	}
	name := packageOf(specifier)
	for _, lib := range libraries {
		if lib.Package == name && slices.Contains(lib.Stacks, stack) {
			return lib, true
		}
	}
	return Library{}, false
}

type Import struct {
	Importer  string `json:"importer"`
	Specifier string `json:"specifier"`
}

func CheckImports(stack Stack, imports []Import) []Issue {
	var issues []Issue
	for _, imp := range imports {
		if !importAllowed(stack, imp) {
			issues = append(issues, Issue{Code: CodeImportForbidden, Path: imp.Importer, Detail: imp.Specifier})
		}
	}
	return issues
}

func importAllowed(stack Stack, imp Import) bool {
	spec := imp.Specifier
	switch {
	case spec == "" || strings.ContainsAny(spec, "\\\x00") || strings.Contains(spec, ":") || strings.HasPrefix(spec, "/"):
		return false
	case relativeTarget(spec):
		resolved := path.Join(path.Dir(imp.Importer), spec)
		root := "scripts/"
		if stack == StackReact {
			root = "src/"
		}
		return insideProject(resolved) && strings.HasPrefix(resolved, root)
	case strings.HasPrefix(spec, projectAlias):
		if stack != StackReact {
			return false
		}
		resolved := path.Join("src", strings.TrimPrefix(spec, projectAlias))
		return !strings.Contains(spec, "..") && strings.HasPrefix(resolved, "src/")
	}
	_, ok := LibraryFor(stack, spec)
	return ok
}
