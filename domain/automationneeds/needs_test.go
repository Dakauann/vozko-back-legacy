package automationneeds

import (
	"errors"
	"reflect"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/conversation"
	"vozko/domain/workflow"
)

type fakeSource struct {
	agents    map[string]*agent.Agent
	workflows map[string]*workflow.Workflow
	failing   map[string]bool
}

func (f fakeSource) Agent(id string) (*agent.Agent, error) {
	if f.failing[id] {
		return nil, errors.New("store down")
	}
	return f.agents[id], nil
}

func (f fakeSource) Workflow(id string) (*workflow.Workflow, error) {
	if f.failing[id] {
		return nil, errors.New("store down")
	}
	return f.workflows[id], nil
}

func agentRef(id string) conversation.Automation {
	return conversation.Automation{Kind: conversation.AutomationAgent, ID: id}
}

func workflowRef(id string) conversation.Automation {
	return conversation.Automation{Kind: conversation.AutomationWorkflow, ID: id}
}

func nodes(list ...workflow.Node) workflow.Graph { return workflow.Graph{Nodes: list} }

func textNode(id, text string) workflow.Node {
	return workflow.Node{ID: id, Type: workflow.NodeTypeActionSendText, Config: map[string]interface{}{"text": text}}
}

func aiNode(id, agentID string) workflow.Node {
	return workflow.Node{ID: id, Type: workflow.NodeTypeActionAIAgent, Config: map[string]interface{}{"source": "agent", "agent_id": agentID}}
}

func subFlowNode(id, workflowID string) workflow.Node {
	return workflow.Node{ID: id, Type: workflow.NodeTypeActionRunWorkflow, Config: map[string]interface{}{"workflow_id": workflowID}}
}

func source() fakeSource {
	return fakeSource{
		agents: map[string]*agent.Agent{
			"escola": {ID: "escola", WorkspaceID: "ws", Name: "Escola", MessagingPrompt: "Olá {{lead_name}} da turma {{turma}}", Variables: []agent.AgentVariable{
				{Name: "escola", Description: "Nome da escola"},
				{Name: "cidade", DefaultValue: "Curitiba"},
			}},
			"livre":  {ID: "livre", WorkspaceID: "ws", Name: "Livre", MessagingPrompt: "Sem variáveis"},
			"caller": {ID: "caller", WorkspaceID: "ws", Name: "Chamador", InternalTools: []agent.ToolBinding{{Name: workflow.AgentRunWorkflowTool, Config: map[string]interface{}{"workflow_id": "cobranca"}}}},
			"alheio": {ID: "alheio", WorkspaceID: "other", Name: "Alheio"},
		},
		workflows: map[string]*workflow.Workflow{
			"cobranca": {ID: "cobranca", WorkspaceID: "ws", Name: "Cobrança", Graph: nodes(textNode("n1", "Oi {{ campvars.nome_cliente }}, valor {{var.campvars.valor}} e {{campvars.campaign_name}}"))},
			"com-ia":   {ID: "com-ia", WorkspaceID: "ws", Name: "Com IA", Graph: nodes(aiNode("n1", "escola"))},
			"pai":      {ID: "pai", WorkspaceID: "ws", Name: "Pai", Graph: nodes(subFlowNode("n1", "cobranca"), textNode("n2", "{{campvars.extra}}"))},
			"ciclo-a":  {ID: "ciclo-a", WorkspaceID: "ws", Name: "A", Graph: nodes(subFlowNode("n1", "ciclo-b"), textNode("n2", "{{campvars.a}}"))},
			"ciclo-b":  {ID: "ciclo-b", WorkspaceID: "ws", Name: "B", Graph: nodes(subFlowNode("n1", "ciclo-a"))},
			"modelo":   {ID: "modelo", WorkspaceID: "ws", Name: "Modelo", Graph: nodes(subFlowNode("n1", "{{var.alvo}}"))},
			"orfao":    {ID: "orfao", WorkspaceID: "ws", Name: "Órfão", Graph: nodes(aiNode("n1", "sumiu"))},
			"inline":   {ID: "inline", WorkspaceID: "ws", Name: "Inline", Graph: nodes(workflow.Node{ID: "n1", Type: workflow.NodeTypeActionAIAgent, Config: map[string]interface{}{"source": "prompt", "agent_id": "escola", "instructions": "{{campvars.tom}}"}})},
		},
	}
}

