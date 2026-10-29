package site

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"vozko/domain/inlinemarkup"
	"vozko/domain/strictjson"
)

type OperationKind string

const (
	OpWriteFile     OperationKind = "write_file"
	OpEditFile      OperationKind = "edit_file"
	OpDeleteFile    OperationKind = "delete_file"
	OpRenameFile    OperationKind = "rename_file"
	OpSetText       OperationKind = "set_text"
	OpSetAttr       OperationKind = "set_attr"
	OpSetClasses    OperationKind = "set_classes"
	OpReplaceImage  OperationKind = "replace_image"
	OpMove          OperationKind = "move"
	OpDuplicate     OperationKind = "duplicate"
	OpRemove        OperationKind = "remove"
	OpInsertSection OperationKind = "insert_section"
	OpSetToken      OperationKind = "set_token"
	OpSetKnob       OperationKind = "set_knob"

	OpDefineKnob        OperationKind = "define_knob"
	OpRenameKnob        OperationKind = "rename_knob"
	OpRemoveKnob        OperationKind = "remove_knob"
	OpSetWhatsAppLink   OperationKind = "set_whatsapp_link"
	OpClearWhatsAppLink OperationKind = "clear_whatsapp_link"
	OpInsertElement     OperationKind = "insert_element"
	OpSetFormField      OperationKind = "set_form_field"
	OpInsertFormField   OperationKind = "insert_form_field"
)

type Family string

const (
	FamilyCode   Family = "code"
	FamilyVisual Family = "visual"
	FamilyTheme  Family = "theme"

	FamilyShorthand Family = "shorthand"
)

func OperationKinds() []OperationKind {
	return []OperationKind{
		OpWriteFile, OpEditFile, OpDeleteFile, OpRenameFile,
		OpSetText, OpSetAttr, OpSetClasses, OpReplaceImage, OpMove, OpDuplicate, OpRemove, OpInsertSection,
		OpSetToken, OpSetKnob,
	}
}

func ShorthandKinds() []OperationKind {
	return []OperationKind{OpDefineKnob, OpRenameKnob, OpRemoveKnob, OpSetWhatsAppLink, OpClearWhatsAppLink, OpInsertElement, OpSetFormField, OpInsertFormField}
}

func (k OperationKind) Family() Family {
	switch k {
	case OpDefineKnob, OpRenameKnob, OpRemoveKnob, OpSetWhatsAppLink, OpClearWhatsAppLink, OpInsertElement, OpSetFormField, OpInsertFormField:
		return FamilyShorthand
	case OpWriteFile, OpEditFile, OpDeleteFile, OpRenameFile:
		return FamilyCode
	case OpSetToken, OpSetKnob:
		return FamilyTheme
	case OpSetText, OpSetAttr, OpSetClasses, OpReplaceImage, OpMove, OpDuplicate, OpRemove, OpInsertSection:
		return FamilyVisual
	}
	return ""
}

func (k OperationKind) Known() bool {
	return k.Family() != ""
}

type Placement struct {
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Into   string `json:"into,omitempty"`
	Page   string `json:"page,omitempty"`
}

type Operation struct {
	Op         OperationKind
	Path       string
	To         string
	Content    *string
	Old        string
	New        *string
	ReplaceAll bool
	Node       string
	Rev        uint64
	Item       *int
	Text       *string
	Name       string
	Value      *string
	Number     *float64
	Add        []string
	Remove     []string
	Breakpoint string
	MediaID    string
	Alt        *string
	Before     string
	After      string
	Into       string
	At         *Placement
	HTML       string
	LibraryKey string
	Ref        string
	Variable   string
	Kind       KnobKind
	Min        *float64
	Max        *float64
	Step       *float64
	Unit       string
	NumberRef  string
	Element    PlatformElement
	Key        string
	Source     string
	SourceRev  uint64
	Field      string
	Input      string
	Required   *bool
}

