package agenttool

import (
	"errors"
	"fmt"
	"strings"

	"vozko/domain/agent"
	"vozko/domain/tools"
)

var (
	ErrToolNotFound      = errors.New("agenttool: tool not found on this agent")
	ErrTypeUnknown       = errors.New("agenttool: unknown tool type")
	ErrNameInvalid       = errors.New("agenttool: invalid function name")
	ErrNameTaken         = errors.New("agenttool: function name already used by this agent")
	ErrTargetTaken       = errors.New("agenttool: this agent already has a tool of this type for the same target")
	ErrSingleTypeTaken   = errors.New("agenttool: this tool type allows a single instance per agent")
	ErrRevisionConflict  = errors.New("agenttool: the tool changed since it was read")
	ErrValidationOffline = errors.New("agenttool: a validation check could not run")
)

type SetRules struct {
	Definitions map[string]tools.Definition
	Normalize   func(def tools.Definition, config map[string]interface{}) (map[string]interface{}, error)
	Reserved    func(name string) bool
	NewID       func() string
}

type prepared struct {
	binding agent.ToolBinding
	def     tools.Definition
	current *agent.ToolBinding
}

func PrepareSet(rules SetRules, current, next []agent.ToolBinding) ([]agent.ToolBinding, error) {
	items, err := matchIdentities(rules, current, next)
	if err != nil {
		return nil, err
	}
	if err := checkPolicies(items); err != nil {
		return nil, err
	}
	if err := assignNames(rules, items); err != nil {
		return nil, err
	}
	out := make([]agent.ToolBinding, len(items))
	for i, item := range items {
		out[i] = item.binding
	}
	return out, nil
}

func matchIdentities(rules SetRules, current, next []agent.ToolBinding) ([]*prepared, error) {
	byID := make(map[string]int, len(current))
	for i, c := range current {
		byID[c.ID] = i
	}
	used := make([]bool, len(current))
	items := make([]*prepared, 0, len(next))
	for _, b := range next {
		def, ok := rules.Definitions[strings.ToLower(strings.TrimSpace(b.Name))]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrTypeUnknown, b.Name)
		}
		b.Name = def.Name
		if rules.Normalize != nil && len(b.Config) > 0 {
			normalized, err := rules.Normalize(def, b.Config)
			if err != nil {
				return nil, err
			}
			b.Config = normalized
		}
		item := &prepared{binding: b, def: def}
		if b.ID != "" {
			i, found := byID[b.ID]
			if !found || used[i] || !strings.EqualFold(current[i].Name, def.Name) {
				return nil, fmt.Errorf("%w: %s", ErrToolNotFound, b.ID)
			}
			used[i] = true
			item.current = &current[i]
		} else if i, found := sameKind(def, current, used, b.Config); found {
			used[i] = true
			item.current = &current[i]
			item.binding.ID = current[i].ID
		} else {
			item.binding.ID = rules.NewID()
		}
		if item.binding.Description == "" && item.current != nil {
			item.binding.Description = item.current.Description
		}
		item.binding.TargetKey = TargetKeyOf(def, item.binding.Config)
		items = append(items, item)
	}
	return items, nil
}

func sameKind(def tools.Definition, current []agent.ToolBinding, used []bool, config map[string]interface{}) (int, bool) {
	target := def.TargetOf(config)
	for i, c := range current {
		if used[i] || !strings.EqualFold(c.Name, def.Name) {
			continue
		}
		if def.Instances.IsPerTarget() && def.TargetOf(c.Config) != target {
			continue
		}
		return i, true
	}
	return 0, false
}

func checkPolicies(items []*prepared) error {
	singles := map[string]bool{}
	targets := map[string]bool{}
	for _, item := range items {
		def := item.def
		switch def.Instances.Effective() {
		case tools.InstancesSingle:
			if singles[def.Name] {
				return fmt.Errorf("%w: %s", ErrSingleTypeTaken, def.Name)
			}
			singles[def.Name] = true
		case tools.InstancesPerTarget:
			key := def.Name + "\x00" + def.TargetOf(item.binding.Config)
			if targets[key] {
				return fmt.Errorf("%w: %s", ErrTargetTaken, def.Name)
			}
			targets[key] = true
		}
	}
	return nil
}

func assignNames(rules SetRules, items []*prepared) error {
	singleTypes := map[string]bool{}
	for _, def := range rules.Definitions {
		if def.Instances.Effective() == tools.InstancesSingle {
			singleTypes[strings.ToLower(def.Name)] = true
		}
	}
	used := map[string]bool{}
	claim := func(item *prepared, name string) error {
		key := strings.ToLower(name)
		if used[key] || (singleTypes[key] && !strings.EqualFold(item.def.Name, name)) {
			return fmt.Errorf("%w: %s", ErrNameTaken, name)
		}
		used[key] = true
		item.binding.Function = name
		return nil
	}
	for _, item := range items {
		name := item.binding.Function
		if name == "" && item.current != nil {
			name = item.current.Function
		}
		if name == "" && item.def.Instances.Effective() == tools.InstancesSingle {
			name = strings.ToLower(item.def.Name)
		}
		if name == "" {
			continue
		}
		if err := acceptable(rules, item, name); err != nil {
			return err
		}
		if err := claim(item, name); err != nil {
			return err
		}
	}
	taken := func(name string) bool {
		key := strings.ToLower(name)
		return used[key] || singleTypes[key] || (rules.Reserved != nil && rules.Reserved(name))
	}
	for _, item := range items {
		if item.binding.Function != "" {
			continue
		}
		name := item.def.LegacyName(item.binding.Config)
		if !item.def.Instances.IsPerTarget() || name == "" || taken(name) {
			name = ProposeName(item.def.Name, taken)
		}
		if err := claim(item, name); err != nil {
			return err
		}
	}
	return nil
}

func acceptable(rules SetRules, item *prepared, name string) error {
	if strings.Contains(name, mcpNameSeparator) || (rules.Reserved != nil && rules.Reserved(name)) {
		return fmt.Errorf("%w: %s", ErrNameInvalid, name)
	}
	if ValidName(name) {
		return nil
	}
	if item.current != nil && item.current.Function == name && tools.ValidToolName(name) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrNameInvalid, name)
}

const singleTargetKey = "single"

func TargetKeyOf(def tools.Definition, config map[string]interface{}) string {
	switch def.Instances.Effective() {
	case tools.InstancesSingle:
		return singleTargetKey
	case tools.InstancesPerTarget:
		return "target:" + def.TargetOf(config)
	}
	return ""
}

var (
	ErrDescriptionRequired = errors.New("agenttool: describe when the model should use this tool")
	ErrOrderMismatch       = errors.New("agenttool: the order must list every tool of the agent once")
	ErrRevisionNotFound    = errors.New("agenttool: revision not found")
)
