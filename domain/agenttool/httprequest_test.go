package agenttool

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"vozko/domain/calltrace"
	"vozko/domain/tools"
)

func orderConfig() HTTPConfig {
	c, err := ParseHTTPConfig(map[string]interface{}{
		"method": "post",
		"url":    "https://api.loja.test/v1/{loja}/pedidos/{pedido}?origem=vozko&k={{secret.loja_key}}",
		"fields": []interface{}{
			map[string]interface{}{"name": "loja", "location": "path", "source": "fixed", "value": "matriz"},
			map[string]interface{}{"name": "pedido", "location": "path", "source": "model", "type": "string", "description": "Número do pedido", "required": true},
			map[string]interface{}{"name": "canal", "location": "query", "source": "model", "type": "string", "enum": []interface{}{"site", "loja"}},
			map[string]interface{}{"name": "X-Conversa", "location": "header", "source": "context", "value": "{{conversation.entry_id}}"},
			map[string]interface{}{"name": "quantidade", "location": "body", "source": "model", "type": "integer"},
		},
		"auth":   map[string]interface{}{"mode": "bearer", "token": "{{secret.loja_token}}"},
		"output": map[string]interface{}{"fields": []interface{}{map[string]interface{}{"name": "status", "path": "data.status"}, map[string]interface{}{"name": "primeiro_item", "path": "data.itens[0].sku"}}, "include_status": true},
	})
	if err != nil {
		panic(err)
	}
	return c
}

func lookup(scope, key string) (string, bool) {
	values := map[string]string{"secret.loja_key": "k&1", "secret.loja_token": "tok", "conversation.entry_id": "e-9"}
	v, ok := values[scope+"."+key]
	return v, ok
}

func TestOnlyModelFieldsBecomeTypedParameters(t *testing.T) {
	params, required := orderConfig().Parameters()
	if len(params) != 3 || params["pedido"].Type != "string" || params["quantidade"].Type != "integer" || len(params["canal"].Enum) != 2 {
		t.Fatalf("params %+v", params)
	}
	if len(required) != 1 || required[0] != "pedido" {
		t.Fatalf("required %v", required)
	}
	def := tools.Definition{Name: "x", Parameters: params, Required: required}
	if issues := tools.SchemaIssues(def.JSONSchema()); len(issues) != 0 {
		t.Fatalf("schema issues %v", issues)
	}
}

func TestRenderFillsEveryLocationFromItsSource(t *testing.T) {
	got, err := orderConfig().Render(map[string]interface{}{"pedido": "A/1", "canal": "site", "quantidade": float64(2)}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "POST" || got.URL != "https://api.loja.test/v1/matriz/pedidos/A%2F1?origem=vozko&k=k%261&canal=site" {
		t.Fatalf("url %q", got.URL)
	}
	if got.Headers["X-Conversa"] != "e-9" || got.Body != `{"quantidade":2}` || got.Auth.Token != "tok" {
		t.Fatalf("got %+v", got)
	}
	if len(got.Secrets) != 2 {
		t.Fatalf("the used secret values must come back for scrubbing: %v", got.Secrets)
	}
}

func TestBadArgumentsAreReportedForTheModel(t *testing.T) {
	_, err := orderConfig().Render(map[string]interface{}{"canal": "telefone", "quantidade": 1.5}, lookup)
	var callErr *CallError
	if !errors.As(err, &callErr) || callErr.Kind != calltrace.ErrorKindArguments {
		t.Fatalf("err %v", err)
	}
	for _, want := range []string{"pedido é obrigatório", "canal precisa ser um destes", "quantidade precisa ser número inteiro"} {
		if !strings.Contains(callErr.Message, want) {
			t.Fatalf("%q missing from %q", want, callErr.Message)
		}
	}
}

func TestAMissingSecretOrContextValueStopsTheCall(t *testing.T) {
	_, err := orderConfig().Render(map[string]interface{}{"pedido": "1"}, func(scope, key string) (string, bool) {
		if scope == ScopeSecret && key == "loja_token" {
			return "", false
		}
		return lookup(scope, key)
	})
	var callErr *CallError
	if !errors.As(err, &callErr) || callErr.Kind != calltrace.ErrorKindSecret {
		t.Fatalf("err %v", err)
	}
}

func TestABodyTemplateRendersEveryValueAsJSON(t *testing.T) {
	c := orderConfig()
	c.BodyTemplate = `{"pedido": {{args.pedido}}, "conversa": {{conversation.entry_id}}, "nota": {{args.nota}}}`
	got, err := c.Render(map[string]interface{}{"pedido": "1"}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]interface{}
	if json.Unmarshal([]byte(got.Body), &body) != nil || body["pedido"] != "1" || body["conversa"] != "e-9" || body["nota"] != nil {
		t.Fatalf("body %s", got.Body)
	}
}

func TestModelArgumentsNeverReachHeaderTemplates(t *testing.T) {
	c := orderConfig()
	c.Fields = append(c.Fields, HTTPField{Name: "X-Inject", Location: LocationHeader, Source: SourceContext, Value: "{{args.pedido}}"})
	if _, err := c.Render(map[string]interface{}{"pedido": "1"}, lookup); err == nil {
		t.Fatal("a header template must not read model arguments")
	}
}

func TestOutputMappingSendsOnlyTheDeclaredPaths(t *testing.T) {
	out, err := orderConfig().MapOutput(200, []byte(`{"data":{"status":"enviado","itens":[{"sku":"X1"}],"cpf":"52998224725"}}`))
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]interface{})
	if m["status"] != 200 || m["primeiro_item"] != "X1" || len(m) != 2 {
		t.Fatalf("mapped %+v", m)
	}
	if _, err := orderConfig().MapOutput(200, []byte("<html>")); err == nil {
		t.Fatal("a mapping over a non JSON body must fail")
	}
	plain := HTTPConfig{}
	if out, _ := plain.MapOutput(200, []byte(`{"a":1}`)); out.(map[string]interface{})["a"] != float64(1) {
		t.Fatalf("without a mapping the JSON body goes as is: %v", out)
	}
}

func TestLegacyConfigsBecomeCanonicalFields(t *testing.T) {
	c, err := ParseHTTPConfig(map[string]interface{}{
		"url": "https://a.test/u/{id}", "method": "get", "timeout_seconds": float64(10),
		"headers":     map[string]interface{}{"Authorization": "{{secret.t}}", "Accept": "json"},
		"path_params": []interface{}{map[string]interface{}{"name": "id", "type": "string", "required": true}},
		"body_schema": []interface{}{map[string]interface{}{"name": "id", "type": "string"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Method != "GET" || len(c.Fields) != 4 || c.TimeoutSeconds != 10 {
		t.Fatalf("config %+v", c)
	}
	byArg := map[string]HTTPField{}
	for _, f := range c.Fields {
		byArg[f.ArgName()] = f
	}
	if byArg["Authorization"].Source != SourceContext || byArg["Accept"].Source != SourceFixed || byArg["id"].Location != LocationPath || byArg["body_id"].Location != LocationBody {
		t.Fatalf("fields %+v", c.Fields)
	}
	if !IsCanonicalHTTPConfig(c.Map()) {
		t.Fatal("the map form is canonical")
	}
}

func TestOutputPathSyntax(t *testing.T) {
	for path, want := range map[string]bool{"data.items[0].id": true, "status": true, "a..b": false, "": false, "a[x]": false} {
		if ValidOutputPath(path) != want {
			t.Errorf("%q", path)
		}
	}
}
