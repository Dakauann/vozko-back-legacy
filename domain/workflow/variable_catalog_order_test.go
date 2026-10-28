package workflow

import "testing"

func orderDefs() []NodeDefinition {
	return append(catalogDefs(), NodeDefinition{Type: NodeTypeActionAIAgent, Label: "Agente IA", OutputKeys: []OutputKeyDefinition{
		{Key: "response_text", Label: "Resposta da IA", Type: VariableTypeText},
		{Key: "tool_name", Label: "Ferramenta chamada", Type: VariableTypeText},
		{Key: "tool_args", Label: "Argumentos da ferramenta", Type: VariableTypeObject},
	}})
}

func refsOf(vars []CatalogVariable, group VariableGroup) []string {
	var out []string
	for _, v := range vars {
		if v.Group == group {
			out = append(out, v.Ref)
		}
	}
	return out
}

func indexOf(refs []string, ref string) int {
	for i, r := range refs {
		if r == ref {
			return i
		}
	}
	return -1
}

func aiToolGraph() *Graph {
	return &Graph{
		Nodes: []Node{
			{ID: "t", Type: NodeTypeTriggerMessageReceived},
			{ID: "far", Type: NodeTypeActionHTTPRequest, Config: map[string]interface{}{}},
			{ID: "ai", Type: NodeTypeActionAIAgent, Config: map[string]interface{}{"source": "prompt", "custom_tools": []interface{}{
				map[string]interface{}{"name": "agendar", "parameters": []interface{}{map[string]interface{}{"name": "horario"}, map[string]interface{}{"name": "dia"}}},
			}}},
			{ID: "near", Type: NodeTypeActionHTTPRequest, Config: map[string]interface{}{}},
		},
		Edges: []Edge{{Source: "t", Target: "far"}, {Source: "far", Target: "ai", Label: "sucesso"}, {Source: "ai", Target: "near", Label: "agendar"}, {Source: "near", Target: "ai", Label: "sucesso"}},
	}
}

func TestTheNearestStepComesFirstAndAStepNeverRanksItselfFirstThroughALoop(t *testing.T) {
	g := aiToolGraph()
	vars := VariableCatalogAt(g, orderDefs(), VariableSources{Specs: TriggerInputSpecsOf(g, TriggerInputSources{})}, "near").Variables
	steps := refsOf(vars, VariableGroupNodes)
	if len(steps) == 0 || steps[0] != "node.ai.horario" {
		t.Fatalf("steps = %v, want the agent tool parameters first", steps)
	}
	if indexOf(steps, "node.ai.dia") != 1 {
		t.Fatalf("steps = %v, want the parameters in declaration order", steps)
	}
	agentLast, farFirst, ownFirst := indexOf(steps, "node.ai.tool_args"), indexOf(steps, "node.far.status_code"), indexOf(steps, "node.near.status_code")
	if agentLast < 0 || farFirst < 0 || ownFirst < 0 || agentLast > farFirst || farFirst > ownFirst {
		t.Fatalf("steps = %v, want the agent, then the farther step, then the step's own outputs from the loop", steps)
	}
}

