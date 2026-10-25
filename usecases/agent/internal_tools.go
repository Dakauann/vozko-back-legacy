package agent_usecase

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/tools"
)

var deprecatedTools = map[string]struct{}{
	"query_knowledge_base":  {},
	"conversation_analysis": {},
}

type toolRegistryIndex struct {
	defs map[string]tools.Definition
	all  []string
}

func newToolRegistryIndex(registry tools.Service) *toolRegistryIndex {
	index := &toolRegistryIndex{
		defs: make(map[string]tools.Definition),
		all:  []string{},
	}
	if registry == nil {
		return index
	}
	for _, def := range registry.Definitions() {
		normalized := strings.TrimSpace(strings.ToLower(def.Name))
		if normalized == "" {
			continue
		}
		index.defs[normalized] = def
		index.all = append(index.all, def.Name)
	}
	sort.Strings(index.all)
	return index
}

func resolveInternalToolSelection(registry tools.Service, requested []agent.ToolBinding, fallback []agent.ToolBinding) ([]agent.ToolBinding, error) {
	index := newToolRegistryIndex(registry)

	var selected []agent.ToolBinding
	var err error
	switch {
	case requested == nil && fallback == nil:
		return nil, agent.ErrAgentInternalToolsRequired
	case requested == nil:
		selected = filterExistingToolBindings(index, fallback)
	default:
		selected, err = validateRequestedToolBindings(index, requested)
	}
	if err != nil {
		return nil, err
	}
	return agenttool.PrepareSet(setRulesFor(registry, index.defs), fallback, selected)
}

func filterExistingToolBindings(index *toolRegistryIndex, bindings []agent.ToolBinding) []agent.ToolBinding {
	result := make([]agent.ToolBinding, 0, len(bindings))
	for _, tb := range bindings {
		if def, ok := index.defs[strings.TrimSpace(strings.ToLower(tb.Name))]; ok {
			tb.Name = def.Name
			result = append(result, tb)
		}
	}
	return result
}

func validateRequestedToolBindings(index *toolRegistryIndex, bindings []agent.ToolBinding) ([]agent.ToolBinding, error) {
	if len(bindings) == 0 {
		return []agent.ToolBinding{}, nil
	}
	if len(index.defs) == 0 {
		names := make([]string, len(bindings))
		for i, tb := range bindings {
			names[i] = tb.Name
		}
		return nil, fmt.Errorf("%w: %s", agent.ErrAgentInternalToolInvalid, strings.Join(names, ", "))
	}
	result := make([]agent.ToolBinding, 0, len(bindings))
	for _, tb := range bindings {
		normalized := strings.TrimSpace(strings.ToLower(tb.Name))
		if normalized == "" {
			continue
		}
		if _, deprecated := deprecatedTools[normalized]; deprecated {
			continue
		}
		def, ok := index.defs[normalized]
		if !ok {
			return nil, fmt.Errorf("%w: %s", agent.ErrAgentInternalToolInvalid, tb.Name)
		}
		if err := def.ValidateConfig(tb.Config); err != nil {
			return nil, fmt.Errorf("%w: %s config invalid: %v", agent.ErrAgentInternalToolInvalid, def.Name, err)
		}
		tb.Name = def.Name
		result = append(result, tb)
	}
	return result, nil
}

func setRulesFor(registry tools.Service, defs map[string]tools.Definition) agenttool.SetRules {
	return agenttool.SetRules{
		Definitions: defs,
		NewID:       uuid.NewString,
		Normalize: func(def tools.Definition, config map[string]interface{}) (map[string]interface{}, error) {
			if registry == nil {
				return config, nil
			}
			handler, ok := registry.Handler(strings.ToLower(def.Name))
			if !ok {
				return config, nil
			}
			normalizer, ok := handler.(tools.ConfigNormalizer)
			if !ok {
				return config, nil
			}
			normalized, err := normalizer.NormalizeConfig(config)
			if err != nil {
				return nil, fmt.Errorf("%w: %s config invalid: %v", agent.ErrAgentInternalToolInvalid, def.Name, err)
			}
			return normalized, nil
		},
	}
}
