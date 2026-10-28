package campaignaudience

import (
	"maps"

	"vozko/domain/automationneeds"
	"vozko/domain/campaign"
)

type NeedSuggest struct {
	Field  func(name string) campaign.BindingSource
	Column func(name string) string
}

func (s NeedSuggest) field(name string) campaign.BindingSource {
	if s.Field == nil {
		return ""
	}
	return s.Field(name)
}

func (s NeedSuggest) column(name string) string {
	if s.Column == nil {
		return ""
	}
	return s.Column(name)
}

func AutomationSlots(needs, optional automationneeds.Needs, template []Slot, suggest NeedSuggest) []Slot {
	taken := make(map[string]bool, len(template))
	for _, s := range template {
		taken[s.Key] = true
	}
	var out []Slot
	for _, need := range needs {
		if taken[need.Name] {
			continue
		}
		taken[need.Name] = true
		out = append(out, Slot{Key: need.Name, Name: need.Display(), Kind: SlotAutomation, NeededBy: need.Owner().Name, Suggested: suggest.field(need.Name), Column: suggest.column(need.Name)})
	}
	for _, need := range optional {
		if taken[need.Name] {
			continue
		}
		taken[need.Name] = true
		out = append(out, Slot{Key: need.Name, Name: need.Display(), Kind: SlotAutomation, NeededBy: need.Owner().Name, Optional: true, Default: need.Default, Column: suggest.column(need.Name)})
	}
	return out
}

func KnownVariables(vars []Variable, slots []Slot, needs automationneeds.Needs) []Variable {
	known := make(map[string]bool, len(slots))
	for _, s := range slots {
		known[s.Key] = true
	}
	out := make([]Variable, 0, len(vars))
	for _, v := range vars {
		if !known[v.Key] && needs.Find(v.Key) != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

func MarkColumns(bound []Bound, layout campaign.ListLayout) []Bound {
	out := make([]Bound, len(bound))
	for i, b := range bound {
		b.ColumnMissing = b.Column != "" && !layout.HasColumn(b.Column)
		out[i] = b
	}
	return out
}

func ListTargets(bound []Bound) []campaign.ListTarget {
	out := make([]campaign.ListTarget, 0, len(bound))
	for _, b := range bound {
		if b.Automation() {
			out = append(out, campaign.ListTarget{Key: b.Key, Column: b.Column})
			continue
		}
		out = append(out, campaign.ListTarget{Field: b.Source})
	}
	return out
}

func (t *Tally) Split(values []string) ([]string, map[string]any) {
	return t.EntryMetadata(nil, values)
}

func (t *Tally) EntryMetadata(base map[string]any, values []string) ([]string, map[string]any) {
	split := SplitRow(t.bound, base, values)
	return split.Template, split.Metadata
}

type RowSplit struct {
	Template []string
	Metadata map[string]any
	Unmet    []string
}

func SplitRow(bound []Bound, base map[string]any, values []string) RowSplit {
	split := RowSplit{Template: make([]string, 0, len(values)), Metadata: maps.Clone(base)}
	for i, b := range bound {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		if !b.Automation() {
			split.Template = append(split.Template, value)
			continue
		}
		delete(split.Metadata, b.Key)
		if value == "" {
			if !b.Optional {
				split.Unmet = append(split.Unmet, b.Key)
			}
			continue
		}
		if split.Metadata == nil {
			split.Metadata = map[string]any{}
		}
		split.Metadata[b.Key] = value
	}
	if len(split.Metadata) == 0 {
		split.Metadata = nil
	}
	return split
}

func (t *Tally) templateSlots() int {
	n := 0
	for _, b := range t.bound {
		if !b.Automation() {
			n++
		}
	}
	return n
}
