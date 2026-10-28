package site

import (
	"path"
	"strings"

	"vozko/domain/sitehtml"
)

const (
	maxOutlineLabel = 60
	nodeAttribute   = "data-vz-node"
)

type OutlineNode struct {
	Node     string        `json:"node"`
	Tag      string        `json:"tag"`
	Path     string        `json:"path"`
	Label    string        `json:"label,omitempty"`
	Children []OutlineNode `json:"children,omitempty"`
}

type outlineWalk struct {
	doc      *sitehtml.Document
	maxDepth int
	left     int
}

func NodeOutline(doc *sitehtml.Document, maxDepth, maxNodes int) []OutlineNode {
	w := &outlineWalk{doc: doc, maxDepth: maxDepth, left: maxNodes}
	var out []OutlineNode
	for i, el := range doc.Elements {
		if el.Parent < 0 {
			out = append(out, w.children(i, 1, "")...)
		}
	}
	return out
}

func (w *outlineWalk) children(i, depth int, file string) []OutlineNode {
	var out []OutlineNode
	for _, child := range w.doc.Elements[i].Children {
		if w.left <= 0 {
			return out
		}
		node, annotated := w.doc.AttrValue(child, nodeAttribute)
		if !annotated || node == "" {
			out = append(out, w.children(child, depth, file)...)
			continue
		}
		if depth > w.maxDepth {
			continue
		}
		w.left--
		owner, _, _ := strings.Cut(node, "#")
		entry := OutlineNode{Node: node, Tag: w.doc.Elements[child].Tag, Path: w.doc.Elements[child].Path, Label: w.label(child, owner, file)}
		entry.Children = w.children(child, depth+1, owner)
		out = append(out, entry)
	}
	return out
}

func (w *outlineWalk) label(i int, owner, parentFile string) string {
	if alt, ok := w.doc.AttrValue(i, "alt"); ok && strings.TrimSpace(alt) != "" {
		return clip(alt)
	}
	if owner != parentFile && strings.HasPrefix(owner, "src/components/") {
		return strings.TrimSuffix(path.Base(owner), path.Ext(owner))
	}
	return clip(w.doc.Text(i))
}

func clip(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= maxOutlineLabel {
		return text
	}
	return strings.TrimSpace(string(runes[:maxOutlineLabel])) + "…"
}

type RouteOutline struct {
	Route    string        `json:"route"`
	Rendered bool          `json:"rendered"`
	Nodes    []OutlineNode `json:"nodes"`
}