type operationWire struct {
	Op         OperationKind   `json:"op"`
	Path       string          `json:"path,omitempty"`
	To         string          `json:"to,omitempty"`
	Content    *string         `json:"content,omitempty"`
	Old        string          `json:"old,omitempty"`
	New        *string         `json:"new,omitempty"`
	ReplaceAll bool            `json:"replace_all,omitempty"`
	Node       string          `json:"node,omitempty"`
	Rev        uint64          `json:"rev,omitempty"`
	Item       *int            `json:"item,omitempty"`
	Text       *string         `json:"text,omitempty"`
	Name       string          `json:"name,omitempty"`
	Value      any             `json:"value,omitempty"`
	Add        []string        `json:"add,omitempty"`
	Remove     []string        `json:"remove,omitempty"`
	Breakpoint string          `json:"breakpoint,omitempty"`
	MediaID    string          `json:"media_id,omitempty"`
	Alt        *string         `json:"alt,omitempty"`
	Before     string          `json:"before,omitempty"`
	After      string          `json:"after,omitempty"`
	Into       string          `json:"into,omitempty"`
	At         *Placement      `json:"at,omitempty"`
	HTML       string          `json:"html,omitempty"`
	LibraryKey string          `json:"library_key,omitempty"`
	Ref        string          `json:"ref,omitempty"`
	Variable   string          `json:"variable,omitempty"`
	Kind       KnobKind        `json:"kind,omitempty"`
	Min        *float64        `json:"min,omitempty"`
	Max        *float64        `json:"max,omitempty"`
	Step       *float64        `json:"step,omitempty"`
	Unit       string          `json:"unit,omitempty"`
	NumberRef  string          `json:"number_ref,omitempty"`
	Element    PlatformElement `json:"element,omitempty"`
	Key        string          `json:"key,omitempty"`
	Source     string          `json:"source,omitempty"`
	SourceRev  uint64          `json:"source_rev,omitempty"`
	Field      string          `json:"field,omitempty"`
	Input      string          `json:"input,omitempty"`
	Required   *bool           `json:"required,omitempty"`
}

func (o Operation) MarshalJSON() ([]byte, error) {
	w := operationWire{
		Op: o.Op, Path: o.Path, To: o.To, Content: o.Content, Old: o.Old, New: o.New, ReplaceAll: o.ReplaceAll,
		Node: o.Node, Rev: o.Rev, Item: o.Item, Text: o.Text, Name: o.Name, Add: o.Add, Remove: o.Remove,
		Breakpoint: o.Breakpoint, MediaID: o.MediaID, Alt: o.Alt, Before: o.Before, After: o.After, Into: o.Into, At: o.At,
		HTML: o.HTML, LibraryKey: o.LibraryKey, Ref: o.Ref, Variable: o.Variable, Kind: o.Kind, Min: o.Min, Max: o.Max, Step: o.Step,
		Unit: o.Unit, NumberRef: o.NumberRef, Element: o.Element, Key: o.Key, Source: o.Source, SourceRev: o.SourceRev,
		Field: o.Field, Input: o.Input, Required: o.Required,
	}
	switch {
	case o.Number != nil:
		w.Value = *o.Number
	case o.Value != nil:
		w.Value = *o.Value
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(w); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(out.Bytes()), nil
}

var operationFields = map[OperationKind][]string{
	OpWriteFile:     {"path", "rev", "content"},
	OpEditFile:      {"path", "old", "new", "replace_all"},
	OpDeleteFile:    {"path", "rev"},
	OpRenameFile:    {"path", "rev", "to"},
	OpSetText:       {"node", "rev", "text", "html", "item"},
	OpSetAttr:       {"node", "rev", "name", "value"},
	OpSetClasses:    {"node", "rev", "add", "remove", "breakpoint"},
	OpReplaceImage:  {"node", "rev", "media_id", "alt"},
	OpMove:          {"node", "rev", "before", "after", "into"},
	OpDuplicate:     {"node", "rev", "ref"},
	OpRemove:        {"node", "rev"},
	OpInsertSection: {"at", "rev", "html", "library_key", "ref", "source", "source_rev"},
	OpSetToken:      {"name", "value"},
	OpSetKnob:       {"name", "value"},

	OpDefineKnob:        {"name", "variable", "kind", "min", "max", "step", "unit"},
	OpRenameKnob:        {"name", "to"},
	OpRemoveKnob:        {"name"},
	OpSetWhatsAppLink:   {"node", "rev", "number_ref", "text"},
	OpClearWhatsAppLink: {"node", "rev"},
	OpInsertElement:     {"at", "rev", "element", "key", "ref"},
	OpSetFormField:      {"node", "rev", "field", "input", "required"},
	OpInsertFormField:   {"at", "rev", "field", "input", "text", "required", "ref"},
}

type Ignored struct {
	Index int    `json:"index"`
	Field string `json:"field"`
}

type decoder struct {
	fields map[string]json.RawMessage
	failed *Failure
	index  int
}

func (d *decoder) fail(code, field string) {
	if d.failed == nil {
		d.failed = &Failure{Index: d.index, Code: code, Field: field}
	}
}

func (d *decoder) raw(name string) (json.RawMessage, bool) {
	raw, ok := d.fields[name]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, false
	}
	return raw, true
}

