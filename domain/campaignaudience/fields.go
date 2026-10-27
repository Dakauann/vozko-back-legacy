package campaignaudience

import (
	"vozko/domain/campaign"
	"vozko/domain/customfield"
)

type FieldKind string

const (
	FieldLead   FieldKind = "lead"
	FieldCustom FieldKind = "custom"
)

type Field struct {
	Source campaign.BindingSource `json:"source"`
	Kind   FieldKind              `json:"kind" enums:"lead,custom"`
	Column string                 `json:"column"`
	Key    string                 `json:"key,omitempty"`
	Label  string                 `json:"label,omitempty"`
	Type   customfield.FieldType  `json:"type,omitempty"`
}

func FieldsOf(defs []*customfield.Definition) []Field {
	fields := make([]Field, 0, len(campaign.BindableSources())+len(defs))
	for _, source := range campaign.BindableSources() {
		fields = append(fields, Field{Source: source, Kind: FieldLead, Column: campaign.FieldColumn(source, nil)})
	}
	for _, def := range campaign.BindableCustomFields(defs) {
		source := campaign.CustomBinding(def.Key)
		fields = append(fields, Field{Source: source, Kind: FieldCustom, Column: campaign.FieldColumn(source, defs), Key: def.Key, Label: def.Label, Type: def.Type})
	}
	return fields
}

func FieldPlan(fields []Field, defs []*customfield.Definition) (campaign.BindingPlan, error) {
	bindings := make([]campaign.VariableBinding, 0, len(fields))
	for _, f := range fields {
		bindings = append(bindings, campaign.VariableBinding{Source: f.Source})
	}
	return campaign.PlanBindings(bindings, len(bindings), defs)
}
