package agenttool

import (
	"strings"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/lint"
	"vozko/domain/tools"
)

func ruleDefs() map[string]tools.Definition {
	return map[string]tools.Definition{
		"http_request": {Name: "http_request", Instances: tools.Many(), RequiredConfig: []string{"url", "method"}, ConfigSchema: map[string]tools.ConfigParameter{
			"url": {Type: "string"}, "method": {Type: "string"}, "headers": {Type: "object", Sensitive: true}, "timeout_seconds": {Type: "number"},
			"path_params": {Type: "array"}, "query_schema": {Type: "array"}, "body_schema": {Type: "array"},
		}},
		"manage_lead_memory": {Name: "manage_lead_memory"},
		"transfer_to_human":  {Name: "transfer_to_human"},
	}
}

const goodDescription = "Consulta o status de um pedido pelo número que o cliente informou. Use quando o cliente perguntar onde está a compra. Não use para trocas. Retorna o status e a previsão de entrega."

func httpBinding(id, name, url string, extra map[string]interface{}) agent.ToolBinding {
	config := map[string]interface{}{"url": url, "method": "GET"}
	for k, v := range extra {
		config[k] = v
	}
	return agent.ToolBinding{ID: id, Name: "http_request", Function: name, Description: goodDescription, Config: config}
}

func issueOf(r Report, code string) (Issue, bool) {
	for _, i := range r.Issues {
		if i.Code == code {
			return i, true
		}
	}
	return Issue{}, false
}

func requireIssue(t *testing.T, r Report, code string, severity lint.Severity) Issue {
	t.Helper()
	i, ok := issueOf(r, code)
	if !ok || i.Severity != severity {
		t.Fatalf("want %s %s, got %+v", severity, code, r.Issues)
	}
	return i
}

func requireNoIssue(t *testing.T, r Report, code string) {
	t.Helper()
	if i, ok := issueOf(r, code); ok {
		t.Fatalf("unexpected %+v", i)
	}
}

func check(tools ...agent.ToolBinding) Report {
	return Check(Subject{Tools: tools, Definitions: ruleDefs()})
}

func TestAGoodHTTPToolIsGreen(t *testing.T) {
	r := check(httpBinding("t1", "consultar_pedido", "https://api.loja.test/orders/{id}", map[string]interface{}{
		"path_params": []interface{}{map[string]interface{}{"name": "id", "type": "string", "description": "Número do pedido"}},
		"headers":     map[string]interface{}{"Authorization": "{{secret.loja_token}}"},
	}))
	if len(r.Issues) != 0 {
		t.Fatalf("issues %+v", r.Issues)
	}
}

func TestNamesFollowTheProviderRuleAndStayUnique(t *testing.T) {
	r := check(httpBinding("t1", "pedido status", "https://a.test", nil))
	requireIssue(t, r, CodeNameInvalid, lint.SeverityBlocking)
	r = check(httpBinding("t1", "run_workflow_3f2a-11", "https://a.test", nil))
	requireIssue(t, r, CodeNameUnreadable, lint.SeverityAdvisory)
	requireNoIssue(t, r, CodeNameInvalid)
	r = check(httpBinding("t1", "pedido", "https://a.test", nil), httpBinding("t2", "PEDIDO", "https://b.test", nil))
	requireIssue(t, r, CodeNameTaken, lint.SeverityBlocking)
	r = Check(Subject{Tools: []agent.ToolBinding{httpBinding("t1", "crm__buscar", "https://a.test", nil)}, Definitions: ruleDefs(), OfferedNames: []string{"crm__buscar"}})
	requireIssue(t, r, CodeNameTaken, lint.SeverityBlocking)
}

func TestDescriptionsGuideTheModel(t *testing.T) {
	missing := httpBinding("t1", "pedido", "https://a.test", nil)
	missing.Description = ""
	requireIssue(t, check(missing), CodeDescriptionMissing, lint.SeverityAdvisory)
	thin := httpBinding("t1", "pedido", "https://a.test", nil)
	thin.Description = "Consulta pedido."
	requireIssue(t, check(thin), CodeDescriptionThin, lint.SeverityAdvisory)
	twin := httpBinding("t2", "ticket", "https://b.test", nil)
	requireIssue(t, check(httpBinding("t1", "pedido", "https://a.test", nil), twin), CodeDescriptionsOverlap, lint.SeverityAdvisory)
	single := agent.ToolBinding{ID: "m", Name: "manage_lead_memory", Function: "manage_lead_memory"}
	if r := check(single); len(r.Issues) != 0 {
		t.Fatalf("a single type inherits its description: %+v", r.Issues)
	}
}