func (d *decoder) identifier(name string) string {
	raw, ok := d.raw(name)
	if !ok {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		d.fail(CodeFieldInvalid, name)
		return ""
	}
	return strings.TrimSpace(value)
}

func (d *decoder) text(name string) *string {
	raw, ok := d.raw(name)
	if !ok {
		return nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		d.fail(CodeFieldInvalid, name)
		return nil
	}
	return &value
}

func (d *decoder) flag(name string) bool {
	raw, ok := d.raw(name)
	if !ok {
		return false
	}
	var value bool
	if json.Unmarshal(raw, &value) != nil {
		d.fail(CodeFieldInvalid, name)
	}
	return value
}

func (d *decoder) rev(name string) uint64 {
	raw, ok := d.raw(name)
	if !ok {
		return 0
	}
	var value uint64
	if json.Unmarshal(raw, &value) != nil {
		d.fail(CodeFieldInvalid, name)
	} else if value == 0 {
		d.fail(CodeRevInvalid, name)
	}
	return value
}

func (d *decoder) number(name string) *float64 {
	raw, ok := d.raw(name)
	if !ok {
		return nil
	}
	var value float64
	if json.Unmarshal(raw, &value) != nil {
		d.fail(CodeFieldInvalid, name)
		return nil
	}
	return &value
}

func (d *decoder) textFromHTML(o *Operation, html string) {
	if o.Text != nil {
		d.fail(CodeFieldInvalid, "html")
		return
	}
	paragraphs, err := inlinemarkup.FromHTML(html)
	if err != nil || len(paragraphs) > 1 {
		d.fail(CodeTextMarkupInvalid, "html")
		return
	}
	markup := inlinemarkup.Format(paragraphs)
	o.Text = &markup
}

func (d *decoder) item() *int {
	raw, ok := d.raw("item")
	if !ok {
		return nil
	}
	var value int
	if json.Unmarshal(raw, &value) != nil || value < 0 {
		d.fail(CodeFieldInvalid, "item")
		return nil
	}
	return &value
}

