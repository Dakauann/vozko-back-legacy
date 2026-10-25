package tools_usecase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/tools"
)

func TestTheOwnersThreeHTTPToolsResolveAndRunInTheSimulator(t *testing.T) {
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.Method+" "+r.URL.Path]++
		_, _ = w.Write([]byte(`{"data":{"status":"enviado","estoque":3}}`))
	}))
	defer srv.Close()
	real := NewService(NewHTTPRequestToolUseCase(clientEgress(srv.Client()), nil))
	bindings := []agent.ToolBinding{
		{ID: "t1", Name: "http_request", Function: "consultar_pedido", Description: "Consulta o status do pedido.", Config: map[string]interface{}{
			"url": srv.URL + "/pedidos/{numero}", "method": "GET",
			"fields": []interface{}{map[string]interface{}{"name": "numero", "location": "path", "source": "model", "type": "string", "description": "Número do pedido", "required": true}},
			"output": map[string]interface{}{"fields": []interface{}{map[string]interface{}{"name": "status", "path": "data.status"}}},
		}},
		{ID: "t2", Name: "http_request", Function: "abrir_chamado", Description: "Abre um chamado de suporte.", Config: map[string]interface{}{
			"url": srv.URL + "/chamados", "method": "POST",
			"fields": []interface{}{map[string]interface{}{"name": "assunto", "location": "body", "source": "model", "type": "string", "description": "Assunto"}},
		}},
		{ID: "t3", Name: "http_request", Function: "consultar_estoque", Description: "Consulta o estoque de um produto.", Config: map[string]interface{}{
			"url": srv.URL + "/estoque", "method": "GET",
			"fields": []interface{}{map[string]interface{}{"name": "sku", "location": "query", "source": "model", "type": "string", "description": "Código do produto"}},
			"output": map[string]interface{}{"fields": []interface{}{map[string]interface{}{"name": "estoque", "path": "data.estoque"}}},
		}},
	}
	resolved := ResolveTools(real, bindings, agent.ToolVisibilityMessaging, ToolResolverOptions{})
	if len(resolved.Definitions) != 3 {
		t.Fatalf("definitions %+v", resolved.Definitions)
	}
	for _, def := range resolved.Definitions {
		if def.Name == "consultar_pedido" && (def.Parameters["numero"].Type != "string" || len(def.Required) != 1) {
			t.Fatalf("schema %+v", def)
		}
	}
	sim := NewSimulatedToolService(real)
	run := func(name string, args map[string]interface{}) tools.ExecutionResult {
		def, _ := tools.Offered(resolved.Definitions, name)
		res, err := sim.ExecuteWithConfig(context.Background(), def.Handler(), resolved.Configs[name], args)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	order := run("consultar_pedido", map[string]interface{}{"numero": "42"})
	ticket := run("abrir_chamado", map[string]interface{}{"assunto": "troca"})
	stock := run("consultar_estoque", map[string]interface{}{"sku": "X1"})
	if order.Result.(map[string]interface{})["status"] != "enviado" || stock.Result.(map[string]interface{})["estoque"] != float64(3) {
		t.Fatalf("order %+v stock %+v", order, stock)
	}
	if hits["POST /chamados"] != 0 || !strings.Contains(stringOf(ticket.Result), "SIMULA") {
		t.Fatalf("the simulator must never send a POST: hits %v ticket %+v", hits, ticket)
	}
	if hits["GET /pedidos/42"] != 1 || hits["GET /estoque"] != 1 {
		t.Fatalf("hits %v", hits)
	}
}

func stringOf(v interface{}) string {
	s, _ := v.(string)
	return s
}
