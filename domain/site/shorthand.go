package site

import (
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"vozko/domain/sitehtml"
)

const whatsAppPlaceholderHref = "#"

type NumberUse struct {
	Index int
	Ref   string
	Field string
}

func (s *batchState) expand(i int, o Operation) ([]Operation, *Failure) {
	switch o.Op {
	case OpDefineKnob, OpRenameKnob, OpRemoveKnob:
		return s.expandKnob(i, o)
	case OpSetWhatsAppLink:
		return expandWhatsAppLink(i, o)
	case OpClearWhatsAppLink:
		return []Operation{attrOp(o, WhatsAppNumberAttr, nil), attrOp(o, WhatsAppTextAttr, nil)}, nil
	case OpInsertElement:
		return expandElement(i, o)
	case OpSetFormField:
		return expandSetFormField(i, o)
	case OpInsertFormField:
		return expandInsertFormField(i, o)
	}
	return []Operation{o}, nil
}

func attrOp(o Operation, name string, value *string) Operation {
	return Operation{Op: OpSetAttr, Node: o.Node, Rev: o.Rev, Name: name, Value: value}
}

func expandWhatsAppLink(i int, o Operation) ([]Operation, *Failure) {
	if o.NumberRef == "" {
		return nil, failure(i, CodeFieldRequired, "number_ref")
	}
	if !numberRef.MatchString(o.NumberRef) {
		return nil, failure(i, CodeFieldInvalid, "number_ref")
	}
	text := ""
	if o.Text != nil {
		text = strings.TrimSpace(*o.Text)
	}
	if utf8.RuneCountInString(text) > MaxWhatsAppTextRunes {
		return nil, failure(i, CodeWhatsAppTextTooLong, "text")
	}
	ref, href := o.NumberRef, whatsAppPlaceholderHref
	message := attrOp(o, WhatsAppTextAttr, nil)
	if text != "" {
		message.Value = &text
	}
	return []Operation{attrOp(o, WhatsAppNumberAttr, &ref), message, attrOp(o, "href", &href)}, nil
}

func (s *batchState) expandKnob(i int, o Operation) ([]Operation, *Failure) {
	if o.Name == "" {
		return nil, failure(i, CodeFieldRequired, "name")
	}
	if !s.knobsKnown {
		return nil, failure(i, CodeKnobsInvalid, "name")
	}
	list := slices.Clone(s.knobs.Knobs)
	at := slices.IndexFunc(list, func(k Knob) bool { return k.Name == o.Name })
	switch o.Op {
	case OpDefineKnob:
		if o.Kind == "" {
			return nil, failure(i, CodeFieldRequired, "kind")
		}
		if at >= 0 {
			return nil, failure(i, CodeKnobExists, "name")
		}
		if slices.ContainsFunc(list, func(k Knob) bool { return k.Variable == o.Variable }) {
			return nil, failure(i, CodeKnobExists, "variable")
		}
		knob := Knob{Name: o.Name, Variable: o.Variable, Kind: o.Kind, Min: o.Min, Max: o.Max, Step: o.Step, Unit: o.Unit}
		if field, reason := knob.problem(); reason != "" {
			return nil, &Failure{Index: i, Code: CodeKnobsInvalid, Field: field, Detail: reason}
		}
		list = append(list, knob)
	case OpRenameKnob:
		if o.To == "" {
			return nil, failure(i, CodeFieldRequired, "to")
		}
		if at < 0 {
			return nil, failure(i, CodeKnobUnknown, "name")
		}
		if _, taken := s.knobs.Find(o.To); taken {
			return nil, failure(i, CodeKnobExists, "to")
		}
		list[at].Name = o.To
		if _, reason := list[at].problem(); reason != "" {
			return nil, &Failure{Index: i, Code: CodeKnobsInvalid, Field: "to", Detail: reason}
		}
	case OpRemoveKnob:
		if at < 0 {
			return nil, failure(i, CodeKnobUnknown, "name")
		}
		list = slices.Delete(list, at, at+1)
	}
	content, err := Knobs{Knobs: list}.Encode()
	if err != nil {
		return nil, failure(i, CodeKnobsInvalid, "name")
	}
	write := Operation{Op: OpWriteFile, Path: KnobsPath, Content: &content}
	if base, ok := s.p.Files[KnobsPath]; ok {
		write.Rev = base.Rev
	}
	return []Operation{write}, nil
}

func (b Batch) WhatsAppNumbers() []NumberUse {
	var out []NumberUse
	for i, o := range b.Operations {
		if o.Op != OpSetAttr || o.Name != WhatsAppNumberAttr || o.Value == nil {
			continue
		}
		if ref := strings.TrimSpace(*o.Value); ref != "" {
			origin, field := b.Origin(i), "value"
			if origin.Op == OpSetWhatsAppLink {
				field = "number_ref"
			}
			out = append(out, NumberUse{Index: origin.Index, Ref: ref, Field: field})
		}
	}
	return out
}

func (o Operation) literalFiles() []string {
	var nodes []string
	switch o.Op.Family() {
	case FamilyVisual:
		nodes = append(nodes, o.Node, o.Before, o.After, o.Into)
		if o.At != nil {
			nodes = append(nodes, o.At.Before, o.At.After, o.At.Into)
		}
	default:
		return nil
	}
	var files []string
	for _, node := range nodes {
		if file, ok := literalFile(node); ok {
			files = append(files, file)
		}
	}
	if o.At != nil && o.At.Page != "" {
		files = append(files, o.At.Page)
	}
	return files
}

func literalFile(node string) (string, bool) {
	if node == "" || strings.HasPrefix(node, refPrefix) {
		return "", false
	}
	file, _, err := sitehtml.ParseNodeID(node)
	return file, err == nil
}

type staleRevs map[string]map[uint64]bool

func (s staleRevs) add(p Project, file string, rev uint64) {
	current, ok := p.Files[file]
	if rev == 0 || !ok || rev >= current.Rev {
		return
	}
	if s[file] == nil {
		s[file] = map[uint64]bool{}
	}
	s[file][rev] = true
}

func NodeStaleRevs(p Project, node string, rev uint64) map[string][]uint64 {
	wanted := staleRevs{}
	if file, ok := literalFile(node); ok {
		wanted.add(p, file, rev)
	}
	return wanted.sorted()
}

func (b Batch) StaleRevs(p Project) map[string][]uint64 {
	wanted := staleRevs{}
	for _, o := range b.Operations {
		for _, file := range o.literalFiles() {
			wanted.add(p, file, o.Rev)
		}
		if file, ok := literalFile(o.Source); ok {
			wanted.add(p, file, o.SourceRev)
		}
	}
	return wanted.sorted()
}

func (wanted staleRevs) sorted() map[string][]uint64 {
	out := make(map[string][]uint64, len(wanted))
	for file, revs := range wanted {
		out[file] = slices.Sorted(maps.Keys(revs))
	}
	return out
}
