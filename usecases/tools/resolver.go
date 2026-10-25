package tools_usecase

import (
	"context"
	"strings"

	"vozko/domain/agent"
	"vozko/domain/tools"
	"vozko/usecases/agentctx"
)

type ResolvedToolSet struct {
	Definitions []tools.Definition
	Configs     map[string]map[string]interface{}
}

type ToolResolverOptions struct {
	Agent        *agent.Agent
	CampaignID   string
	CampaignType string
	EntryID      string
	EntryType    string
}

func ResolveTools(
	registry tools.Service,
	bindings []agent.ToolBinding,
	visibility agent.ToolVisibility,
	opts ToolResolverOptions,
) ResolvedToolSet {
	if registry == nil || len(bindings) == 0 {
		return ResolvedToolSet{}
	}
	index := definitionIndex(registry)
	toolCtx := toolContextOf(opts)
	result := make([]tools.Definition, 0, len(bindings))
	configsMap := make(map[string]map[string]interface{})
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		def, ok := bindingDefinition(index, binding)
		if !ok || IsMCPSource(def.Name) || !bindingVisibleIn(def, binding, visibility) {
			continue
		}
		rendered, ok := renderBinding(registry, def, binding, toolCtx)
		if !ok {
			continue
		}
		if _, exists := seen[rendered.Name]; exists {
			continue
		}
		seen[rendered.Name] = struct{}{}
		if len(binding.Config) > 0 {
			configsMap[rendered.Name] = binding.Config
		}
		result = append(result, rendered)
	}
	return ResolvedToolSet{
		Definitions: result,
		Configs:     configsMap,
	}
}

func ModelDefinition(registry tools.Service, binding agent.ToolBinding, opts ToolResolverOptions) (tools.Definition, bool) {
	if registry == nil {
		return tools.Definition{}, false
	}
	def, ok := bindingDefinition(definitionIndex(registry), binding)
	if !ok {
		return tools.Definition{}, false
	}
	return renderBinding(registry, def, binding, toolContextOf(opts))
}

func definitionIndex(registry tools.Service) map[string]tools.Definition {
	allDefs := registry.Definitions()
	index := make(map[string]tools.Definition, len(allDefs))
	for _, def := range allDefs {
		key := strings.ToLower(strings.TrimSpace(def.Name))
		if key != "" {
			index[key] = def
		}
	}
	return index
}

func toolContextOf(opts ToolResolverOptions) tools.ToolContext {
	toolCtx := tools.ToolContext{}
	if opts.Agent != nil {
		toolCtx.Agent = opts.Agent
		toolCtx.WorkspaceID = opts.Agent.WorkspaceID
	}
	if opts.CampaignID != "" {
		toolCtx.CampaignID = opts.CampaignID
		toolCtx.CampaignType = opts.CampaignType
	}
	toolCtx.EntryID, toolCtx.EntryType = opts.EntryID, opts.EntryType
	return toolCtx
}

func bindingDefinition(index map[string]tools.Definition, binding agent.ToolBinding) (tools.Definition, bool) {
	key := strings.ToLower(strings.TrimSpace(binding.Name))
	if key == "" {
		return tools.Definition{}, false
	}
	if def, ok := index[key]; ok {
		return def, true
	}
	if canonical := CanonicalToolName(key); canonical != key {
		def, ok := index[canonical]
		return def, ok
	}
	return tools.Definition{}, false
}

func renderBinding(registry tools.Service, def tools.Definition, binding agent.ToolBinding, toolCtx tools.ToolContext) (tools.Definition, bool) {
	toolName := strings.ToLower(strings.TrimSpace(binding.Function))
	if toolName == "" {
		toolName = def.LegacyName(binding.Config)
	}
	if toolName == "" {
		return tools.Definition{}, false
	}
	handlerName := strings.TrimSpace(def.Name)
	expandedDef := def
	if h, ok := registry.Handler(strings.ToLower(handlerName)); ok {
		if ch, ok := h.(tools.ContextualHandler); ok {
			bindingCtx := toolCtx
			bindingCtx.Config = binding.Config
			expandedDef = ch.DefinitionWithContext(bindingCtx)
		}
	}
	if binding.Description != "" {
		expandedDef.Description = binding.Description
	}
	expandedDef.Name = toolName
	expandedDef.HandlerName = handlerName
	expandedDef.Instance = &tools.InstanceRef{ID: binding.ID, Type: handlerName, Revision: binding.Revision}
	return expandedDef, true
}

func bindingVisibleIn(def tools.Definition, binding agent.ToolBinding, visibility agent.ToolVisibility) bool {
	if !def.IsVisibleIn(tools.ToolVisibility(visibility)) {
		return false
	}
	if len(binding.Visibility) == 0 {
		return true
	}
	for _, v := range binding.Visibility {
		if v == visibility {
			return true
		}
	}
	return false
}

func ResolveToolsWithOptions(
	registry tools.Service,
	bindings []agent.ToolBinding,
	visibility agent.ToolVisibility,
	opts ToolResolverOptions,
) ResolvedToolSet {
	result := ResolveTools(registry, bindings, visibility, opts)
	if opts.Agent == nil {
		return result
	}
	offered := make(map[string]bool, len(result.Definitions))
	for _, def := range result.Definitions {
		offered[def.Name] = true
	}
	add := func(defs []tools.Definition) {
		for _, def := range defs {
			if !offered[def.Name] {
				offered[def.Name] = true
				result.Definitions = append(result.Definitions, def)
			}
		}
	}
	if expander, ok := registry.(interface {
		MCPDefinitionsForBindings(ctx context.Context, workspaceID string, bindings []agent.ToolBinding) []tools.Definition
	}); ok {
		add(expander.MCPDefinitionsForBindings(context.Background(), opts.Agent.WorkspaceID, visibleMCPSources(registry, bindings, visibility)))
	}
	if composite, ok := registry.(interface {
		MCPDefinitionsForAgent(ctx context.Context) []tools.Definition
	}); ok && opts.Agent.HasMCPCollections() {
		add(composite.MCPDefinitionsForAgent(agentctx.WithAgent(context.Background(), opts.Agent)))
	}
	return result
}

func visibleMCPSources(registry tools.Service, bindings []agent.ToolBinding, visibility agent.ToolVisibility) []agent.ToolBinding {
	index := definitionIndex(registry)
	out := []agent.ToolBinding{}
	for _, binding := range bindings {
		def, ok := bindingDefinition(index, binding)
		if ok && IsMCPSource(def.Name) && bindingVisibleIn(def, binding, visibility) {
			out = append(out, binding)
		}
	}
	return out
}
