package automationneeds

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"vozko/domain/agent"
	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

type fakeAgents map[string]*agent.Agent

func (f fakeAgents) FindByID(id string) (*agent.Agent, error) {
	if a, ok := f[id]; ok {
		return a, nil
	}
	return nil, agent.ErrAgentNotFound
}

type fakeWorkflows map[string]*workflow.Workflow

func (f fakeWorkflows) FindByID(id string) (*workflow.Workflow, error) {
	if w, ok := f[id]; ok {
		return w, nil
	}
	return nil, workflow.ErrWorkflowNotFound
}

func (f fakeWorkflows) FindVersion(string, int) (*workflow.WorkflowVersion, error) { return nil, nil }

type fakeValues struct {
	values map[string]string
	err    error
}

func (f fakeValues) Values(context.Context, string, shared.EntryType) (map[string]string, error) {
	return f.values, f.err
}

func ref(kind conversation.AutomationKind, id string) conversation.Automation {
	return conversation.Automation{Kind: kind, ID: id}
}

func id(s string) *string { return &s }

func newService(t *testing.T, values fakeValues) *Service {
	t.Helper()
	s, err := NewService(ServiceDeps{
		Source: PublishedSource{
			Agents: fakeAgents{
				"escola": {ID: "escola", WorkspaceID: "ws", Name: "Escola", MessagingPrompt: "Turma {{turma}}"},
				"livre":  {ID: "livre", WorkspaceID: "ws", Name: "Livre", MessagingPrompt: "Oi {{lead_name}}"},
			},
			Workflows: fakeWorkflows{
				"cobranca": {ID: "cobranca", WorkspaceID: "ws", Name: "Cobrança", Graph: workflow.Graph{Nodes: []workflow.Node{{ID: "n", Type: workflow.NodeTypeActionSendText, Config: map[string]interface{}{"text": "{{campvars.valor}}"}}}}},
			},
		},
		Values:       values,
		Declarations: declared,
		Sheets:       &fakeSheetFiles{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type fakeDeclarations struct {
	rows []variabledecl.Declaration
	err  error
}

func (f fakeDeclarations) Catalog(context.Context, string) (variabledecl.Catalog, error) {
	return variabledecl.NewCatalog(f.rows), f.err
}

func (fakeDeclarations) Registry() *variabledecl.Registry {
	r, _ := variabledecl.NewRegistry(variabledecl.Builtin()...)
	return r
}

var declared = fakeDeclarations{rows: []variabledecl.Declaration{{Name: "valor", Label: "Valor da parcela", Type: variabledecl.Type{Kind: variabledecl.KindText}}}}

func TestRefusalsNameVariablesByTheirFriendlyLabel(t *testing.T) {
	s := newService(t, fakeValues{values: map[string]string{}})
	_, err := s.ValuesFor(context.Background(), "ws", "e1", shared.EntryTypeWhatsApp, ref(conversation.AutomationWorkflow, "cobranca"))
	refusal, ok := domain.RefusalOf(err)
	if !ok || refusal.Needs[0].Label != "Valor da parcela" || !strings.Contains(refusal.Sentence(), "Valor da parcela") || strings.Contains(refusal.Sentence(), "valor,") {
		t.Fatalf("refusal = %+v, sentence %q", refusal, refusal.Sentence())
	}
	if err := s.AdmitReceptive("ws", domain.SlotInstagram, conversation.ChannelAutomation{WorkflowID: id("cobranca"), EnableWorkflow: true}); err == nil || !strings.Contains(err.(*domain.Refusal).Sentence(), "Valor da parcela") {
		t.Fatalf("receptive = %v", err)
	}
}

func TestUnreadableLabelsFallBackToHumanNamesNotToRawKeys(t *testing.T) {
	s := newService(t, fakeValues{values: map[string]string{}})
	s.declarations = fakeDeclarations{err: errors.New("down")}
	_, err := s.ValuesFor(context.Background(), "ws", "e1", shared.EntryTypeWhatsApp, ref(conversation.AutomationWorkflow, "cobranca"))
	refusal, _ := domain.RefusalOf(err)
	if refusal == nil || refusal.Needs[0].Display() != "Valor" {
		t.Fatalf("refusal = %+v", refusal)
	}
}

func TestAFlowDefaultFillsTheValueWhenTheConversationHasNone(t *testing.T) {
	s := newService(t, fakeValues{values: map[string]string{"valor": "10"}})
	src := s.source.(PublishedSource)
	g := workflow.Graph{Nodes: []workflow.Node{{ID: "n", Type: workflow.NodeTypeActionSendText, Config: map[string]interface{}{"text": "{{campvars.valor}} {{campvars.tom}}"}}}, Variables: []workflow.VariableUse{{Name: "tom", Default: "cordial"}}}
	src.Workflows.(fakeWorkflows)["tom"] = &workflow.Workflow{ID: "tom", WorkspaceID: "ws", Name: "Tom", Graph: g}
	values, err := s.ValuesFor(context.Background(), "ws", "e1", shared.EntryTypeWhatsApp, ref(conversation.AutomationWorkflow, "tom"))
	if err != nil || values["tom"] != "cordial" || values["valor"] != "10" {
		t.Fatalf("values = %v, err = %v", values, err)
	}
}

func TestAdmitReceptiveRefusesAnEnabledAutomationThatNeedsVariables(t *testing.T) {
	s := newService(t, fakeValues{})
	cases := []struct {
		name   string
		config conversation.ChannelAutomation
		want   error
		names  []string
	}{
		{name: "nothing enabled", config: conversation.ChannelAutomation{AgentID: id("escola")}},
		{name: "an agent without variables", config: conversation.ChannelAutomation{AgentID: id("livre"), EnableAgentResponses: true}},
		{name: "an agent that needs a variable", config: conversation.ChannelAutomation{AgentID: id("escola"), EnableAgentResponses: true}, want: domain.ErrReceptiveUnmet, names: []string{"turma"}},
		{name: "a workflow that needs campvars", config: conversation.ChannelAutomation{WorkflowID: id("cobranca"), EnableWorkflow: true}, want: domain.ErrReceptiveUnmet, names: []string{"valor"}},
		{name: "an automation that no longer exists", config: conversation.ChannelAutomation{AgentID: id("sumiu"), EnableAgentResponses: true}, want: domain.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s.AdmitReceptive("ws", domain.SlotInstagram, tc.config)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.names == nil {
				return
			}
			refusal, ok := domain.RefusalOf(err)
			if !ok || !reflect.DeepEqual(refusal.Needs.Names(), tc.names) || refusal.Automation.Name == "" {
				t.Fatalf("refusal = %+v", refusal)
			}
		})
	}
}

func TestConversationMissingNamesWhatTheConversationLacks(t *testing.T) {
	cases := []struct {
		name   string
		values fakeValues
		target conversation.Automation
		want   error
		names  []string
	}{
		{name: "the conversation has the value", values: fakeValues{values: map[string]string{"turma": "A"}}, target: ref(conversation.AutomationAgent, "escola")},
		{name: "the conversation lacks the value", values: fakeValues{values: map[string]string{"lead_name": "Ana"}}, target: ref(conversation.AutomationAgent, "escola"), want: domain.ErrConversationMissing, names: []string{"turma"}},
		{name: "an automation without needs runs on whatever values can be read", values: fakeValues{err: errors.New("down")}, target: ref(conversation.AutomationAgent, "livre")},
		{name: "unreadable values fail closed", values: fakeValues{err: errors.New("down")}, target: ref(conversation.AutomationAgent, "escola"), want: domain.ErrUnreadable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := newService(t, tc.values).ConversationMissing(context.Background(), "ws", "e1", shared.EntryTypeInstagram, tc.target)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.names != nil {
				refusal, _ := domain.RefusalOf(err)
				if !reflect.DeepEqual(refusal.Needs.Names(), tc.names) {
					t.Fatalf("missing = %v", refusal.Needs.Names())
				}
			}
		})
	}
}

func TestValuesForReturnsWhatAnAutomationReadsOrRefuses(t *testing.T) {
	s := newService(t, fakeValues{values: map[string]string{"turma": "A", "lead_name": "Ana"}})
	got, err := s.ValuesFor(context.Background(), "ws", "e1", shared.EntryTypeWhatsApp, ref(conversation.AutomationAgent, "escola"))
	if err != nil || got["turma"] != "A" {
		t.Fatalf("values = %v, %v", got, err)
	}
	if _, err := s.ValuesFor(context.Background(), "ws", "e1", shared.EntryTypeWhatsApp, ref(conversation.AutomationWorkflow, "cobranca")); !errors.Is(err, domain.ErrConversationMissing) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewServiceRefusesMissingDependencies(t *testing.T) {
	if _, err := NewService(ServiceDeps{}); err == nil {
		t.Fatal("expected a refusal")
	}
}
