package sitehtml

import (
	"errors"
	"io"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const AnnotationAttr = "data-vz-node"

var (
	ErrMalformed = errors.New("sitehtml: the rendered html is malformed")
	ErrNodeID    = errors.New("sitehtml: the node id is malformed")
)

type ParseError struct {
	Offset int
	Reason string
}

func (e *ParseError) Error() string {
	return ErrMalformed.Error() + " at byte " + strconv.Itoa(e.Offset) + ": " + e.Reason
}

func (e *ParseError) Is(target error) bool {
	return target == ErrMalformed
}

type Attr struct {
	Name     string
	Value    string
	HasValue bool
	Start    int
	End      int
}

type Element struct {
	Tag         string
	Start       int
	StartTagEnd int
	InnerStart  int
	InnerEnd    int
	End         int
	Void        bool
	SelfClosing bool
	Attrs       []Attr
	Node        string
	Parent      int
	Children    []int
	Path        string
}

type Document struct {
	Source   string
	Elements []Element
	byPath   map[string]int
}

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true,
}

var foreignRoots = map[string]bool{"svg": true, "math": true}

func NodeID(file string, offset int) string {
	return file + "#" + strconv.Itoa(offset)
}

func ParseNodeID(id string) (string, int, error) {
	at := strings.LastIndexByte(id, '#')
	if at <= 0 || at == len(id)-1 {
		return "", 0, ErrNodeID
	}
	file, digits := id[:at], id[at+1:]
	if strings.TrimSpace(file) != file || (len(digits) > 1 && digits[0] == '0') {
		return "", 0, ErrNodeID
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return "", 0, ErrNodeID
		}
	}
	offset, err := strconv.Atoi(digits)
	if err != nil {
		return "", 0, ErrNodeID
	}
	return file, offset, nil
}

type parser struct {
	doc     *Document
	stack   []int
	foreign int
}

