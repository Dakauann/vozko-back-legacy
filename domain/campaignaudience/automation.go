package campaignaudience

import (
	"vozko/domain/automationneeds"
	"vozko/domain/campaign"
)

func AutomationSlots(needs automationneeds.Needs, template []Slot, suggest func(name string) campaign.BindingSource) []Slot {
	taken := make(map[string]bool, len(template))
	for _, s := range template {
		taken[s.Key] = true
	}
	var out []Slot
	for _, need := range needs {
		if taken[need.Name] {
			continue
		}
		slot := Slot{Key: need.Name, Name: need.Name, Kind: SlotAutomation, NeededBy: need.Owner().Name}
		if suggest != nil {
			slot.Suggested = suggest(need.Name)
		}
		out = append(out, slot)
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

func WithoutTemplateKeys(rule MetadataRule, template []Slot) MetadataRule {
	if rule == nil {
		return nil
	}
	taken := make(map[string]bool, len(template))
	for _, s := range template {
		taken[s.Key] = true
	}
	return func(metadata map[string]any) []string {
		var out []string
		for _, key := range rule(metadata) {
			if !taken[key] {
				out = append(out, key)
			}
		}
		return out
	}
}

func (t *Tally) Split(values []string) ([]string, map[string]any) {
	template := make([]string, 0, len(values))
	var metadata map[string]any
	for i, b := range t.bound {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		if !b.Automation() {
			template = append(template, value)
			continue
		}
		if value == "" {
			continue
		}
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata[b.Key] = value
	}
	return template, metadata
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
