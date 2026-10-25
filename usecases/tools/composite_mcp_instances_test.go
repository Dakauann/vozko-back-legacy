package tools_usecase

import (
	"strings"
	"testing"

	"vozko/domain/agent"
	domainmcp "vozko/domain/agent/mcp"
	"vozko/domain/tools"
	ucmcp "vozko/usecases/agent/mcp"
)

func mcpRegistry() *ucmcp.Registry {
	return ucmcp.NewRegistry(
		ucmcp.NewStaticCatalog(builtinDescriptor("alpha", "zeta", "beta"), builtinDescriptor("omega", "kappa")),
		connectedBindings{keys: []string{"alpha", "omega"}},
		noRemotes{},
		nil,
		nil,
	)
}

func mcpComposite() *CompositeToolService {
	native := NewService(NewMCPSourceTool(mcpRegistry()))
	return NewCompositeToolService(native, mcpRegistry(), oneCollection{})
}

func names(defs []tools.Definition) string {
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		out = append(out, def.Name)
	}
	return strings.Join(out, ",")
}

func TestAnMCPSourceInstanceOffersOnlyItsAllowedToolsAsFunctions(t *testing.T) {
	resolved := ResolveToolsWithOptions(mcpComposite(), []agent.ToolBinding{
		{ID: "m1", Name: MCPSourceToolName, Function: "mcp_alpha", Revision: 3, Config: map[string]interface{}{"source_id": "builtin:alpha", "tools": []interface{}{"beta"}}},
		{ID: "m2", Name: MCPSourceToolName, Function: "mcp_omega", Config: map[string]interface{}{"source_id": "builtin:omega", "all_tools": true}},
	}, agent.ToolVisibility(tools.VisibilityMessaging), ToolResolverOptions{Agent: &agent.Agent{ID: "ag-1", WorkspaceID: "ws-1"}})

	if got := names(resolved.Definitions); got != "builtin_alpha__beta,builtin_omega__kappa" {
		t.Fatalf("functions %s", got)
	}
	first := resolved.Definitions[0]
	if first.Instance == nil || first.Instance.ID != "m1" || first.Instance.Type != MCPSourceToolName || first.Instance.Revision != 3 || first.HandlerName != "" {
		t.Fatalf("instance %+v handler %q", first.Instance, first.HandlerName)
	}
}

func TestASourceThatIsNotConnectedOffersNothing(t *testing.T) {
	resolved := ResolveToolsWithOptions(mcpComposite(), []agent.ToolBinding{
		{ID: "m1", Name: MCPSourceToolName, Function: "mcp_x", Config: map[string]interface{}{"source_id": "remote:gone", "all_tools": true}},
	}, agent.ToolVisibility(tools.VisibilityMessaging), ToolResolverOptions{Agent: &agent.Agent{ID: "ag-1", WorkspaceID: "ws-1"}})
	if len(resolved.Definitions) != 0 {
		t.Fatalf("functions %s", names(resolved.Definitions))
	}
}

func TestTheSourceTypeItselfIsNeverOfferedToTheModel(t *testing.T) {
	resolved := ResolveTools(mcpComposite(), []agent.ToolBinding{
		{ID: "m1", Name: MCPSourceToolName, Function: "mcp_alpha", Config: map[string]interface{}{"source_id": "builtin:alpha", "all_tools": true}},
	}, agent.ToolVisibility(tools.VisibilityMessaging), ToolResolverOptions{})
	if len(resolved.Definitions) != 0 {
		t.Fatalf("functions %s", names(resolved.Definitions))
	}
}

func TestSavedAgentsWithLegacyCollectionsStillOfferThem(t *testing.T) {
	composite := NewCompositeToolService(NewService(NewMCPSourceTool(mcpRegistry())), mcpRegistry(), oneCollection{members: []domainmcp.CollectionMember{{Kind: domainmcp.CollectionMemberBuiltin, RefID: "omega"}}})
	resolved := ResolveToolsWithOptions(composite, []agent.ToolBinding{
		{ID: "m1", Name: MCPSourceToolName, Function: "mcp_alpha", Config: map[string]interface{}{"source_id": "builtin:alpha", "tools": []interface{}{"zeta"}}},
	}, agent.ToolVisibility(tools.VisibilityMessaging), ToolResolverOptions{Agent: &agent.Agent{ID: "ag-1", WorkspaceID: "ws-1", MCPCollectionIDs: []string{"col-1"}}})
	if got := names(resolved.Definitions); got != "builtin_alpha__zeta,builtin_omega__kappa" {
		t.Fatalf("functions %s", got)
	}
}