func TestOfNamesWhatEachAutomationNeedsPerConversation(t *testing.T) {
	cases := []struct {
		name string
		root conversation.Automation
		want []string
	}{
		{"an agent needs its undeclared and default-less variables, never system keys", agentRef("escola"), []string{"escola", "turma"}},
		{"an agent without variables needs nothing", agentRef("livre"), nil},
		{"a workflow needs its campvars, written with or without spaces", workflowRef("cobranca"), []string{"nome_cliente", "valor"}},
		{"a workflow inherits the agent its AI node runs", workflowRef("com-ia"), []string{"escola", "turma"}},
		{"a workflow inherits the sub-flows it calls", workflowRef("pai"), []string{"extra", "nome_cliente", "valor"}},
		{"an agent inherits the workflows its run_workflow tool calls", agentRef("caller"), []string{"nome_cliente", "valor"}},
		{"a cycle of sub-flows stops at the first repeat", workflowRef("ciclo-a"), []string{"a"}},
		{"a templated sub-flow id cannot be followed", workflowRef("modelo"), nil},
		{"an inline prompt AI node does not run the saved agent", workflowRef("inline"), []string{"tom"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Of(source(), "ws", tc.root)
			if err != nil {
				t.Fatalf("Of: %v", err)
			}
			if names := got.Names(); !reflect.DeepEqual(names, tc.want) {
				t.Fatalf("names = %v, want %v", names, tc.want)
			}
		})
	}
}

func TestOfRecordsWhichAutomationNeedsEachVariable(t *testing.T) {
	got, err := Of(source(), "ws", workflowRef("com-ia"))
	if err != nil {
		t.Fatal(err)
	}
	escola := got.Find("escola")
	if escola == nil || escola.Description != "Nome da escola" {
		t.Fatalf("escola = %+v, want the agent's description", escola)
	}
	want := []conversation.Automation{workflowRef("com-ia"), agentRef("escola")}
	if !reflect.DeepEqual(escola.Path(), want) {
		t.Fatalf("via = %+v, want %+v", escola.Via, want)
	}
	if escola.Via[0].Name != "Com IA" || escola.Owner().Name != "Escola" {
		t.Fatalf("via = %+v, want the names of the chain", escola.Via)
	}
	if escola.Owner().Automation != agentRef("escola") {
		t.Fatalf("owner = %+v, want the agent", escola.Owner())
	}
}

