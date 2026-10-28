package workflow_usecase

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	"vozko/domain/workflow"
)

type memoryTestValues struct {
	rows    map[string]map[string]string
	failing bool
}

func newMemoryTestValues() *memoryTestValues {
	return &memoryTestValues{rows: map[string]map[string]string{}}
}

func (m *memoryTestValues) key(workspaceID, workflowID, userID string) string {
	return workspaceID + "|" + workflowID + "|" + userID
}

func (m *memoryTestValues) TestValues(workspaceID, workflowID, userID string) (map[string]string, error) {
	if m.failing {
		return nil, errors.New("db down")
	}
	out := map[string]string{}
	for k, v := range m.rows[m.key(workspaceID, workflowID, userID)] {
		out[k] = v
	}
	return out, nil
}

func (m *memoryTestValues) SaveTestValues(workspaceID, workflowID, userID string, values map[string]string) error {
	if m.failing {
		return errors.New("db down")
	}
	m.rows[m.key(workspaceID, workflowID, userID)] = values
	return nil
}

func (m *memoryTestValues) ClearTestValues(workspaceID, workflowID, userID string) error {
	if m.failing {
		return errors.New("db down")
	}
	delete(m.rows, m.key(workspaceID, workflowID, userID))
	return nil
}

type needsSourceStub struct {
	agents    map[string]*agent.Agent
	workflows map[string]*workflow.Workflow
}

func (s needsSourceStub) Agent(id string) (*agent.Agent, error) { return s.agents[id], nil }

func (s needsSourceStub) Workflow(id string) (*workflow.Workflow, error) { return s.workflows[id], nil }

func payingWorkflow() *workflow.Workflow {
	return &workflow.Workflow{ID: "wf-pay", WorkspaceID: "ws1", Name: "Cobrança", Graph: workflow.Graph{Nodes: []workflow.Node{
		{ID: "t", Type: workflow.NodeTypeTriggerFirstMessage},
		{ID: "n1", Type: workflow.NodeTypeActionAIAgent, Config: map[string]interface{}{"source": "agent", "agent_id": "ag1"}},
		{ID: "n8", Type: workflow.NodeTypeActionSendText, Config: map[string]interface{}{"text": "{{campvars.matricula}} {{campvars.valor_pendencia}} {{campvars.lead_name}}"}},
	}}}
}

func payingNeeds() needsSourceStub {
	return needsSourceStub{agents: map[string]*agent.Agent{
		"ag1": {ID: "ag1", WorkspaceID: "ws1", Name: "Atendente", MessagingPrompt: "Olá {{nome_cliente}}", Variables: []agent.AgentVariable{{Name: "nome_cliente", Description: "Nome do cliente"}}},
	}}
}

func newTestValues(t *testing.T, store workflow.TestValuesRepository) *TestValuesUseCase {
	t.Helper()
	uc, err := NewTestValuesUseCase(store, payingNeeds())
	if err != nil {
		t.Fatal(err)
	}
	return uc
}