func (d *decoder) list(name string) []string {
	raw, ok := d.raw(name)
	if !ok {
		return nil
	}
	var values []string
	if json.Unmarshal(raw, &values) != nil {
		d.fail(CodeFieldInvalid, name)
		return nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (d *decoder) placement() *Placement {
	raw, ok := d.raw("at")
	if !ok {
		return nil
	}
	var p Placement
	if strictjson.Decode(raw, &p) != nil {
		d.fail(CodeFieldInvalid, "at")
		return nil
	}
	p = Placement{Before: strings.TrimSpace(p.Before), After: strings.TrimSpace(p.After), Into: strings.TrimSpace(p.Into), Page: strings.TrimSpace(p.Page)}
	return &p
}

func (d *decoder) value(op OperationKind, o *Operation) {
	raw, ok := d.raw("value")
	if !ok {
		return
	}
	trimmed := bytes.TrimSpace(raw)
	switch {
	case len(trimmed) > 0 && trimmed[0] == '"':
		var text string
		if json.Unmarshal(trimmed, &text) != nil {
			d.fail(CodeFieldInvalid, "value")
			return
		}
		o.Value = &text
	case op == OpSetKnob && len(trimmed) > 0 && (trimmed[0] == '-' || (trimmed[0] >= '0' && trimmed[0] <= '9')):
		var number float64
		if json.Unmarshal(trimmed, &number) != nil {
			d.fail(CodeFieldInvalid, "value")
			return
		}
		o.Number = &number
	default:
		d.fail(CodeFieldInvalid, "value")
	}
}

func decodeOperation(index int, raw json.RawMessage) (Operation, []Ignored, *Failure) {
	d := &decoder{index: index}
	if json.Unmarshal(raw, &d.fields) != nil || d.fields == nil {
		return Operation{}, nil, &Failure{Index: index, Code: CodeOperationInvalid}
	}
	kindRaw, ok := d.raw("op")
	if !ok {
		return Operation{}, nil, &Failure{Index: index, Code: CodeFieldRequired, Field: "op"}
	}
	var kind OperationKind
	if json.Unmarshal(kindRaw, &kind) != nil || !kind.Known() {
		return Operation{}, nil, &Failure{Index: index, Code: CodeOperationUnknown, Field: "op"}
	}
	allowed := map[string]bool{"op": true}
	for _, name := range operationFields[kind] {
		allowed[name] = true
	}
	var ignored []Ignored
	for _, name := range slices.Sorted(maps.Keys(d.fields)) {
		if !allowed[name] {
			ignored = append(ignored, Ignored{Index: index, Field: name})
			delete(d.fields, name)
		}
	}
	o := Operation{
		Op:         kind,
		Path:       d.identifier("path"),
		To:         d.identifier("to"),
		Content:    d.text("content"),
		New:        d.text("new"),
		ReplaceAll: d.flag("replace_all"),
		Node:       d.identifier("node"),
		Rev:        d.rev("rev"),
		Item:       d.item(),
		Text:       d.text("text"),
		Name:       d.identifier("name"),
		Add:        d.list("add"),
		Remove:     d.list("remove"),
		Breakpoint: d.identifier("breakpoint"),
		MediaID:    d.identifier("media_id"),
		Alt:        d.text("alt"),
		Before:     d.identifier("before"),
		After:      d.identifier("after"),
		Into:       d.identifier("into"),
		At:         d.placement(),
		LibraryKey: d.identifier("library_key"),
		Ref:        d.identifier("ref"),
		Variable:   d.identifier("variable"),
		Kind:       KnobKind(d.identifier("kind")),
		Min:        d.number("min"),
		Max:        d.number("max"),
		Step:       d.number("step"),
		Unit:       d.identifier("unit"),
		NumberRef:  d.identifier("number_ref"),
		Element:    PlatformElement(d.identifier("element")),
		Key:        d.identifier("key"),
		Source:     d.identifier("source"),
		SourceRev:  d.rev("source_rev"),
		Field:      d.identifier("field"),
		Input:      d.identifier("input"),
		Required:   d.optionalFlag("required"),
	}
	if old := d.text("old"); old != nil {
		o.Old = *old
	}
	if html := d.text("html"); html != nil {
		if kind == OpSetText {
			d.textFromHTML(&o, *html)
		} else {
			o.HTML = *html
		}
	}
	d.value(kind, &o)
	if d.failed != nil {
		return o, ignored, d.failed
	}
	return o, ignored, nil
}
