package tools_usecase

import (
	"net/http"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/tools"
)

func TestTwoHTTPInstancesReachTheModelWithTheirOwnNamesConfigsAndDescriptions(t *testing.T) {
	registry := NewService(NewHTTPRequestToolUseCase(clientEgress(http.DefaultClient), nil))
	resolved := ResolveTools(registry, []agent.ToolBinding{
		{ID: "t1", Name: "http_request", Function: "consultar_pedido", Description: "Consulta o status de um pedido.", Revision: 2, Config: map[string]interface{}{"url": "https://a.test/orders", "method": "GET"}},
		{ID: "t2", Name: "http_request", Function: "criar_ticket", Description: "Abre um ticket de suporte.", Revision: 5, Config: map[string]interface{}{"url": "https://a.test/tickets", "method": "POST"}},
	}, agent.ToolVisibility(tools.VisibilityMessaging), ToolResolverOptions{})

	if len(resolved.Definitions) != 2 {
		t.Fatalf("definitions %+v", resolved.Definitions)
	}
	first, second := resolved.Definitions[0], resolved.Definitions[1]
	if first.Name != "consultar_pedido" || first.HandlerName != "http_request" || second.Name != "criar_ticket" {
		t.Fatalf("names %q/%q %q", first.Name, first.HandlerName, second.Name)
	}
	if first.Instance == nil || first.Instance.ID != "t1" || first.Instance.Revision != 2 || first.Instance.Type != "http_request" {
		t.Fatalf("instance %+v", first.Instance)
	}
	if first.Description[:len("Consulta o status de um pedido.")] != "Consulta o status de um pedido." {
		t.Fatalf("description %q", first.Description)
	}
	if resolved.Configs["consultar_pedido"]["url"] != "https://a.test/orders" || resolved.Configs["criar_ticket"]["method"] != "POST" {
		t.Fatalf("configs %+v", resolved.Configs)
	}
}

func TestABindingWithoutAFunctionKeepsItsLegacyName(t *testing.T) {
	registry := NewService(newRunWorkflowHarness().tool)
	resolved := ResolveTools(registry, []agent.ToolBinding{
		{Name: "run_workflow", Config: map[string]interface{}{"workflow_id": "wf-a"}},
	}, agent.ToolVisibility(tools.VisibilityMessaging), ToolResolverOptions{})
	if len(resolved.Definitions) != 1 || resolved.Definitions[0].Name != "run_workflow_wf-a" || resolved.Definitions[0].HandlerName != "run_workflow" {
		t.Fatalf("definitions %+v", resolved.Definitions)
	}
}

func TestTwoBindingsWithTheSameFunctionNameOfferOnlyTheFirst(t *testing.T) {
	registry := NewService(NewHTTPRequestToolUseCase(clientEgress(http.DefaultClient), nil))
	resolved := ResolveTools(registry, []agent.ToolBinding{
		{ID: "t1", Name: "http_request", Function: "pedido", Config: map[string]interface{}{"url": "https://a", "method": "GET"}},
		{ID: "t2", Name: "http_request", Function: "pedido", Config: map[string]interface{}{"url": "https://b", "method": "GET"}},
	}, agent.ToolVisibility(tools.VisibilityMessaging), ToolResolverOptions{})
	if len(resolved.Definitions) != 1 || resolved.Configs["pedido"]["url"] != "https://a" {
		t.Fatalf("definitions %+v configs %+v", resolved.Definitions, resolved.Configs)
	}
}

func TestModelDefinitionRendersOneInstanceWhateverTheVisibility(t *testing.T) {
	registry := NewService(NewHTTPRequestToolUseCase(clientEgress(http.DefaultClient), nil))
	def, ok := ModelDefinition(registry, agent.ToolBinding{
		ID: "t1", Name: "http_request", Function: "consultar_pedido", Description: "Consulta o pedido.",
		Visibility: []agent.ToolVisibility{agent.ToolVisibility(tools.VisibilityAnalysis)},
		Config: map[string]interface{}{"url": "https://a.test/orders/{numero}", "method": "GET", "fields": []interface{}{
			map[string]interface{}{"name": "numero", "location": "path", "source": "model", "type": "string", "description": "Número do pedido", "required": true},
		}},
	}, ToolResolverOptions{})
	if !ok || def.Name != "consultar_pedido" || def.Description != "Consulta o pedido." {
		t.Fatalf("definition %+v ok %v", def, ok)
	}
	schema := def.JSONSchema()
	props, _ := schema["properties"].(map[string]interface{})
	if _, has := props["numero"]; !has {
		t.Fatalf("schema %+v", schema)
	}
	if required, _ := schema["required"].([]string); len(required) != 1 || required[0] != "numero" {
		t.Fatalf("required %+v", schema["required"])
	}
}

func TestModelDefinitionRefusesAnUnknownType(t *testing.T) {
	registry := NewService(NewHTTPRequestToolUseCase(clientEgress(http.DefaultClient), nil))
	if _, ok := ModelDefinition(registry, agent.ToolBinding{Name: "nao_existe"}, ToolResolverOptions{}); ok {
		t.Fatal("unknown type rendered")
	}
}
