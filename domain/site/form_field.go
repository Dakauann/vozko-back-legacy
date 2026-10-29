package site

import (
	"encoding/json"
	"html"
	"slices"
	"strings"
	"unicode/utf8"

	"vozko/domain/siteform"
)

const (
	CodeFormLabelTooLong = "form_label_too_long"

	MaxFormLabelRunes = 120

	formFieldRowClass      = "grid gap-1.5 text-sm"
	formFieldCheckRowClass = "flex items-start gap-2 text-sm"
	formFieldInputClass    = "w-full rounded-md border border-current/20 bg-transparent px-3 py-2"
)

var settableInputs = []siteform.Kind{siteform.KindText, siteform.KindEmail, siteform.KindTel, siteform.KindNumber, siteform.KindDate, siteform.KindURL, siteform.KindCheckbox}

var insertableInputs = append(slices.Clone(settableInputs), siteform.KindTextarea)

func SettableFormInputs() []siteform.Kind {
	return slices.Clone(settableInputs)
}

func InsertableFormInputs() []siteform.Kind {
	return slices.Clone(insertableInputs)
}

func (d *decoder) optionalFlag(name string) *bool {
	raw, ok := d.raw(name)
	if !ok {
		return nil
	}
	var value bool
	if json.Unmarshal(raw, &value) != nil {
		d.fail(CodeFieldInvalid, name)
		return nil
	}
	return &value
}

func formTarget(i int, o Operation) *Failure {
	if o.Field == "" {
		return failure(i, CodeFieldRequired, "field")
	}
	if len(siteform.TargetKinds(o.Field)) == 0 {
		return failure(i, CodeFormFieldUnknown, "field")
	}
	if o.Required != nil && !*o.Required && o.Field == siteform.TargetConsent {
		return failure(i, CodeFormFieldTypeInvalid, "required")
	}
	return nil
}

func formInput(i int, o Operation, allowed []siteform.Kind) *Failure {
	kind := siteform.Kind(o.Input)
	if !slices.Contains(allowed, kind) || !slices.Contains(siteform.TargetKinds(o.Field), kind) {
		return failure(i, CodeFormFieldTypeInvalid, "input")
	}
	return nil
}

func expandSetFormField(i int, o Operation) ([]Operation, *Failure) {
	if f := formTarget(i, o); f != nil {
		return nil, f
	}
	name := o.Field
	ops := []Operation{attrOp(o, "name", &name)}
	if o.Input != "" {
		if f := formInput(i, o, settableInputs); f != nil {
			return nil, f
		}
		input := o.Input
		ops = append(ops, attrOp(o, "type", &input))
	}
	if o.Required != nil {
		var present *string
		if *o.Required {
			empty := ""
			present = &empty
		}
		ops = append(ops, attrOp(o, "required", present))
	}
	return ops, nil
}

func expandInsertFormField(i int, o Operation) ([]Operation, *Failure) {
	if f := formTarget(i, o); f != nil {
		return nil, f
	}
	if o.Input == "" {
		return nil, failure(i, CodeFieldRequired, "input")
	}
	if f := formInput(i, o, insertableInputs); f != nil {
		return nil, f
	}
	label := ""
	if o.Text != nil {
		label = strings.Join(strings.Fields(*o.Text), " ")
	}
	if label == "" {
		return nil, failure(i, CodeFieldRequired, "text")
	}
	if utf8.RuneCountInString(label) > MaxFormLabelRunes {
		return nil, failure(i, CodeFormLabelTooLong, "text")
	}
	required := o.Field == siteform.TargetConsent || (o.Required != nil && *o.Required)
	return []Operation{{Op: OpInsertSection, At: o.At, Rev: o.Rev, Ref: o.Ref, HTML: formFieldMarkup(o.Field, siteform.Kind(o.Input), label, required)}}, nil
}

func formFieldMarkup(name string, kind siteform.Kind, label string, required bool) string {
	attrs := ` name="` + html.EscapeString(name) + `"`
	if required {
		attrs += " required"
	}
	text := "<span>" + html.EscapeString(label) + "</span>"
	switch kind {
	case siteform.KindCheckbox:
		return `<label class="` + formFieldCheckRowClass + `"><input type="checkbox"` + attrs + ">" + text + "</label>"
	case siteform.KindTextarea:
		return `<label class="` + formFieldRowClass + `">` + text + `<textarea class="` + formFieldInputClass + `"` + attrs + ` rows="3"></textarea></label>`
	}
	return `<label class="` + formFieldRowClass + `">` + text + `<input class="` + formFieldInputClass + `" type="` + string(kind) + `"` + attrs + "></label>"
}