func TestOfFailsClosedWhenAnAutomationCannotBeRead(t *testing.T) {
	src := source()
	src.failing = map[string]bool{"escola": true}
	cases := []struct {
		name string
		src  fakeSource
		root conversation.Automation
		want error
	}{
		{"an unreadable agent behind an AI node", src, workflowRef("com-ia"), ErrUnreadable},
		{"an agent that no longer exists", source(), workflowRef("orfao"), ErrNotFound},
		{"an agent of another workspace", source(), agentRef("alheio"), ErrNotFound},
		{"a workflow that does not exist", source(), workflowRef("nada"), ErrNotFound},
		{"an automation without a kind", source(), conversation.Automation{ID: "x"}, ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Of(tc.src, "ws", tc.root); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestMissingIgnoresSuppliedValuesAndCountsBlankTextAsMissing(t *testing.T) {
	needs, err := Of(source(), "ws", workflowRef("pai"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		values map[string]string
		want   []string
	}{
		{"nothing supplied", nil, []string{"extra", "nome_cliente", "valor"}},
		{"blank text is missing", map[string]string{"extra": " ", "nome_cliente": "Ana", "valor": "10"}, []string{"extra"}},
		{"everything supplied", map[string]string{"extra": "x", "nome_cliente": "Ana", "valor": "10"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needs.Missing(tc.values).Names(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("missing = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMissingMetadataReadsAnyTypedValues(t *testing.T) {
	needs, err := Of(source(), "ws", workflowRef("cobranca"))
	if err != nil {
		t.Fatal(err)
	}
	got := needs.MissingMetadata(map[string]any{"nome_cliente": "Ana", "valor": 10}).Names()
	if len(got) != 0 {
		t.Fatalf("missing = %v, want none: a number is a value", got)
	}
	if got := needs.MissingMetadata(map[string]any{"valor": ""}).Names(); !reflect.DeepEqual(got, []string{"nome_cliente", "valor"}) {
		t.Fatalf("missing = %v", got)
	}
}

func TestSystemKeysAreAlwaysSupplied(t *testing.T) {
	for _, key := range []string{"lead_name", "lead_number", "campaign_name"} {
		if !IsSystemKey(key) {
			t.Fatalf("%s should be a system key", key)
		}
	}
	if IsSystemKey("nome_cliente") {
		t.Fatal("a campaign column is not a system key")
	}
}

func TestOverlayAnswersTheCandidateInsteadOfThePublishedVersion(t *testing.T) {
	draft := &agent.Agent{ID: "livre", WorkspaceID: "ws", Name: "Livre", MessagingPrompt: "Agora com {{novo}}"}
	got, err := Of(OverlayAgent(source(), draft), "ws", workflowRef("com-ia"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Names(), []string{"escola", "turma"}) {
		t.Fatalf("an unrelated overlay must not change the workflow: %v", got.Names())
	}
	got, err = Of(OverlayAgent(source(), draft), "ws", agentRef("livre"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Names(), []string{"novo"}) {
		t.Fatalf("names = %v, want the draft's variable", got.Names())
	}
	draftFlow := &workflow.Workflow{ID: "cobranca", WorkspaceID: "ws", Name: "Cobrança", Graph: nodes(textNode("n1", "{{campvars.so_isso}}"))}
	got, err = Of(OverlayWorkflow(source(), draftFlow), "ws", agentRef("caller"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Names(), []string{"so_isso"}) {
		t.Fatalf("names = %v, want the candidate graph seen through the caller", got.Names())
	}
}

func TestDependsOnTellsWhetherAnAutomationRunsAnother(t *testing.T) {
	cases := []struct {
		root, on conversation.Automation
		want     bool
	}{
		{workflowRef("com-ia"), agentRef("escola"), true},
		{agentRef("caller"), workflowRef("cobranca"), true},
		{workflowRef("pai"), workflowRef("cobranca"), true},
		{workflowRef("pai"), agentRef("escola"), false},
		{agentRef("escola"), agentRef("escola"), true},
	}
	for _, tc := range cases {
		got, err := DependsOn(source(), "ws", tc.root, tc.on)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("DependsOn(%v, %v) = %v, want %v", tc.root, tc.on, got, tc.want)
		}
	}
}

func TestUnionKeepsTheFirstNeedOfEachNameInOrder(t *testing.T) {
	left := Needs{{Name: "b", Via: []Link{{Automation: workflowRef("w")}}}, {Name: "d"}}
	right := Needs{{Name: "a"}, {Name: "b", Via: []Link{{Automation: agentRef("x")}}}}
	got := left.Union(right)
	if !reflect.DeepEqual(got.Names(), []string{"a", "b", "d"}) {
		t.Fatalf("names = %v", got.Names())
	}
	if got.Find("b").Owner().Automation != workflowRef("w") {
		t.Fatalf("b = %+v, want the left need", got.Find("b"))
	}
}

func TestOfAgentNamesWhatOneAgentNeedsWithoutFollowingItsTools(t *testing.T) {
	src := source()
	got := OfAgent(src.agents["caller"])
	if len(got) != 0 {
		t.Fatalf("the caller declares nothing itself: %v", got.Names())
	}
	got = OfAgent(src.agents["escola"])
	if !reflect.DeepEqual(got.Names(), []string{"escola", "turma"}) || got.Find("turma").Owner().Name != "Escola" {
		t.Fatalf("needs = %+v", got)
	}
	if OfAgent(nil) != nil {
		t.Fatal("no agent needs nothing")
	}
}

func TestVariablesOfListsAgentVariablesWithADefaultAsOptional(t *testing.T) {
	got, err := VariablesOf(source(), "ws", workflowRef("com-ia"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Required.Names(), []string{"escola", "turma"}) {
		t.Fatalf("required = %v", got.Required.Names())
	}
	if !reflect.DeepEqual(got.Optional.Names(), []string{"cidade"}) {
		t.Fatalf("optional = %v", got.Optional.Names())
	}
	cidade := got.Optional.Find("cidade")
	if cidade.Default != "Curitiba" || cidade.Owner().Name != "Escola" || len(cidade.Via) != 2 {
		t.Fatalf("cidade = %+v", cidade)
	}
	needs, err := Of(source(), "ws", workflowRef("com-ia"))
	if err != nil || !reflect.DeepEqual(needs, got.Required) {
		t.Fatalf("Of = %v %v, want the required variables only", needs.Names(), err)
	}
}

func TestAVariableRequiredSomewhereIsNeverOptional(t *testing.T) {
	src := source()
	src.workflows["mix"] = &workflow.Workflow{ID: "mix", WorkspaceID: "ws", Name: "Mix", Graph: nodes(aiNode("n1", "escola"), textNode("n2", "{{campvars.cidade}}"))}
	got, err := VariablesOf(src, "ws", workflowRef("mix"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Required.Find("cidade") == nil || got.Optional.Find("cidade") != nil {
		t.Fatalf("required %v optional %v", got.Required.Names(), got.Optional.Names())
	}
	left := Variables{Optional: Needs{{Name: "cidade", Default: "Curitiba"}}}
	right := Variables{Required: Needs{{Name: "cidade"}}}
	merged := left.Union(right)
	if merged.Required.Find("cidade") == nil || len(merged.Optional) != 0 {
		t.Fatalf("union = %+v", merged)
	}
}

func TestOptionalOfAgentCarriesTheDefault(t *testing.T) {
	got := OptionalOfAgent(source().agents["escola"])
	if len(got) != 1 || got[0].Name != "cidade" || got[0].Default != "Curitiba" {
		t.Fatalf("optional = %+v", got)
	}
	if OptionalOfAgent(nil) != nil {
		t.Fatal("no agent has nothing optional")
	}
}
