package site

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

type Stack string

const (
	StackReact Stack = "react"
	StackHTML  Stack = "html"

	DefaultStack = StackReact
)

func Stacks() []Stack {
	return []Stack{StackReact, StackHTML}
}

func (s Stack) Valid() bool {
	return s == StackReact || s == StackHTML
}

const (
	ManifestPath  = "site.json"
	KnobsPath     = "knobs.json"
	ThemePath     = "theme.css"
	ReactEntryJSX = "src/main.jsx"
	ReactEntryTSX = "src/main.tsx"
	maxPathLength = 200
)

var (
	ErrStackInvalid = errors.New("site: the stack is not react or html")
	ErrPathInvalid  = errors.New("site: the path is outside the project layout")
)

var (
	htmlName  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	reactName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
)

type FileKind string

const (
	KindManifest FileKind = "manifest"
	KindKnobs    FileKind = "knobs"
	KindStyle    FileKind = "style"
	KindPage     FileKind = "page"
	KindPartial  FileKind = "partial"
	KindScript   FileKind = "script"
	KindModule   FileKind = "module"
)

var htmlFolders = map[string]struct {
	ext  string
	kind FileKind
}{
	"pages":    {".html", KindPage},
	"partials": {".html", KindPartial},
	"scripts":  {".js", KindScript},
}

var reactExtensions = map[string]FileKind{
	".jsx": KindModule,
	".tsx": KindModule,
	".js":  KindModule,
	".ts":  KindModule,
	".css": KindStyle,
}

func ValidatePath(stack Stack, p string) error {
	_, err := KindOf(stack, p)
	return err
}

func KindOf(stack Stack, p string) (FileKind, error) {
	if !stack.Valid() {
		return "", fmt.Errorf("%w: %w", ErrPathInvalid, ErrStackInvalid)
	}
	if p == "" || len(p) > maxPathLength || strings.ContainsAny(p, "\\\x00") || path.Clean(p) != p || strings.HasPrefix(p, "/") {
		return "", ErrPathInvalid
	}
	switch p {
	case ManifestPath:
		return KindManifest, nil
	case KnobsPath:
		return KindKnobs, nil
	}
	if stack == StackHTML {
		return htmlKind(p)
	}
	return reactKind(p)
}

func htmlKind(p string) (FileKind, error) {
	if p == ThemePath {
		return KindStyle, nil
	}
	folder, file, ok := strings.Cut(p, "/")
	rule, known := htmlFolders[folder]
	if !ok || !known || strings.Contains(file, "/") {
		return "", ErrPathInvalid
	}
	name, found := strings.CutSuffix(file, rule.ext)
	if !found || !htmlName.MatchString(name) {
		return "", ErrPathInvalid
	}
	return rule.kind, nil
}

func reactKind(p string) (FileKind, error) {
	segments := strings.Split(p, "/")
	if len(segments) < 2 || segments[0] != "src" {
		return "", ErrPathInvalid
	}
	for _, segment := range segments[1:] {
		if !reactName.MatchString(segment) || strings.Contains(segment, "..") {
			return "", ErrPathInvalid
		}
	}
	kind, ok := reactExtensions[path.Ext(p)]
	if !ok || strings.TrimSuffix(segments[len(segments)-1], path.Ext(p)) == "" {
		return "", ErrPathInvalid
	}
	return kind, nil
}

func PageKind(stack Stack, p string) bool {
	kind, err := KindOf(stack, p)
	if err != nil {
		return false
	}
	if stack == StackHTML {
		return kind == KindPage
	}
	ext := path.Ext(p)
	return kind == KindModule && (ext == ".jsx" || ext == ".tsx")
}

func MarkupKind(stack Stack, p string) bool {
	kind, err := KindOf(stack, p)
	if err != nil {
		return false
	}
	return kind == KindPage || kind == KindPartial || kind == KindModule
}

func requiredPaths(stack Stack) []string {
	if stack == StackHTML {
		return []string{ManifestPath, ThemePath}
	}
	return []string{ManifestPath}
}
