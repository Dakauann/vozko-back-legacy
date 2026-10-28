package site

import (
	"vozko/domain/sitehtml"
	"vozko/domain/strictjson"
)

const (
	RevisionsMeta = "vz-revs"
	MaxMarks      = 20
	MinMarkWidth  = 16
	MinMarkHeight = 12
)

func PreviewRevisions(stack Stack, files map[string]File) map[string]uint64 {
	out := map[string]uint64{}
	for path, file := range files {
		if MarkupKind(stack, path) {
			out[path] = file.Rev
		}
	}
	return out
}

func BuiltFrom(doc *sitehtml.Document, stack Stack, files map[string]File) bool {
	var stamp string
	stamps := 0
	for _, meta := range doc.Metas() {
		if meta.Name == RevisionsMeta {
			stamp, stamps = meta.Content, stamps+1
		}
	}
	if stamps != 1 {
		return false
	}
	var claimed map[string]uint64
	if strictjson.Decode([]byte(stamp), &claimed) != nil {
		return false
	}
	expected := PreviewRevisions(stack, files)
	if len(claimed) != len(expected) {
		return false
	}
	for path, rev := range expected {
		if got, ok := claimed[path]; !ok || got != rev {
			return false
		}
	}
	return true
}

func MarkRanks() [][]string {
	return [][]string{
		{"a", "button", "input", "select", "textarea", "form"},
		{"h1", "h2", "h3", "h4", "h5", "h6", "img", "picture", "video"},
		{"p", "li", "label", "blockquote", "figure"},
		{"section", "header", "footer", "nav", "article", "aside", "ul", "ol", "svg"},
	}
}