func TestHTTPURLRules(t *testing.T) {
	cases := map[string]string{
		"::not a url":                          CodeHTTPURLInvalid,
		"https://user:pw@api.test/x":           CodeHTTPURLInvalid,
		"https://{tenant}.api.test/x":          CodeHTTPURLInvalid,
		"https://{{secret.host}}/x":            CodeHTTPURLInvalid,
		"ftp://api.test/x":                     CodeHTTPURLInvalid,
		"http://api.test/x":                    CodeHTTPURLNotHTTPS,
		"https://api.test/x/{id}":              CodeHTTPFieldUnbound,
		"https://api.test/x?token=s3cr3tvalue": CodeSecretInPlainText,
	}
	for url, code := range cases {
		t.Run(url, func(t *testing.T) {
			requireIssue(t, check(httpBinding("t1", "pedido", url, nil)), code, lint.SeverityBlocking)
		})
	}
}

func TestAnUntouchedInsecureURLOnlyWarns(t *testing.T) {
	stored := httpBinding("t1", "pedido", "http://legacy.test/x", nil)
	r := Check(Subject{Tools: []agent.ToolBinding{stored}, Stored: []agent.ToolBinding{stored}, Definitions: ruleDefs()})
	requireIssue(t, r, CodeHTTPURLNotHTTPS, lint.SeverityAdvisory)
	edited := httpBinding("t1", "pedido", "http://legacy.test/y", nil)
	r = Check(Subject{Tools: []agent.ToolBinding{edited}, Stored: []agent.ToolBinding{stored}, Definitions: ruleDefs()})
	requireIssue(t, r, CodeHTTPURLNotHTTPS, lint.SeverityBlocking)
}

func TestCredentialsInPlainTextAreBlocked(t *testing.T) {
	cases := []map[string]interface{}{
		{"headers": map[string]interface{}{"Authorization": "Bearer live-token-123"}},
		{"headers": map[string]interface{}{"X-Client": "sk_live_51HxQabcdefghijklmnop"}},
		{"headers": map[string]interface{}{"X-Session": "Bearer abcdefghijklmnop"}},
	}
	for _, extra := range cases {
		requireIssue(t, check(httpBinding("t1", "pedido", "https://a.test", extra)), CodeSecretInPlainText, lint.SeverityBlocking)
	}
	ok := check(httpBinding("t1", "pedido", "https://a.test", map[string]interface{}{"headers": map[string]interface{}{"Accept": "application/json", "X-Source": "vozko"}}))
	requireNoIssue(t, ok, CodeSecretInPlainText)
}

func TestHTTPFieldsAndTimeout(t *testing.T) {
	dup := httpBinding("t1", "pedido", "https://a.test", map[string]interface{}{"query_schema": []interface{}{
		map[string]interface{}{"name": "q"}, map[string]interface{}{"name": "q"},
	}})
	requireIssue(t, check(dup), CodeHTTPFieldDuplicate, lint.SeverityBlocking)
	unused := httpBinding("t1", "pedido", "https://a.test", map[string]interface{}{"path_params": []interface{}{map[string]interface{}{"name": "id"}}})
	requireIssue(t, check(unused), CodeHTTPFieldUnused, lint.SeverityAdvisory)
	body := httpBinding("t1", "pedido", "https://a.test", map[string]interface{}{"body_schema": []interface{}{map[string]interface{}{"name": "x"}}})
	requireIssue(t, check(body), CodeHTTPMethodBody, lint.SeverityAdvisory)
	requireIssue(t, check(httpBinding("t1", "pedido", "https://a.test", map[string]interface{}{"timeout_seconds": 0.5})), CodeHTTPTimeoutRange, lint.SeverityBlocking)
	requireIssue(t, check(httpBinding("t1", "pedido", "https://a.test", map[string]interface{}{"timeout_seconds": float64(90)})), CodeHTTPTimeoutRange, lint.SeverityAdvisory)
}

func TestConfigAndTypeAreChecked(t *testing.T) {
	broken := agent.ToolBinding{ID: "t1", Name: "http_request", Function: "pedido", Description: goodDescription, Config: map[string]interface{}{"method": "GET"}}
	requireIssue(t, check(broken), CodeConfigInvalid, lint.SeverityBlocking)
	requireIssue(t, check(agent.ToolBinding{ID: "t2", Name: "teleport", Function: "teleport"}), CodeTypeUnknown, lint.SeverityBlocking)
}

func TestAgentLevelBudgets(t *testing.T) {
	r := Check(Subject{Definitions: ruleDefs(), OfferedNames: make([]string, 21), DefinitionChars: 44000})
	requireIssue(t, r, CodeToolsMany, lint.SeverityAdvisory)
	requireIssue(t, r, CodeToolsTokenBudget, lint.SeverityAdvisory)
}