func TestTheTestFormListsEveryNeedOfTheDraftWithTheViewersSavedValues(t *testing.T) {
	store := newMemoryTestValues()
	store.rows["ws1|wf-pay|u1"] = map[string]string{"matricula": "123", "sumiu": "x"}
	form, err := newTestValues(t, store).Form(payingWorkflow(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if names := form.Needs.Names(); !reflect.DeepEqual(names, []string{"matricula", "nome_cliente", "valor_pendencia"}) {
		t.Fatalf("needs = %v", names)
	}
	if !reflect.DeepEqual(form.Values, map[string]string{"matricula": "123"}) {
		t.Fatalf("values = %v, want only values the draft still needs", form.Values)
	}
	if owner := form.Needs.Find("nome_cliente").Owner(); owner.Name != "Atendente" {
		t.Fatalf("owner = %+v, want the agent that uses it", owner)
	}
}

func TestSavedTestValuesBelongToOneViewerOfOneWorkflow(t *testing.T) {
	store := newMemoryTestValues()
	uc := newTestValues(t, store)
	if _, err := uc.Save(payingWorkflow(), "u1", map[string]string{"matricula": " 123 ", "valor_pendencia": ""}); err != nil {
		t.Fatal(err)
	}
	if got := store.rows["ws1|wf-pay|u1"]; !reflect.DeepEqual(got, map[string]string{"matricula": "123"}) {
		t.Fatalf("saved = %v, want trimmed values without blanks", got)
	}
	other, err := uc.Form(payingWorkflow(), "u2")
	if err != nil || len(other.Values) != 0 {
		t.Fatalf("another viewer sees %v, %v", other, err)
	}
	if err := uc.Clear(payingWorkflow(), "u1"); err != nil {
		t.Fatal(err)
	}
	if _, kept := store.rows["ws1|wf-pay|u1"]; kept {
		t.Fatal("clear keeps the values")
	}
}

func TestSavingRefusesValuesTheTestDoesNotUse(t *testing.T) {
	uc := newTestValues(t, newMemoryTestValues())
	cases := []struct {
		name   string
		values map[string]string
		want   error
	}{
		{"a variable no step reads", map[string]string{"cpf": "1"}, workflow.ErrTestValueNotNeeded},
		{"a system key the platform fills", map[string]string{"lead_name": "Ana"}, workflow.ErrTestValueNotNeeded},
		{"a value past the limit", map[string]string{"matricula": strings.Repeat("a", 2001)}, workflow.ErrTestValueInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := uc.Save(payingWorkflow(), "u1", tc.values); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRunValuesFailClosedUntilEveryNeedHasAValue(t *testing.T) {
	store := newMemoryTestValues()
	store.rows["ws1|wf-pay|u1"] = map[string]string{"matricula": "123"}
	uc := newTestValues(t, store)
	_, err := uc.RunValues(payingWorkflow(), "u1")
	refusal, ok := automationneeds.RefusalOf(err)
	if !ok || !errors.Is(err, automationneeds.ErrTestValuesMissing) {
		t.Fatalf("err = %v, want the test values refusal", err)
	}
	if names := refusal.Needs.Names(); !reflect.DeepEqual(names, []string{"nome_cliente", "valor_pendencia"}) {
		t.Fatalf("missing = %v", names)
	}
	if refusal.Automation.Name != "Cobrança" {
		t.Fatalf("automation = %+v", refusal.Automation)
	}

	store.rows["ws1|wf-pay|u1"] = map[string]string{"matricula": "123", "valor_pendencia": "R$ 10", "nome_cliente": "Ana"}
	values, err := uc.RunValues(payingWorkflow(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"matricula": "123", "valor_pendencia": "R$ 10", "nome_cliente": "Ana", "lead_name": simLeadName, "lead_number": simContactNumber, "campaign_name": ""}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("values = %v\nwant %v", values, want)
	}
}

func TestTestValuesFailClosedWhenTheyCannotBeRead(t *testing.T) {
	store := newMemoryTestValues()
	store.failing = true
	uc := newTestValues(t, store)
	if _, err := uc.Form(payingWorkflow(), "u1"); !errors.Is(err, workflow.ErrTestValuesUnavailable) {
		t.Fatalf("form err = %v", err)
	}
	if _, err := uc.RunValues(payingWorkflow(), "u1"); !errors.Is(err, workflow.ErrTestValuesUnavailable) {
		t.Fatalf("run err = %v", err)
	}
	broken := payingWorkflow()
	broken.Graph.Nodes[1].Config["agent_id"] = "sumiu"
	if _, err := newTestValues(t, newMemoryTestValues()).Form(broken, "u1"); !errors.Is(err, automationneeds.ErrNotFound) {
		t.Fatalf("needs err = %v", err)
	}
	if _, err := NewTestValuesUseCase(nil, payingNeeds()); err == nil {
		t.Fatal("the use case needs a store")
	}
}
