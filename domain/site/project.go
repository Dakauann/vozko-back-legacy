package site

import (
	"regexp"
	"slices"
	"strings"

	"vozko/domain/studio"
)

const MaxProjectBytes = studio.MaxDocumentBytes

type File struct {
	Content string `json:"content"`
	Rev     uint64 `json:"rev"`
}

type Project struct {
	Stack Stack           `json:"stack"`
	Files map[string]File `json:"files"`
}

func NewProject(stack Stack, contents map[string]string) Project {
	files := make(map[string]File, len(contents))
	for path, content := range contents {
		files[path] = File{Content: content, Rev: 1}
	}
	return Project{Stack: stack, Files: files}
}

func (p Project) Content(path string) (string, bool) {
	file, ok := p.Files[path]
	return file.Content, ok
}

func (p Project) Paths() []string {
	out := make([]string, 0, len(p.Files))
	for path := range p.Files {
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}

func (p Project) Size() int {
	total := 0
	for path, file := range p.Files {
		total += len(path) + len(file.Content)
	}
	return total
}

func (p Project) Clone() Project {
	files := make(map[string]File, len(p.Files))
	for path, file := range p.Files {
		files[path] = file
	}
	return Project{Stack: p.Stack, Files: files}
}

func (p Project) Has(path string) bool {
	_, ok := p.Files[path]
	return ok
}

func Validate(p Project) []Issue {
	if !p.Stack.Valid() {
		return []Issue{{Code: CodeStackInvalid}}
	}
	var issues []Issue
	folded := map[string]string{}
	for _, path := range p.Paths() {
		if err := ValidatePath(p.Stack, path); err != nil {
			issues = append(issues, Issue{Code: CodePathInvalid, Path: path})
			continue
		}
		key := strings.ToLower(path)
		if first, taken := folded[key]; taken {
			issues = append(issues, Issue{Code: CodePathDuplicate, Path: path, Detail: first})
			continue
		}
		folded[key] = path
		if len(p.Files[path].Content) > MaxProjectBytes {
			issues = append(issues, Issue{Code: CodeFileTooLarge, Path: path})
		}
	}
	if p.Size() > MaxProjectBytes {
		issues = append(issues, Issue{Code: CodeProjectTooLarge})
	}
	issues = append(issues, requiredIssues(p)...)
	issues = append(issues, manifestIssues(p)...)
	issues = append(issues, knobsIssues(p)...)
	for _, path := range p.Paths() {
		if kind, err := KindOf(p.Stack, path); err == nil && kind == KindStyle {
			issues = append(issues, CheckCSS(p, path)...)
		}
	}
	return issues
}

func requiredIssues(p Project) []Issue {
	var issues []Issue
	for _, path := range requiredPaths(p.Stack) {
		if !p.Has(path) {
			issues = append(issues, Issue{Code: CodeFileRequired, Path: path})
		}
	}
	if p.Stack != StackReact {
		return issues
	}
	jsx, tsx := p.Has(ReactEntryJSX), p.Has(ReactEntryTSX)
	switch {
	case jsx && tsx:
		issues = append(issues, Issue{Code: CodeEntryAmbiguous, Path: ReactEntryTSX})
	case !jsx && !tsx:
		issues = append(issues, Issue{Code: CodeFileRequired, Path: ReactEntryJSX})
	}
	return issues
}

func manifestIssues(p Project) []Issue {
	raw, ok := p.Content(ManifestPath)
	if !ok {
		return nil
	}
	m, err := DecodeManifest(p.Stack, []byte(raw))
	if err != nil {
		return []Issue{{Code: CodeManifestInvalid, Path: ManifestPath, Detail: err.Error()}}
	}
	var issues []Issue
	for _, route := range m.Routes {
		if !p.Has(route.Page) {
			issues = append(issues, Issue{Code: CodeRoutePageMissing, Path: ManifestPath, Route: route.Path, Detail: route.Page})
		}
	}
	return issues
}

var customProperty = regexp.MustCompile(`(?m)(--[a-zA-Z0-9_-]+)\s*:`)

func declaredVariables(p Project) map[string]bool {
	out := map[string]bool{}
	for path, file := range p.Files {
		if kind, err := KindOf(p.Stack, path); err != nil || kind != KindStyle {
			continue
		}
		for _, match := range customProperty.FindAllStringSubmatch(stripCSSComments(file.Content), -1) {
			out[match[1]] = true
		}
	}
	return out
}

func knobsIssues(p Project) []Issue {
	raw, ok := p.Content(KnobsPath)
	if !ok {
		return nil
	}
	knobs, err := DecodeKnobs([]byte(raw))
	if err != nil {
		return []Issue{{Code: CodeKnobsInvalid, Path: KnobsPath, Detail: err.Error()}}
	}
	declared := declaredVariables(p)
	var issues []Issue
	for _, knob := range knobs.Knobs {
		if !declared[knob.Variable] {
			issues = append(issues, Issue{Code: CodeKnobVariableUnknown, Path: KnobsPath, Detail: knob.Variable})
		}
	}
	return issues
}