func TestAToolParameterNamesItsToolKeepsItsTypeAndIsOfferedOnlyAfterThatTool(t *testing.T) {
	g := &Graph{
		Nodes: []Node{
			{ID: "t", Type: NodeTypeTriggerMessageReceived},
			{ID: "ai", Type: NodeTypeActionAIAgent, Config: map[string]interface{}{"source": "prompt", "custom_tools": []interface{}{
				map[string]interface{}{"name": "buscar_cep", "parameters": []interface{}{map[string]interface{}{"name": "cep", "type": "integer"}}},
				map[string]interface{}{"name": "agendar", "parameters": []interface{}{map[string]interface{}{"name": "turno", "type": "enum", "enum": []interface{}{"manhã", "tarde"}}}},
			}}},
			{ID: "cep_http", Type: NodeTypeActionHTTPRequest, Config: map[string]interface{}{}},
			{ID: "agenda_http", Type: NodeTypeActionHTTPRequest, Config: map[string]interface{}{}},
		},
		Edges: []Edge{{Source: "t", Target: "ai"}, {Source: "ai", Target: "cep_http", Label: "buscar_cep"}, {Source: "ai", Target: "agenda_http", Label: "agendar"}},
	}
	sources := VariableSources{Specs: TriggerInputSpecsOf(g, TriggerInputSources{})}
	atCep := VariableCatalogAt(g, orderDefs(), sources, "cep_http").Variables
	cep := variableByRef(atCep, "node.ai.cep")
	if cep == nil {
		t.Fatal("the tool parameter is offered after its tool")
	}
	if cep.Label != "Agente IA › Ferramenta buscar_cep › cep" || cep.LabelPrefix != "Agente IA" {
		t.Fatalf("label = %q prefix = %q", cep.Label, cep.LabelPrefix)
	}
	if cep.Type != VariableTypeNumber || cep.Example == nil || *cep.Example != "42" {
		t.Fatalf("cep = %+v, want the declared type and its sample", cep)
	}
	if variableByRef(atCep, "node.ai.turno") != nil {
		t.Fatal("another tool's parameter is not offered on this tool's path")
	}
	turno := variableByRef(VariableCatalogAt(g, orderDefs(), sources, "agenda_http").Variables, "node.ai.turno")
	if turno == nil || turno.Type != VariableTypeText || turno.Example == nil || *turno.Example != "manhã" {
		t.Fatalf("turno = %+v, want the first option as the example", turno)
	}
}

func TestAStepOnEveryPathComesBeforeAStepOnSomePathsAtTheSameDistance(t *testing.T) {
	g := &Graph{
		Nodes: []Node{
			{ID: "t", Type: NodeTypeTriggerWebhook},
			{ID: "a", Type: NodeTypeActionSendText, Config: map[string]interface{}{"text": "a"}},
			{ID: "c", Type: NodeTypeConditionBranch, Config: map[string]interface{}{}},
			{ID: "x", Type: NodeTypeActionSendText, Config: map[string]interface{}{"text": "x"}},
			{ID: "island", Type: NodeTypeActionSendText, Config: map[string]interface{}{"text": "solta"}},
		},
		Edges: []Edge{{Source: "t", Target: "c"}, {Source: "c", Target: "a", Label: "true"}, {Source: "a", Target: "x"}, {Source: "c", Target: "x", Label: "false"}},
	}
	vars := VariableCatalogAt(g, orderDefs(), VariableSources{Specs: TriggerInputSpecsOf(g, TriggerInputSources{})}, "x").Variables
	steps := refsOf(vars, VariableGroupNodes)
	if len(steps) == 0 || steps[0] != "node.c.matched" {
		t.Fatalf("steps = %v, want the condition on every path before the branch at the same distance", steps)
	}
	if indexOf(steps, "node.a.text") < 0 {
		t.Fatalf("steps = %v, the branch stays offered", steps)
	}
	if indexOf(steps, "node.island.text") >= 0 || indexOf(steps, "node.x.text") >= 0 {
		t.Fatalf("steps = %v, an unreachable step and the step itself are never offered", steps)
	}
}

