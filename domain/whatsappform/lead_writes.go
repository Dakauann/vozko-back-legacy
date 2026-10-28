package whatsappform

import (
	"strconv"
	"strings"

	"vozko/domain/customfield"
	"vozko/domain/lead"
)

func (a Answer) storedValue(f Field, joiner string) string {
	switch {
	case f.Kind.IsMultiChoice():
		titles := make([]string, 0, len(a.OptionIDs))
		for _, id := range a.OptionIDs {
			if o, ok := f.Option(id); ok {
				titles = append(titles, o.Title)
			}
		}
		return strings.Join(titles, joiner)
	case a.Checked != nil:
		return strconv.FormatBool(*a.Checked)
	case f.Kind == FieldDate:
		return a.Text
	}
	return a.Display(f)
}

func ToProfile(doc Document, answers Answers, defs []*customfield.Definition) lead.Profile {
	types := map[string]customfield.FieldType{}
	for _, d := range defs {
		if d != nil {
			types[d.Key] = d.Type
		}
	}
	var profile lead.Profile
	for _, ref := range doc.Fields() {
		f := ref.Field
		answer, ok := answers.Values[f.Key]
		if !ok || f.Binding == nil || f.IsSensitive() {
			continue
		}
		switch f.Binding.Target {
		case BindName:
			profile.Name = answer.Text
		case BindEmail:
			profile.Email = answer.Text
		case BindBirthDate:
			profile.BirthDate = answer.Text
		case BindAddressZip:
			profile.Address.ZipCode = answer.Display(f)
		case BindAddressCity:
			profile.Address.City = answer.Display(f)
		case BindAddressState:
			profile.Address.State = answer.Display(f)
		case BindCustomField:
			if profile.CustomFields == nil {
				profile.CustomFields = map[string]string{}
			}
			profile.CustomFields[f.Binding.CustomFieldKey] = answer.storedValue(f, joinerFor(types[f.Binding.CustomFieldKey]))
		}
	}
	return profile
}

func joinerFor(t customfield.FieldType) string {
	if t == customfield.TypeMultiSelect {
		return customfield.MultiSelectDelimiter
	}
	return listJoiner
}

func ConsentGrantText(doc Document, answers Answers) (string, bool) {
	for _, ref := range doc.Fields() {
		f := ref.Field
		answer, ok := answers.Values[f.Key]
		if !ok || f.Binding == nil || f.Binding.Target != BindWhatsAppConsent || answer.Checked == nil || !*answer.Checked {
			continue
		}
		return f.Label, true
	}
	return "", false
}
