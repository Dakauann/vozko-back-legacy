package workflow

import (
	"testing"

	"vozko/domain/variabledecl"
)

func conversationGraph() *Graph {
	return &Graph{
		Nodes: []Node{
			{ID: "m", Type: NodeTypeTriggerMessageReceived},
			{ID: "h", Type: NodeTypeActionHTTPRequest, Config: map[string]interface{}{"capture_variable": "cadastro", "capture_label": "Cadastro do aluno", "url": "https://x.test/{{campvars.matricula}}"}},
			{ID: "sv", Type: NodeTypeActionSetVariable, Config: map[string]interface{}{"variable": "saudacao", "variable_label": "Saudação", "value": "Oi {{ campvars.apelido }}"}},
			{ID: "s", Type: NodeTypeActionSendText, Config: map[string]interface{}{"text": "Oi"}},
		},
		Edges: []Edge{{Source: "m", Target: "h"}, {Source: "h", Target: "sv", Label: "sucesso"}, {Source: "sv", Target: "s"}},
	}
}

var conversationDeclarations = []variabledecl.Declaration{
	{Name: "matricula", Label: "Matrícula", Type: variabledecl.Type{Kind: variabledecl.KindNumber}},
	{Name: "vencimento", Label: "Vencimento", Type: variabledecl.Type{Kind: variabledecl.KindDate}},
}

func TestAConversationFlowOffersTheConversationDataByFriendlyName(t *testing.T) {
	vars := catalogAt(t, conversationGraph(), VariableSources{Declarations: conversationDeclarations}, "s")
	matricula := variableByRef(vars, "campvars.matricula")
	if matricula == nil || matricula.Label != "Matrícula" || matricula.Group != VariableGroupConversation || matricula.Kind != VariableKindConversation || matricula.Type != VariableTypeNumber || matricula.Undeclared {
		t.Fatalf("matricula = %+v", matricula)
	}
	if v := variableByRef(vars, "campvars.vencimento"); v == nil || v.Type != VariableTypeDate {
		t.Fatalf("a declared variable not used yet is still offered: %+v", v)
	}
	apelido := variableByRef(vars, "campvars.apelido")
	if apelido == nil || !apelido.Undeclared || apelido.Label != "Apelido" || apelido.Type != VariableTypeText {
		t.Fatalf("apelido = %+v", apelido)
	}
	lead := variableByRef(vars, "campvars.lead_name")
	if lead == nil || lead.LabelKey != "conversation_lead_name" || lead.Label != "Nome do contato" || lead.Undeclared {
		t.Fatalf("lead_name = %+v", lead)
	}
	if vars[0].Group != VariableGroupConversation {
		t.Fatalf("first group = %s, want the conversation data first", vars[0].Group)
	}
}

func TestAWebhookFlowHasNoConversationData(t *testing.T) {
	g := conversationGraph()
	g.Nodes[0] = Node{ID: "m", Type: NodeTypeTriggerWebhook}
	vars := catalogAt(t, g, VariableSources{Declarations: conversationDeclarations}, "s")
	if v := variableByRef(vars, "campvars.matricula"); v != nil {
		t.Fatalf("matricula = %+v", v)
	}
}

func TestProducedVariablesUseTheFriendlyNameTheUserGave(t *testing.T) {
	vars := catalogAt(t, conversationGraph(), VariableSources{}, "s")
	if v := variableByRef(vars, "saudacao"); v == nil || v.Label != "Saudação" {
		t.Fatalf("saudacao = %+v", v)
	}
	if v := variableByRef(vars, "cadastro"); v == nil || v.Label != "Cadastro do aluno" || !v.Open {
		t.Fatalf("cadastro = %+v", v)
	}
}

func TestSystemValuesCarryALabelKeyForTheFront(t *testing.T) {
	vars := catalogAt(t, conversationGraph(), VariableSources{}, "s")
	for ref, key := range map[string]string{"sys.date": "sys_date", "message": "conversation_message", "contact_number": "contact_number"} {
		if v := variableByRef(vars, ref); v == nil || v.LabelKey != key {
			t.Fatalf("%s = %+v, want key %s", ref, v, key)
		}
	}
}

func TestLabelOfResolvesExactAndOpenReferences(t *testing.T) {
	catalog := VariableCatalogAt(conversationGraph(), catalogDefs(), VariableSources{Specs: TriggerInputSpecsOf(conversationGraph(), TriggerInputSources{}), Declarations: conversationDeclarations}, "s")
	cases := map[string]string{
		"campvars.matricula":    "Matrícula",
		"cadastro.status":       "Cadastro do aluno › status",
		"cadastro.items.0.nome": "Cadastro do aluno › items › 0 › nome",
		"node.h.status_code":    "Requisição HTTP › Código HTTP",
		"sem.dono":              "",
	}
	for ref, want := range cases {
		if got, _ := catalog.LabelOf(ref); got != want {
			t.Errorf("LabelOf(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestEveryOfferedValueCarriesAnExample(t *testing.T) {
	declared := []variabledecl.Declaration{{Name: "matricula", Label: "Matrícula", Example: "2026-17", Type: variabledecl.Type{Kind: variabledecl.KindText}}}
	vars := catalogAt(t, conversationGraph(), VariableSources{Declarations: declared}, "s")
	cases := map[string]string{
		"campvars.matricula": "2026-17",
		"campvars.lead_name": variabledecl.SystemExample(variabledecl.KeyLeadName),
		"contact_number":     "5511999990000",
	}
	for ref, want := range cases {
		v := variableByRef(vars, ref)
		if v == nil || v.Example == nil || *v.Example != want {
			t.Fatalf("%s = %+v, want example %q", ref, v, want)
		}
	}
	date := variableByRef(vars, "sys.date")
	if date == nil || date.Example == nil || *date.Example == "" {
		t.Fatalf("sys.date = %+v", date)
	}
	status := variableByRef(vars, "node.h.status_code")
	if status == nil || status.Example == nil || *status.Example != "42" {
		t.Fatalf("status_code = %+v, want the number sample", status)
	}
}

func TestAnOutputExampleFromTheNodeSpecWins(t *testing.T) {
	defs := []NodeDefinition{{Type: NodeTypeActionHTTPRequest, Label: "Requisição HTTP", OutputKeys: []OutputKeyDefinition{{Key: "status_code", Label: "Código HTTP", Type: VariableTypeNumber, Example: "200"}}}}
	g := conversationGraph()
	vars := VariableCatalogAt(g, defs, VariableSources{Specs: TriggerInputSpecsOf(g, TriggerInputSources{})}, "s").Variables
	status := variableByRef(vars, "node.h.status_code")
	if status == nil || status.Example == nil || *status.Example != "200" {
		t.Fatalf("status_code = %+v", status)
	}
}