func TestConversationDataHoldsOnlyVariablesAndTheContactGroupHoldsThePlatformFields(t *testing.T) {
	vars := catalogAt(t, conversationGraph(), VariableSources{Declarations: conversationDeclarations}, "s")
	for _, v := range vars {
		if v.Group == VariableGroupConversation && v.Kind != VariableKindConversation {
			t.Fatalf("%s in the conversation data is %s", v.Ref, v.Kind)
		}
	}
	lead := variableByRef(vars, "campvars.lead_name")
	if lead == nil || lead.Group != VariableGroupContact || lead.Kind != VariableKindSystem {
		t.Fatalf("lead_name = %+v, want the contact group", lead)
	}
	if v := variableByRef(vars, "message"); v == nil || v.Group != VariableGroupContact {
		t.Fatalf("message = %+v", v)
	}
	contact := refsOf(vars, VariableGroupContact)
	if indexOf(contact, "campvars.lead_name") != 0 || indexOf(contact, "campvars.lead_number") != 1 || indexOf(contact, "campvars.campaign_name") != 2 {
		t.Fatalf("contact = %v, want the platform fields first in a stable order", contact)
	}
	groups := []VariableGroup{}
	for _, v := range vars {
		if len(groups) == 0 || groups[len(groups)-1] != v.Group {
			groups = append(groups, v.Group)
		}
	}
	want := []VariableGroup{VariableGroupConversation, VariableGroupNodes, VariableGroupContact, VariableGroupSystem}
	if len(groups) != len(want) {
		t.Fatalf("groups = %v, want %v", groups, want)
	}
	for i := range want {
		if groups[i] != want[i] {
			t.Fatalf("groups = %v, want %v", groups, want)
		}
	}
}

func TestSearchRanksExactNamesFirstAndKeepsTheCatalogOrderInsideEachTier(t *testing.T) {
	catalog := VariableCatalog{Variables: []CatalogVariable{
		{Ref: "node.a.status", Label: "Busca › Status do pedido"},
		{Ref: "campvars.status", Label: "Status"},
		{Ref: "node.b.x", Label: "Statusbar"},
		{Ref: "node.c.y", Label: "Outro", LabelKey: "status_key"},
		{Ref: "sys.date", Label: "Sistema › data de hoje"},
	}}
	got := catalog.Search("  STATUS ")
	refs := make([]string, 0, len(got.Variables))
	for _, v := range got.Variables {
		refs = append(refs, v.Ref)
	}
	want := []string{"campvars.status", "node.b.x", "node.a.status"}
	if len(refs) != len(want) {
		t.Fatalf("refs = %v, want %v", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("refs = %v, want %v", refs, want)
		}
	}
	if all := catalog.Search(""); len(all.Variables) != len(catalog.Variables) {
		t.Fatalf("an empty search keeps everything: %d", len(all.Variables))
	}
	if accents := catalog.Search("SISTEMA DATA"); len(accents.Variables) != 0 {
		t.Fatalf("words must appear in order: %v", accents.Variables)
	}
	if byRef := catalog.Search("sys.date"); len(byRef.Variables) != 1 {
		t.Fatalf("search by technical name: %v", byRef.Variables)
	}
}

func TestReferenceLabelsNameEveryTokenOfEveryNodeAndMarkTheUnknownOnes(t *testing.T) {
	g := conversationGraph()
	g.Nodes[3].Config = map[string]interface{}{"text": "Oi {{campvars.matricula}} {{sys.date}} {{node.h.status_code}} {{cadastro.status}} {{sumiu.x}}", "nested": []interface{}{map[string]interface{}{"v": "{{ campvars.lead_name }}"}}}
	labels := ReferenceLabels(g, catalogDefs(), VariableSources{Specs: TriggerInputSpecsOf(g, TriggerInputSources{}), Declarations: conversationDeclarations})
	node := labels["s"]
	cases := map[string]RefLabel{
		"campvars.matricula": {Label: "Matrícula"},
		"sys.date":           {Label: "Sistema › data de hoje", LabelKey: "sys_date"},
		"node.h.status_code": {Label: "Requisição HTTP › Código HTTP", LabelKey: "output_status_code", LabelPrefix: "Requisição HTTP"},
		"cadastro.status":    {Label: "Cadastro do aluno › status", LabelDetail: "status"},
		"sumiu.x":            {Unknown: true},
		"campvars.lead_name": {Label: "Nome do contato", LabelKey: "conversation_lead_name"},
	}
	for ref, want := range cases {
		if got, ok := node[ref]; !ok || got != want {
			t.Errorf("%s = %+v, want %+v", ref, got, want)
		}
	}
	if _, ok := labels["t"]; ok {
		t.Fatal("a node without tokens has no entry")
	}
}
