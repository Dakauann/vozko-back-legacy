package siteform

import (
	"slices"
	"strings"

	"vozko/domain/customfield"
	"vozko/domain/sitehtml"
)

var customKinds = []Kind{KindText, KindTextarea, KindNumber, KindDate, KindEmail, KindTel, KindURL, KindSelect, KindRadio, KindCheckbox, KindHidden}

var leadTargetKinds = map[string][]Kind{
	TargetName:       {KindText},
	TargetPhone:      {KindTel, KindText},
	TargetEmail:      {KindEmail},
	TargetConsent:    {KindCheckbox},
	TargetEmailOptIn: {KindCheckbox},
}

var controlTags = []string{"input", "select", "textarea", "button"}

func LeadTargets() []string {
	return []string{TargetName, TargetPhone, TargetEmail, TargetConsent, TargetEmailOptIn}
}

func TargetKinds(target string) []Kind {
	if kinds, ok := leadTargetKinds[target]; ok {
		return slices.Clone(kinds)
	}
	if key, ok := strings.CutPrefix(target, FieldPrefix); ok && ValidFieldKey(key) {
		return slices.Clone(customKinds)
	}
	return nil
}

type Control struct {
	Name        string
	Kind        Kind
	Required    bool
	Label       string
	LabelTextAt int
	RowAt       int
}

func controlKind(doc *sitehtml.Document, i int) (Kind, bool) {
	el := doc.Elements[i]
	switch el.Tag {
	case "select", "textarea":
		return Kind(el.Tag), true
	case "input":
		value, _ := doc.AttrValue(i, "type")
		value = strings.ToLower(strings.TrimSpace(value))
		if buttonTypes[value] {
			return "", false
		}
		if value == "" {
			return KindText, true
		}
		return Kind(value), true
	}
	return "", false
}

func labelOf(doc *sitehtml.Document, i int) int {
	if around, ok := doc.Ancestor(i, "label"); ok {
		return around
	}
	id, ok := doc.AttrValue(i, "id")
	if !ok || id == "" {
		return -1
	}
	for _, label := range doc.ByTag("label") {
		if target, ok := doc.AttrValue(label, "for"); ok && target == id {
			return label
		}
	}
	return -1
}

func holdsControl(doc *sitehtml.Document, i int) bool {
	return len(doc.Descendants(i, controlTags...)) > 0
}

func labelTextOf(doc *sitehtml.Document, label int) int {
	if label < 0 {
		return -1
	}
	if !holdsControl(doc, label) {
		return label
	}
	for _, child := range doc.Elements[label].Children {
		if slices.Contains(controlTags, doc.Elements[child].Tag) || holdsControl(doc, child) {
			continue
		}
		if doc.Text(child) != "" {
			return child
		}
	}
	return -1
}

func rowOf(doc *sitehtml.Document, i int) int {
	row := i
	for parent := doc.Elements[row].Parent; parent >= 0 && !IsFormRoot(doc, parent) && len(doc.Descendants(parent, controlTags...)) == 1; parent = doc.Elements[row].Parent {
		row = parent
	}
	return row
}

func ReadControl(doc *sitehtml.Document, i int) (Control, bool) {
	kind, ok := controlKind(doc, i)
	if !ok {
		return Control{}, false
	}
	name, _ := doc.AttrValue(i, "name")
	if name == TrapField {
		return Control{}, false
	}
	label := labelOf(doc, i)
	c := Control{Name: name, Kind: kind, Required: doc.HasAttr(i, "required") || name == TargetConsent, LabelTextAt: labelTextOf(doc, label), RowAt: rowOf(doc, i)}
	if label >= 0 {
		c.Label = doc.Text(label)
	}
	return c, true
}

func FormID(doc *sitehtml.Document, i int) string {
	if !IsFormRoot(doc, i) {
		return ""
	}
	attr := "id"
	if doc.Elements[i].Tag == "form" {
		attr = FormMarkerAttr
	}
	id, _ := doc.AttrValue(i, attr)
	return strings.TrimSpace(id)
}

func OwningForm(doc *sitehtml.Document, i int) int {
	for parent := doc.Elements[i].Parent; parent >= 0; parent = doc.Elements[parent].Parent {
		if IsFormRoot(doc, parent) {
			return parent
		}
	}
	return -1
}

var customFieldKinds = map[customfield.FieldType][]Kind{
	customfield.TypeText:        {KindText, KindTextarea, KindEmail, KindTel, KindURL, KindSelect, KindRadio, KindHidden},
	customfield.TypeNumber:      {KindNumber, KindText},
	customfield.TypeDate:        {KindDate, KindText},
	customfield.TypeBoolean:     {KindCheckbox},
	customfield.TypeSelect:      {KindSelect, KindRadio},
	customfield.TypeMultiSelect: {KindCheckbox, KindSelect},
}

func CustomFieldKinds(t customfield.FieldType) []Kind {
	return slices.Clone(customFieldKinds[t])
}

type CustomTarget struct {
	Target string
	Label  string
	Kinds  []Kind
}

func CustomTargets(defs []*customfield.Definition) []CustomTarget {
	out := []CustomTarget{}
	for _, d := range defs {
		if d == nil || d.Sensitive || d.ObjectType != customfield.ObjectLead || !ValidFieldKey(d.Key) {
			continue
		}
		kinds := CustomFieldKinds(d.Type)
		if len(kinds) == 0 {
			continue
		}
		out = append(out, CustomTarget{Target: FieldPrefix + d.Key, Label: d.Label, Kinds: kinds})
	}
	return out
}
