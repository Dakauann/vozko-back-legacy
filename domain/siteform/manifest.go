package siteform

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"vozko/domain/sitehtml"
	"vozko/domain/strictjson"
)

const (
	TargetName       = "lead.name"
	TargetPhone      = "lead.phone"
	TargetEmail      = "lead.email"
	TargetConsent    = "consent"
	TargetEmailOptIn = "email_opt_in"
	FieldPrefix      = "field."

	FormMarkerAttr = "data-vz-form"
	FormElement    = "vz-form"

	maxFieldKeyLength = 64
	maxTextLength     = 500
	maxLongTextLength = 5000
)

const (
	CodeFormInvalid          = "form_invalid"
	CodeFormFieldUnknown     = "form_field_unknown"
	CodeFormFieldTypeInvalid = "form_field_type_invalid"
	CodeFormFieldDuplicate   = "form_field_duplicate"
	CodeFormIDDuplicate      = "form_id_duplicate"
)

var ErrManifestInvalid = errors.New("siteform: the form manifest is invalid")

var formID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type Kind string

const (
	KindText     Kind = "text"
	KindEmail    Kind = "email"
	KindTel      Kind = "tel"
	KindNumber   Kind = "number"
	KindDate     Kind = "date"
	KindURL      Kind = "url"
	KindHidden   Kind = "hidden"
	KindCheckbox Kind = "checkbox"
	KindRadio    Kind = "radio"
	KindSelect   Kind = "select"
	KindTextarea Kind = "textarea"
)

var knownKinds = map[Kind]bool{
	KindText: true, KindEmail: true, KindTel: true, KindNumber: true, KindDate: true, KindURL: true,
	KindHidden: true, KindCheckbox: true, KindRadio: true, KindSelect: true, KindTextarea: true,
}

var buttonTypes = map[string]bool{"submit": true, "button": true, "reset": true, "image": true}

type Field struct {
	Name      string   `json:"name"`
	Kind      Kind     `json:"kind"`
	Required  bool     `json:"required,omitempty"`
	Label     string   `json:"label,omitempty"`
	MaxLength int      `json:"max_length,omitempty"`
	Options   []string `json:"options,omitempty"`
	Value     string   `json:"value,omitempty"`
}

type Manifest struct {
	ID             string  `json:"id"`
	Node           string  `json:"node,omitempty"`
	Fields         []Field `json:"fields"`
	ConsentText    string  `json:"consent_text,omitempty"`
	EmailOptInText string  `json:"email_opt_in_text,omitempty"`
}

type Problem struct {
	Code  string `json:"code"`
	Form  string `json:"form,omitempty"`
	Field string `json:"field,omitempty"`
	Node  string `json:"node,omitempty"`
}

func IsFormRoot(doc *sitehtml.Document, i int) bool {
	el := doc.Elements[i]
	return el.Tag == FormElement || (el.Tag == "form" && doc.HasAttr(i, FormMarkerAttr))
}

func ValidTarget(name string) bool {
	switch name {
	case TargetName, TargetPhone, TargetEmail, TargetConsent, TargetEmailOptIn:
		return true
	}
	key, ok := strings.CutPrefix(name, FieldPrefix)
	return ok && ValidFieldKey(key)
}

func ValidFieldKey(key string) bool {
	return key != "" && len(key) <= maxFieldKeyLength && key == strings.ToLower(key) && !strings.ContainsAny(key, " \t\r\n\"'<>")
}

func kindFits(target string, kind Kind) bool {
	switch target {
	case TargetName:
		return kind == KindText
	case TargetPhone:
		return kind == KindTel || kind == KindText
	case TargetEmail:
		return kind == KindEmail
	case TargetConsent, TargetEmailOptIn:
		return kind == KindCheckbox
	}
	return knownKinds[kind]
}

type extractor struct {
	doc      *sitehtml.Document
	problems []Problem
}

func Extract(doc *sitehtml.Document) ([]Manifest, []Problem) {
	x := &extractor{doc: doc}
	var manifests []Manifest
	seen := map[string]bool{}
	for i := range doc.Elements {
		if !IsFormRoot(doc, i) {
			continue
		}
		m, ok := x.form(i)
		if !ok {
			continue
		}
		if seen[m.ID] {
			x.problems = append(x.problems, Problem{Code: CodeFormIDDuplicate, Form: m.ID, Node: m.Node})
			continue
		}
		seen[m.ID] = true
		manifests = append(manifests, m)
	}
	return manifests, x.problems
}

