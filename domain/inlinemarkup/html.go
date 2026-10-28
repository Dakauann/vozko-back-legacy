package inlinemarkup

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	breakingSpace = regexp.MustCompile(`[ \t\r]*\n\s*`)
	unsafeScheme  = regexp.MustCompile(`^(javascript|vbscript|data):`)
)

var blockTags = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Li: true, atom.Ul: true, atom.Ol: true, atom.Blockquote: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Section: true, atom.Article: true, atom.Header: true, atom.Footer: true,
}

var droppedTags = map[atom.Atom]bool{atom.Script: true, atom.Style: true, atom.Template: true, atom.Noscript: true}

type htmlReader struct {
	paragraphs []Paragraph
	runs       []Run
	err        error
}

func FromHTML(source string) ([]Paragraph, error) {
	nodes, err := html.ParseFragment(strings.NewReader(source), &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"})
	if err != nil {
		return nil, invalid("the html could not be read")
	}
	r := &htmlReader{}
	for _, n := range nodes {
		r.walk(n, inlineMarks{}, nil)
	}
	r.flush()
	if r.err != nil {
		return nil, r.err
	}
	return r.paragraphs, nil
}

func (r *htmlReader) walk(n *html.Node, marks inlineMarks, link *Link) {
	switch n.Type {
	case html.TextNode:
		if text := breakingSpace.ReplaceAllString(n.Data, " "); text != "" {
			r.runs = append(r.runs, Run{Text: text, Bold: marks.bold, Italic: marks.italic, Underline: marks.underline, Link: link})
		}
		return
	case html.ElementNode:
	default:
		return
	}
	if droppedTags[n.DataAtom] {
		return
	}
	switch n.DataAtom {
	case atom.Br:
		r.runs = append(r.runs, Run{Text: " ", Bold: marks.bold, Italic: marks.italic, Underline: marks.underline, Link: link})
		return
	case atom.B, atom.Strong:
		marks.bold = true
	case atom.I, atom.Em:
		marks.italic = true
	case atom.U:
		marks.underline = true
	case atom.A:
		link = r.anchor(n, link)
	}
	block := blockTags[n.DataAtom]
	if block {
		r.flush()
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		r.walk(child, marks, link)
	}
	if block {
		r.flush()
	}
}

func (r *htmlReader) anchor(n *html.Node, outer *Link) *Link {
	for _, attr := range n.Attr {
		if attr.Namespace != "" || attr.Key != "href" {
			continue
		}
		href := strings.TrimSpace(attr.Val)
		if href == "" {
			return outer
		}
		if unsafeScheme.MatchString(strings.ToLower(compactHref(href))) && r.err == nil {
			r.err = invalid("a link must not run a script")
		}
		return &Link{Href: href}
	}
	return outer
}

func compactHref(href string) string {
	var b strings.Builder
	for _, c := range href {
		if c > ' ' && c != 0x7f {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func (r *htmlReader) flush() {
	runs := mergedRuns(r.runs)
	r.runs = nil
	for _, run := range runs {
		if strings.TrimSpace(run.Text) != "" {
			r.paragraphs = append(r.paragraphs, Paragraph{Runs: runs})
			return
		}
	}
}