func TestThePromptNamingAToolTheAgentLacksWarns(t *testing.T) {
	r := Check(Subject{
		Prompt:      "Quando o cliente pedir, use transfer_to_human. Antes, chame consultar_pedido.",
		Tools:       []agent.ToolBinding{httpBinding("t1", "consultar_pedido", "https://a.test", nil)},
		Definitions: ruleDefs(),
	})
	i := requireIssue(t, r, CodePromptUnknownTool, lint.SeverityAdvisory)
	if !strings.Contains(i.Message, "transfer_to_human") || strings.Contains(i.Message, "consultar_pedido") {
		t.Fatalf("message %q", i.Message)
	}
}

func TestEveryIssueOfAToolCarriesItsID(t *testing.T) {
	r := check(httpBinding("t9", "pedido status", "http://a.test", nil))
	for _, i := range r.Issues {
		if i.ToolID != "t9" {
			t.Fatalf("issue without tool id: %+v", i)
		}
	}
}

func canonicalHTTP(extra map[string]interface{}) agent.ToolBinding {
	config := map[string]interface{}{"url": "https://api.test/x", "method": "POST"}
	for k, v := range extra {
		config[k] = v
	}
	return agent.ToolBinding{ID: "t1", Name: "http_request", Function: "pedido", Description: goodDescription, Config: config}
}

func canonicalDefs() map[string]tools.Definition {
	defs := ruleDefs()
	def := defs["http_request"]
	for _, key := range []string{"fields", "body_template", "auth", "output", "idempotency_header"} {
		def.ConfigSchema[key] = tools.ConfigParameter{}
	}
	defs["http_request"] = def
	return defs
}

func checkCanonical(b agent.ToolBinding) Report {
	return Check(Subject{Tools: []agent.ToolBinding{b}, Definitions: canonicalDefs()})
}

func TestCanonicalHTTPRules(t *testing.T) {
	requireIssue(t, checkCanonical(canonicalHTTP(map[string]interface{}{"body_template": `{"pedido": "{{args.pedido}}" x}`})), CodeHTTPBodyNotJSON, lint.SeverityBlocking)
	requireIssue(t, checkCanonical(canonicalHTTP(map[string]interface{}{"output": map[string]interface{}{"fields": []interface{}{map[string]interface{}{"name": "s", "path": "a..b"}}}})), CodeHTTPOutputPathInvalid, lint.SeverityBlocking)
	requireIssue(t, checkCanonical(canonicalHTTP(map[string]interface{}{"fields": []interface{}{map[string]interface{}{"name": "X-Pedido", "location": "header", "source": "context", "value": "{{args.pedido}}"}}})), CodeHTTPFieldUnbound, lint.SeverityBlocking)
	requireIssue(t, checkCanonical(canonicalHTTP(map[string]interface{}{"auth": map[string]interface{}{"mode": "bearer"}})), CodeConfigInvalid, lint.SeverityBlocking)
	requireIssue(t, checkCanonical(canonicalHTTP(map[string]interface{}{"auth": map[string]interface{}{"mode": "bearer", "token": "plain-token"}})), CodeSecretInPlainText, lint.SeverityBlocking)
	requireIssue(t, checkCanonical(canonicalHTTP(map[string]interface{}{"fields": []interface{}{map[string]interface{}{"name": "pedido", "location": "nowhere", "source": "model"}}})), CodeConfigInvalid, lint.SeverityBlocking)
	ok := checkCanonical(canonicalHTTP(map[string]interface{}{
		"fields":        []interface{}{map[string]interface{}{"name": "pedido", "location": "body", "source": "model", "type": "string", "description": "Número"}},
		"body_template": `{"pedido": {{args.pedido}}, "conversa": {{conversation.entry_id}}}`,
		"auth":          map[string]interface{}{"mode": "bearer", "token": "{{secret.loja}}"},
		"output":        map[string]interface{}{"fields": []interface{}{map[string]interface{}{"name": "status", "path": "data.status"}}},
	}))
	if len(ok.Issues) != 0 {
		t.Fatalf("issues %+v", ok.Issues)
	}
}

func TestAKeyTypedIntoAPlainFieldIsBlocked(t *testing.T) {
	b := canonicalHTTP(map[string]interface{}{"fields": []interface{}{map[string]interface{}{"name": "X-Cliente", "location": "header", "source": "fixed", "value": "sk_live_51HxQabcdefghijklmnop"}}})
	requireIssue(t, checkCanonical(b), CodeSecretInPlainText, lint.SeverityBlocking)
}
