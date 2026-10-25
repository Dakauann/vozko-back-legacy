package tools_usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domainmcp "vozko/domain/agent/mcp"
	"vozko/domain/agenttool"
	"vozko/domain/tools"
)

const (
	MCPSourceToolName  = "mcp_source"
	mcpSourceKey       = "source_id"
	mcpAllToolsKey     = "all_tools"
	mcpAllowedToolsKey = "tools"
)

var errMCPSourceDispatched = errors.New("mcp_source: the source offers its own functions and is never called by type")

type MCPSourceCatalog interface {
	Sources(ctx context.Context, ws domainmcp.WorkspaceID) ([]domainmcp.ToolSource, error)
}

type MCPSelection struct {
	SourceID string
	AllTools bool
	Tools    []string
}

func (s MCPSelection) Allows(tool string) bool {
	if s.AllTools {
		return true
	}
	for _, name := range s.Tools {
		if name == tool {
			return true
		}
	}
	return false
}

func MCPSelectionOf(config map[string]interface{}) MCPSelection {
	selection := MCPSelection{SourceID: strings.TrimSpace(configString(config, mcpSourceKey))}
	selection.AllTools, _ = config[mcpAllToolsKey].(bool)
	raw, _ := config[mcpAllowedToolsKey].([]interface{})
	for _, item := range raw {
		if name, ok := item.(string); ok && strings.TrimSpace(name) != "" {
			selection.Tools = append(selection.Tools, strings.TrimSpace(name))
		}
	}
	return selection
}

type mcpSourceTool struct {
	catalog MCPSourceCatalog
}

func NewMCPSourceTool(catalog MCPSourceCatalog) tools.Handler {
	if catalog == nil {
		panic("mcp_source: missing MCP source catalog")
	}
	return &mcpSourceTool{catalog: catalog}
}

func (t *mcpSourceTool) Definition() tools.Definition {
	return tools.Definition{
		Name:               MCPSourceToolName,
		DisplayName:        "Servidor MCP",
		Description:        "Oferece à IA as ferramentas escolhidas de um servidor MCP conectado ao workspace. Cada ferramenta permitida vira uma função própria.",
		DisplayDescription: "Escolha um servidor MCP conectado e quais das ferramentas dele a IA pode usar.",
		ConfigSchema: map[string]tools.ConfigParameter{
			mcpSourceKey: {
				Type:               "string",
				Description:        "Servidor MCP conectado ao workspace.",
				DisplayName:        "Servidor",
				DisplayDescription: "Um servidor MCP já conectado em Agentes, MCP.",
				OptionsSource:      "mcp_sources",
				Required:           true,
			},
			mcpAllToolsKey: {
				Type:               "boolean",
				Description:        "Oferece todas as ferramentas do servidor, inclusive as que ele ganhar depois.",
				DisplayName:        "Todas as ferramentas",
				DisplayDescription: "Ligado: a IA recebe todas as ferramentas do servidor.",
			},
			mcpAllowedToolsKey: {
				Type:               "array",
				Description:        "Nomes das ferramentas do servidor que a IA pode usar.",
				DisplayName:        "Ferramentas permitidas",
				DisplayDescription: "Só estas ferramentas chegam à IA.",
			},
		},
		RequiredConfig: []string{mcpSourceKey},
		RequiresConfig: true,
		Visibility:     []tools.ToolVisibility{tools.VisibilityMessaging},
		Category:       tools.CategoryAgentUtility,
		Instances:      tools.PerTarget(mcpSourceKey),
	}
}

func (t *mcpSourceTool) Execute(context.Context, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, errMCPSourceDispatched
}

func (t *mcpSourceTool) ExecuteWithConfig(context.Context, map[string]interface{}, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, errMCPSourceDispatched
}

func (t *mcpSourceTool) ValidateBinding(ctx context.Context, workspaceID string, config map[string]interface{}) error {
	selection := MCPSelectionOf(config)
	if selection.SourceID == "" {
		return tools.Invalid(agenttool.CodeMCPSourceInvalid, "escolha um servidor MCP")
	}
	if !selection.AllTools && len(selection.Tools) == 0 {
		return tools.Invalid(agenttool.CodeMCPSourceInvalid, "escolha ao menos uma ferramenta do servidor ou permita todas")
	}
	sources, err := t.catalog.Sources(ctx, domainmcp.WorkspaceID(workspaceID))
	if err != nil {
		return err
	}
	for _, source := range sources {
		if source.ID() != selection.SourceID {
			continue
		}
		if selection.AllTools {
			return nil
		}
		offered, err := source.ListTools(ctx, domainmcp.WorkspaceID(workspaceID))
		if err != nil {
			return err
		}
		names := map[string]bool{}
		for _, tool := range offered {
			names[tool.Name] = true
		}
		for _, name := range selection.Tools {
			if !names[name] {
				return tools.Invalid(agenttool.CodeMCPSourceInvalid, fmt.Sprintf("o servidor não oferece mais a ferramenta %q", name))
			}
		}
		return nil
	}
	return tools.Invalid(agenttool.CodeMCPSourceInvalid, "o servidor MCP não está conectado neste workspace")
}

func IsMCPSource(toolType string) bool {
	return strings.EqualFold(strings.TrimSpace(toolType), MCPSourceToolName)
}

type MCPToolView struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type MCPSourceView struct {
	ID          string        `json:"id"`
	Kind        string        `json:"kind"`
	Name        string        `json:"name"`
	Tools       []MCPToolView `json:"tools"`
	Unavailable bool          `json:"unavailable"`
}

func ListMCPSources(ctx context.Context, catalog MCPSourceCatalog, workspaceID string) ([]MCPSourceView, error) {
	ws := domainmcp.WorkspaceID(workspaceID)
	sources, err := catalog.Sources(ctx, ws)
	if err != nil {
		return nil, err
	}
	out := make([]MCPSourceView, 0, len(sources))
	for _, source := range sources {
		view := MCPSourceView{ID: source.ID(), Kind: string(source.Kind()), Name: source.DisplayName(), Tools: []MCPToolView{}}
		listed, err := source.ListTools(ctx, ws)
		if err != nil {
			view.Unavailable = true
			out = append(out, view)
			continue
		}
		for _, tool := range listed {
			view.Tools = append(view.Tools, MCPToolView{Name: tool.Name, Title: tool.Title, Description: tool.Description})
		}
		out = append(out, view)
	}
	return out, nil
}