func Parse(source string) (*Document, error) {
	p := &parser{doc: &Document{Source: source, byPath: map[string]int{}}}
	z := html.NewTokenizer(strings.NewReader(source))
	offset := 0
	for {
		tt := z.Next()
		raw := len(z.Raw())
		start := offset
		offset += raw
		switch tt {
		case html.ErrorToken:
			if !errors.Is(z.Err(), io.EOF) {
				return nil, &ParseError{Offset: start, Reason: z.Err().Error()}
			}
			if offset != len(source) {
				return nil, &ParseError{Offset: offset, Reason: "the tokenizer lost bytes"}
			}
			if len(p.stack) > 0 {
				open := p.doc.Elements[p.stack[len(p.stack)-1]]
				return nil, &ParseError{Offset: open.Start, Reason: "<" + open.Tag + "> is not closed"}
			}
			p.doc.index()
			return p.doc, nil
		case html.StartTagToken, html.SelfClosingTagToken:
			if err := p.open(z, start, offset, tt == html.SelfClosingTagToken); err != nil {
				return nil, err
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if err := p.close(string(name), start, offset); err != nil {
				return nil, err
			}
		}
	}
}

func (p *parser) open(z *html.Tokenizer, start, end int, selfClosing bool) error {
	name, hasAttr := z.TagName()
	tag := string(name)
	var tokenAttrs []string
	for hasAttr {
		var key []byte
		key, _, hasAttr = z.TagAttr()
		tokenAttrs = append(tokenAttrs, string(key))
	}
	attrs, err := scanAttrs(p.doc.Source, start, end)
	if err != nil {
		return err
	}
	if len(attrs) != len(tokenAttrs) {
		return &ParseError{Offset: start, Reason: "the attributes of <" + tag + "> are unreadable"}
	}
	seen := map[string]bool{}
	node := ""
	for i, attr := range attrs {
		if attr.Name != tokenAttrs[i] {
			return &ParseError{Offset: attr.Start, Reason: "the attributes of <" + tag + "> are unreadable"}
		}
		if seen[attr.Name] {
			return &ParseError{Offset: attr.Start, Reason: "<" + tag + "> repeats the attribute " + attr.Name}
		}
		seen[attr.Name] = true
		if attr.Name == AnnotationAttr {
			if _, _, err := ParseNodeID(attr.Value); err != nil {
				return &ParseError{Offset: attr.Start, Reason: "the node annotation " + strconv.Quote(attr.Value) + " is malformed"}
			}
			node = attr.Value
		}
	}
	void := voidElements[tag]
	if selfClosing && !void && p.foreign == 0 {
		return &ParseError{Offset: start, Reason: "<" + tag + "/> is not a void element"}
	}
	parent := -1
	if len(p.stack) > 0 {
		parent = p.stack[len(p.stack)-1]
	}
	index := len(p.doc.Elements)
	el := Element{Tag: tag, Start: start, StartTagEnd: end, InnerStart: end, InnerEnd: end, End: end, Void: void, SelfClosing: selfClosing, Attrs: attrs, Node: node, Parent: parent}
	p.doc.Elements = append(p.doc.Elements, el)
	if parent >= 0 {
		p.doc.Elements[parent].Children = append(p.doc.Elements[parent].Children, index)
	}
	if void || selfClosing {
		return nil
	}
	p.stack = append(p.stack, index)
	if foreignRoots[tag] {
		p.foreign++
	}
	return nil
}

func (p *parser) close(tag string, start, end int) error {
	if voidElements[tag] {
		return &ParseError{Offset: start, Reason: "</" + tag + "> closes a void element"}
	}
	if len(p.stack) == 0 {
		return &ParseError{Offset: start, Reason: "</" + tag + "> closes nothing"}
	}
	top := p.stack[len(p.stack)-1]
	el := &p.doc.Elements[top]
	if el.Tag != tag {
		return &ParseError{Offset: start, Reason: "</" + tag + "> closes <" + el.Tag + ">"}
	}
	el.InnerEnd = start
	el.End = end
	p.stack = p.stack[:len(p.stack)-1]
	if foreignRoots[tag] {
		p.foreign--
	}
	return nil
}

func (d *Document) index() {
	counts := map[int]map[string]int{}
	for i := range d.Elements {
		el := &d.Elements[i]
		if counts[el.Parent] == nil {
			counts[el.Parent] = map[string]int{}
		}
		n := counts[el.Parent][el.Tag]
		counts[el.Parent][el.Tag] = n + 1
		segment := el.Tag + "[" + strconv.Itoa(n) + "]"
		if el.Parent >= 0 {
			segment = d.Elements[el.Parent].Path + "/" + segment
		}
		el.Path = segment
		d.byPath[segment] = i
	}
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func scanAttrs(source string, start, end int) ([]Attr, error) {
	i := start + 1
	for i < end && !isSpace(source[i]) && source[i] != '/' && source[i] != '>' {
		i++
	}
	var attrs []Attr
	for {
		for i < end && (isSpace(source[i]) || source[i] == '/') {
			i++
		}
		if i >= end || source[i] == '>' {
			return attrs, nil
		}
		nameStart := i
		i++
		for i < end && !isSpace(source[i]) && source[i] != '/' && source[i] != '>' && source[i] != '=' {
			i++
		}
		attr := Attr{Name: strings.ToLower(source[nameStart:i]), Start: nameStart}
		valueAt := i
		for valueAt < end && isSpace(source[valueAt]) {
			valueAt++
		}
		if valueAt < end && source[valueAt] == '=' {
			i = valueAt + 1
			for i < end && isSpace(source[i]) {
				i++
			}
			if i >= end {
				return nil, &ParseError{Offset: nameStart, Reason: "an attribute value is not closed"}
			}
			attr.HasValue = true
			switch quote := source[i]; quote {
			case '"', '\'':
				close := strings.IndexByte(source[i+1:end], quote)
				if close < 0 {
					return nil, &ParseError{Offset: nameStart, Reason: "an attribute value is not closed"}
				}
				attr.Value = html.UnescapeString(source[i+1 : i+1+close])
				i = i + 1 + close + 1
			default:
				valueStart := i
				for i < end && !isSpace(source[i]) && source[i] != '>' {
					i++
				}
				attr.Value = html.UnescapeString(source[valueStart:i])
			}
		}
		attr.End = i
		attrs = append(attrs, attr)
	}
}

func (d *Document) ByPath(path string) (int, bool) {
	i, ok := d.byPath[path]
	return i, ok
}

func (d *Document) ByTag(tags ...string) []int {
	var out []int
	for i, el := range d.Elements {
		for _, tag := range tags {
			if el.Tag == tag {
				out = append(out, i)
				break
			}
		}
	}
	return out
}

func (d *Document) byNode(node string) []int {
	var out []int
	for i, el := range d.Elements {
		if el.Node == node {
			out = append(out, i)
		}
	}
	return out
}

func (d *Document) Ancestor(i int, tags ...string) (int, bool) {
	for parent := d.Elements[i].Parent; parent >= 0; parent = d.Elements[parent].Parent {
		for _, tag := range tags {
			if d.Elements[parent].Tag == tag {
				return parent, true
			}
		}
	}
	return -1, false
}

func (d *Document) Descendants(i int, tags ...string) []int {
	el := d.Elements[i]
	var out []int
	for j := i + 1; j < len(d.Elements) && d.Elements[j].Start < el.End; j++ {
		for _, tag := range tags {
			if d.Elements[j].Tag == tag {
				out = append(out, j)
				break
			}
		}
	}
	return out
}

func (d *Document) Attr(i int, name string) (Attr, bool) {
	name = strings.ToLower(name)
	for _, attr := range d.Elements[i].Attrs {
		if attr.Name == name {
			return attr, true
		}
	}
	return Attr{}, false
}

func (d *Document) AttrValue(i int, name string) (string, bool) {
	attr, ok := d.Attr(i, name)
	return attr.Value, ok
}

func (d *Document) HasAttr(i int, name string) bool {
	_, ok := d.Attr(i, name)
	return ok
}

var hiddenText = map[string]bool{"script": true, "style": true, "template": true, "noscript": true}

func (d *Document) Text(i int) string {
	el := d.Elements[i]
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(d.Source[el.InnerStart:el.InnerEnd]))
	hidden := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.Join(strings.Fields(b.String()), " ")
		case html.StartTagToken:
			name, _ := z.TagName()
			if hiddenText[string(name)] {
				hidden++
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if hiddenText[string(name)] && hidden > 0 {
				hidden--
			}
		case html.TextToken:
			if hidden == 0 {
				b.Write(z.Text())
			}
		}
	}
}