func (x *extractor) form(i int) (Manifest, bool) {
	el := x.doc.Elements[i]
	id, _ := x.doc.AttrValue(i, "id")
	if el.Tag == "form" {
		id, _ = x.doc.AttrValue(i, FormMarkerAttr)
	}
	if !formID.MatchString(id) {
		x.problems = append(x.problems, Problem{Code: CodeFormInvalid, Form: id, Node: el.Node})
		return Manifest{}, false
	}
	if _, nested := x.doc.Ancestor(i, FormElement, "form"); nested {
		x.problems = append(x.problems, Problem{Code: CodeFormInvalid, Form: id, Node: el.Node})
		return Manifest{}, false
	}
	m := Manifest{ID: id, Node: el.Node, Fields: []Field{}}
	index := map[string]int{}
	for _, child := range x.doc.Descendants(i, "input", "select", "textarea", FormElement, "form") {
		if IsFormRoot(x.doc, child) || x.doc.Elements[child].Tag == "form" {
			x.problems = append(x.problems, Problem{Code: CodeFormInvalid, Form: id, Node: x.doc.Elements[child].Node})
			return Manifest{}, false
		}
		field, ok := x.field(id, child)
		if !ok {
			continue
		}
		if at, repeated := index[field.Name]; repeated {
			if field.Kind == KindRadio && m.Fields[at].Kind == KindRadio {
				m.Fields[at].Options = append(m.Fields[at].Options, field.Options...)
				m.Fields[at].Required = m.Fields[at].Required || field.Required
				continue
			}
			x.problems = append(x.problems, Problem{Code: CodeFormFieldDuplicate, Form: id, Field: field.Name, Node: x.doc.Elements[child].Node})
			continue
		}
		index[field.Name] = len(m.Fields)
		m.Fields = append(m.Fields, field)
		switch field.Name {
		case TargetConsent:
			m.ConsentText = field.Label
		case TargetEmailOptIn:
			m.EmailOptInText = field.Label
		}
	}
	return m, true
}

func (x *extractor) field(form string, i int) (Field, bool) {
	el := x.doc.Elements[i]
	name, named := x.doc.AttrValue(i, "name")
	kind := Kind(el.Tag)
	if el.Tag == "input" {
		value, _ := x.doc.AttrValue(i, "type")
		value = strings.ToLower(strings.TrimSpace(value))
		if buttonTypes[value] {
			return Field{}, false
		}
		kind = KindText
		if value != "" {
			kind = Kind(value)
		}
	}
	if !named {
		return Field{}, false
	}
	problem := func(code string) (Field, bool) {
		x.problems = append(x.problems, Problem{Code: code, Form: form, Field: name, Node: el.Node})
		return Field{}, false
	}
	if !ValidTarget(name) {
		return problem(CodeFormFieldUnknown)
	}
	if !knownKinds[kind] || !kindFits(name, kind) {
		return problem(CodeFormFieldTypeInvalid)
	}
	field := Field{Name: name, Kind: kind, Required: x.doc.HasAttr(i, "required") || name == TargetConsent, Label: x.label(i)}
	if raw, ok := x.doc.AttrValue(i, "maxlength"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n <= 0 {
			return problem(CodeFormFieldTypeInvalid)
		}
		field.MaxLength = min(n, maxLongTextLength)
	}
	switch kind {
	case KindSelect:
		for _, option := range x.doc.Descendants(i, "option") {
			value, ok := x.doc.AttrValue(option, "value")
			if !ok {
				value = x.doc.Text(option)
			}
			field.Options = append(field.Options, value)
		}
	case KindRadio:
		value, ok := x.doc.AttrValue(i, "value")
		if !ok {
			value = "on"
		}
		field.Options = []string{value}
	case KindHidden, KindCheckbox:
		field.Value, _ = x.doc.AttrValue(i, "value")
	}
	return field, true
}

func (x *extractor) label(i int) string {
	if around, ok := x.doc.Ancestor(i, "label"); ok {
		return x.doc.Text(around)
	}
	id, ok := x.doc.AttrValue(i, "id")
	if !ok || id == "" {
		return ""
	}
	for _, label := range x.doc.ByTag("label") {
		if target, ok := x.doc.AttrValue(label, "for"); ok && target == id {
			return x.doc.Text(label)
		}
	}
	return ""
}

func (m Manifest) Field(name string) (Field, bool) {
	for _, f := range m.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

func (m Manifest) HasIdentity() bool {
	_, phone := m.Field(TargetPhone)
	_, email := m.Field(TargetEmail)
	return phone || email
}

func (m Manifest) fieldKeys() []string {
	var out []string
	for _, f := range m.Fields {
		if key, ok := strings.CutPrefix(f.Name, FieldPrefix); ok {
			out = append(out, key)
		}
	}
	return out
}

func DecodeManifest(raw []byte) (Manifest, error) {
	var m Manifest
	if err := strictjson.Decode(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: %w", ErrManifestInvalid, err)
	}
	if !formID.MatchString(m.ID) {
		return Manifest{}, fmt.Errorf("%w: the form id is invalid", ErrManifestInvalid)
	}
	seen := map[string]bool{}
	for _, f := range m.Fields {
		if !ValidTarget(f.Name) || !knownKinds[f.Kind] || !kindFits(f.Name, f.Kind) || seen[f.Name] {
			return Manifest{}, fmt.Errorf("%w: field %q is invalid", ErrManifestInvalid, f.Name)
		}
		seen[f.Name] = true
	}
	return m, nil
}
