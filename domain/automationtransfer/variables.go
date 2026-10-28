package automationtransfer

import (
	"errors"
	"slices"
	"strings"

	"vozko/domain/automationneeds"
	"vozko/domain/campaign"
	"vozko/domain/conversationvars"
	"vozko/domain/customfield"
)

const (
	keyVariables                            = "variables"
	SourceAI         campaign.BindingSource = "ai"
	variableName                            = "name"
	variableSource                          = "source"
	variableValue                           = "value"
	variableNote                            = "description"
	filledSeparator                         = ","
	filledAssignment                        = "="
)

var (
	ErrVariableNotNeeded = errors.New("automation transfer: a mapped variable is no longer needed by the destination")
	ErrVariableUnmapped  = errors.New("automation transfer: the destination now needs a variable the mapping does not list")
	ErrVariableSource    = errors.New("automation transfer: a variable source is not valid")
	ErrModelValueMissing = errors.New("automation transfer: the model did not inform a variable the destination needs")
)

var variableKeys = map[string]bool{variableName: true, variableSource: true, variableValue: true, variableNote: true}

type VariableSource struct {
	Name        string
	Source      campaign.BindingSource
	Value       string
	Description string
}

type Filled struct {
	Name   string
	Source campaign.BindingSource
}

func parseVariables(raw interface{}) ([]VariableSource, error) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, ErrConfigUnreadable
	}
	out := make([]VariableSource, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		fields, ok := item.(map[string]interface{})
		if !ok {
			return nil, ErrConfigUnreadable
		}
		values := make(map[string]string, len(fields))
		for key, value := range fields {
			text, isText := value.(string)
			if !variableKeys[key] || !isText {
				return nil, ErrConfigUnreadable
			}
			values[key] = strings.TrimSpace(text)
		}
		name, source := values[variableName], values[variableSource]
		if name == "" || source == "" || seen[name] {
			return nil, ErrConfigUnreadable
		}
		seen[name] = true
		out = append(out, VariableSource{Name: name, Source: campaign.BindingSource(source), Value: values[variableValue], Description: values[variableNote]})
	}
	return out, nil
}

func variablesConfig(variables []VariableSource) []interface{} {
	out := make([]interface{}, 0, len(variables))
	for _, v := range variables {
		item := map[string]interface{}{variableName: v.Name, variableSource: string(v.Source)}
		if v.Value != "" {
			item[variableValue] = v.Value
		}
		if v.Description != "" {
			item[variableNote] = v.Description
		}
		out = append(out, item)
	}
	return out
}

func (d Destination) AIVariables() []VariableSource {
	var out []VariableSource
	for _, v := range d.Variables {
		if v.Source == SourceAI {
			out = append(out, v)
		}
	}
	return out
}

func (d Destination) CheckMapping(needs automationneeds.Needs, defs []*customfield.Definition) error {
	var unneeded, picked []string
	mapped := map[string]bool{}
	var checked []conversationvars.Binding
	for _, v := range d.Variables {
		mapped[v.Name] = true
		need := needs.Find(v.Name)
		if need == nil {
			unneeded = append(unneeded, v.Name)
			continue
		}
		if v.Source == SourceAI && len(need.SheetUses) > 0 {
			picked = append(picked, v.Name)
			continue
		}
		if v.Source != SourceAI {
			checked = append(checked, conversationvars.Binding{Name: v.Name, Source: v.Source, Value: v.Value})
		}
	}
	if len(unneeded) > 0 {
		return &conversationvars.VariablesError{Cause: ErrVariableNotNeeded, Names: unneeded}
	}
	if len(picked) > 0 {
		return &conversationvars.VariablesError{Cause: ErrVariableSource, Names: picked}
	}
	var unmapped []string
	for _, need := range needs {
		if !mapped[need.Name] && len(need.SheetUses) == 0 {
			unmapped = append(unmapped, need.Name)
		}
	}
	if len(unmapped) > 0 {
		return &conversationvars.VariablesError{Cause: ErrVariableUnmapped, Names: unmapped}
	}
	if len(checked) == 0 {
		return nil
	}
	if _, err := conversationvars.Assign(checked, defs); err != nil {
		var named *conversationvars.VariablesError
		if errors.As(err, &named) {
			return &conversationvars.VariablesError{Cause: ErrVariableSource, Names: named.Names}
		}
		return &conversationvars.VariablesError{Cause: ErrVariableSource, Names: namesOf(checked)}
	}
	return nil
}

func (d Destination) Bindings(needs automationneeds.Needs, provided map[string]string) ([]conversationvars.Binding, []Filled, error) {
	var bindings []conversationvars.Binding
	var filled []Filled
	var missing []string
	for _, v := range d.Variables {
		if needs.Find(v.Name) == nil {
			continue
		}
		binding := conversationvars.Binding{Name: v.Name, Source: v.Source, Value: v.Value}
		if v.Source == SourceAI {
			value := strings.TrimSpace(provided[v.Name])
			if value == "" {
				missing = append(missing, v.Name)
				continue
			}
			binding = conversationvars.Binding{Name: v.Name, Source: campaign.BindLiteral, Value: value}
		}
		bindings = append(bindings, binding)
		filled = append(filled, Filled{Name: v.Name, Source: v.Source})
	}
	if len(missing) > 0 {
		return nil, nil, &conversationvars.VariablesError{Cause: ErrModelValueMissing, Names: missing}
	}
	return bindings, filled, nil
}

func namesOf(bindings []conversationvars.Binding) []string {
	out := make([]string, 0, len(bindings))
	for _, b := range bindings {
		out = append(out, b.Name)
	}
	return out
}

func filledDetail(filled []Filled) string {
	parts := make([]string, 0, len(filled))
	for _, f := range filled {
		parts = append(parts, f.Name+filledAssignment+string(f.Source))
	}
	return strings.Join(parts, filledSeparator)
}

func filledOf(detail string) []Filled {
	if strings.TrimSpace(detail) == "" {
		return nil
	}
	var out []Filled
	for _, part := range strings.Split(detail, filledSeparator) {
		name, source, ok := strings.Cut(part, filledAssignment)
		if ok && name != "" && !slices.ContainsFunc(out, func(f Filled) bool { return f.Name == name }) {
			out = append(out, Filled{Name: name, Source: campaign.BindingSource(source)})
		}
	}
	return out
}
