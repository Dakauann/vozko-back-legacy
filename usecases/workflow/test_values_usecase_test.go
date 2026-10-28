package workflow_usecase

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	"vozko/domain/conversationvars"
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

type rulesStub struct {
	rejected []string
	err      error
}

func (r rulesStub) Rejected(context.Context, string, map[string]string) ([]string, error) {
	return r.rejected, r.err
}

func (rulesStub) Label(_ context.Context, _ string, needs automationneeds.Needs) automationneeds.Needs {
	out := append(automationneeds.Needs(nil), needs...)
	for i := range out {
		out[i].Label = "Rótulo " + out[i].Name
	}
	return out
}

func newTestValues(t *testing.T, store workflow.TestValuesRepository) *TestValuesUseCase {
	t.Helper()
	uc, err := NewTestValuesUseCase(store, payingNeeds(), rulesStub{})
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
	if _, err := NewTestValuesUseCase(nil, payingNeeds(), rulesStub{}); err == nil {
		t.Fatal("the use case needs a store")
	}
}

func TestTestValuesMustFitTheDeclaredTypeAndTheFormNamesVariablesByLabel(t *testing.T) {
	store := newMemoryTestValues()
	uc, err := NewTestValuesUseCase(store, payingNeeds(), rulesStub{rejected: []string{"matricula"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = uc.Save(payingWorkflow(), "u1", map[string]string{"matricula": "abc"})
	var named *conversationvars.VariablesError
	if !errors.Is(err, workflow.ErrTestValueTypeInvalid) || !errors.As(err, &named) || named.Labels["matricula"] != "Rótulo matricula" {
		t.Fatalf("err = %v", err)
	}
	if len(store.rows) != 0 {
		t.Fatalf("stored = %v", store.rows)
	}
	form, err := newTestValues(t, store).Form(payingWorkflow(), "u1")
	if err != nil || form.Needs[0].Label != "Rótulo matricula" {
		t.Fatalf("form = %+v, %v", form, err)
	}
}

func TestTestValuesFailClosedWhenTheTypesCannotBeRead(t *testing.T) {
	uc, _ := NewTestValuesUseCase(newMemoryTestValues(), payingNeeds(), rulesStub{err: errors.New("down")})
	if _, err := uc.Save(payingWorkflow(), "u1", map[string]string{"matricula": "1"}); !errors.Is(err, workflow.ErrTestValuesUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestTheTestRunTakesTheFlowDefaults(t *testing.T) {
	store := newMemoryTestValues()
	wf := payingWorkflow()
	wf.Graph.Nodes = append(wf.Graph.Nodes, workflow.Node{ID: "n9", Type: workflow.NodeTypeActionSendText, Config: map[string]interface{}{"text": "{{campvars.tom}}"}})
	wf.Graph.Variables = []workflow.VariableUse{{Name: "tom", Default: "cordial"}}
	store.rows["ws1|wf-pay|u1"] = map[string]string{"matricula": "1", "valor_pendencia": "2", "nome_cliente": "Ana"}
	values, err := newTestValues(t, store).RunValues(wf, "u1")
	if err != nil || values["tom"] != "cordial" {
		t.Fatalf("values = %v, err = %v", values, err)
	}
}
