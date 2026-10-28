package workflow_usecase

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"vozko/domain/shared"
	"vozko/domain/workflow"
)

type strictRowExecutor struct {
	ran  int
	seen map[string]interface{}
}

func (e *strictRowExecutor) Definition() workflow.NodeDefinition {
	return workflow.NodeDefinition{Type: workflow.NodeTypeActionSetVariable, Label: "Linha"}
}

func (e *strictRowExecutor) Execute(ctx *workflow.NodeContext) (*workflow.NodeResult, error) {
	e.ran++
	values, _ := ctx.Node.Config["values"].(map[string]interface{})
	e.seen = map[string]interface{}{}
	var missing []string
	for key, raw := range values {
		text, unresolved := workflow.InterpolateStrict(workflow.TemplateText(raw), ctx.State, nil)
		missing = append(missing, unresolved...)
		e.seen[key] = text
	}
	if len(missing) > 0 {
		return &workflow.NodeResult{Error: "variáveis sem valor: " + strings.Join(missing, ", ")}, nil
	}
	return &workflow.NodeResult{Output: map[string]interface{}{"row_id": "row-1"}}, nil
}

func sheetRowWorkflow() *workflow.Workflow {
	return createTestWorkflow("wf-pay", "ws1", []workflow.Node{
		{ID: "t", Type: workflow.NodeTypeTriggerFirstMessage},
		{ID: "n1", Type: workflow.NodeTypeActionAIAgent, Config: map[string]interface{}{"source": "prompt", "instructions": "resuma", "response_variable": "resumo"}},
		{ID: "n8", Type: workflow.NodeTypeActionSetVariable, Config: map[string]interface{}{"values": map[string]interface{}{
			"data": "{{sys.date}}", "matricula": "{{campvars.matricula}}", "valor": "{{campvars.valor_pendencia}}", "forma": "PIX", "obs": "{{node.n1.observacoes}}",
		}}},
	}, []workflow.Edge{{Source: "t", Target: "n1"}, {Source: "n1", Target: "n8"}})
}

func recordedSheetRun(campvars map[string]interface{}) *workflow.RunDetail {
	state := workflow.NewRunState()
	state.Set(workflow.DataKeyCampaignVars, campvars)
	state.Set("message", "quero pagar")
	return &workflow.RunDetail{
		Run: &workflow.WorkflowRun{ID: "run-7", WorkflowID: "wf-pay", WorkspaceID: "ws1", EntryID: "conv-1", EntryType: "whatsapp", State: state},
		Logs: []*workflow.WorkflowRunLog{
			{ID: "s-t", NodeID: "t", NodeType: workflow.NodeTypeTriggerFirstMessage, Status: workflow.LogStatusExecuted, Output: map[string]interface{}{"message": "quero pagar"}},
			{ID: "s-n1", NodeID: "n1", NodeType: workflow.NodeTypeActionAIAgent, Status: workflow.LogStatusExecuted, Output: map[string]interface{}{"observacoes": "cliente pediu PIX"}},
			{ID: "s-n8", NodeID: "n8", NodeType: workflow.NodeTypeActionSetVariable, Status: workflow.LogStatusFailed},
		},
	}
}

type seedFixture struct {
	uc       TestNodeUseCase
	runs     *runDetailStub
	row      *strictRowExecutor
	ai       *countingExecutor
	values   *memoryTestValues
	viewerID string
}

type countingExecutor struct{ ran int }

func (e *countingExecutor) Definition() workflow.NodeDefinition {
	return workflow.NodeDefinition{Type: workflow.NodeTypeActionAIAgent, Label: "IA"}
}

func (e *countingExecutor) Execute(*workflow.NodeContext) (*workflow.NodeResult, error) {
	e.ran++
	return &workflow.NodeResult{Output: map[string]interface{}{"observacoes": "da IA de agora"}}, nil
}

func newSeedFixture(t *testing.T, detail *workflow.RunDetail) *seedFixture {
	t.Helper()
	repo := NewMockWorkflowRepository()
	if err := repo.Create(sheetRowWorkflow()); err != nil {
		t.Fatal(err)
	}
	f := &seedFixture{runs: &runDetailStub{detail: detail}, row: &strictRowExecutor{}, ai: &countingExecutor{}, values: newMemoryTestValues(), viewerID: "u1"}
	registry := NewNodeExecutorRegistry()
	registry.Register(workflow.NodeTypeActionSetVariable, f.row)
	registry.Register(workflow.NodeTypeActionAIAgent, f.ai)
	saved, err := NewTestValuesUseCase(f.values, needsSourceStub{})
	if err != nil {
		t.Fatal(err)
	}
	f.uc = NewTestNodeUseCase(TestNodeDeps{
		WorkflowRepo: repo, Steps: registry, Executors: fixedExecutors(registry),
		Runs: f.runs, Needs: needsSourceStub{}, TestValues: saved,
		ViewerOf: func(context.Context) (shared.Person, bool) {
			return shared.Person{UserID: f.viewerID}, f.viewerID != ""
		},
	})
	return f
}

func mockFieldsByKey(analysis *AnalyzeNodeOutput) map[string]MockFieldSpec {
	out := map[string]MockFieldSpec{}
	for _, field := range analysis.ToUIAnalysis().MockFields {
		out[field.Key] = field
	}
	return out
}

func TestThePlainNodeTestAsksForEveryCampaignVariableTheNodeReads(t *testing.T) {
	f := newSeedFixture(t, nil)
	f.values.rows["ws1|wf-pay|u1"] = map[string]string{"matricula": "123"}

	analysis, err := f.uc.Analyze(context.Background(), AnalyzeNodeInput{WorkflowID: "wf-pay", WorkspaceID: "ws1", NodeID: "n8"})
	if err != nil {
		t.Fatal(err)
	}
	fields := mockFieldsByKey(analysis)
	for _, key := range []string{"_node_n1_observacoes", "campvars.matricula", "campvars.valor_pendencia"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("mock fields = %v, missing %s", fields, key)
		}
	}
	matricula := fields["campvars.matricula"]
	if matricula.Source != string(workflow.DependencySourceAutomationVar) || matricula.DisplayName != "campvars.matricula" || matricula.NeededBy != "Test Workflow" || matricula.Optional {
		t.Fatalf("matricula = %+v, want a required automation variable used by the workflow", matricula)
	}
	if matricula.Value != "123" {
		t.Fatalf("matricula value = %v, want the viewer's saved test value", matricula.Value)
	}
	if value := fields["campvars.valor_pendencia"].Value; value != nil {
		t.Fatalf("valor_pendencia = %v, want no default without a saved value", value)
	}
}

func TestTheNodeTestFromARunPrefillsWhatTheRunRecorded(t *testing.T) {
	f := newSeedFixture(t, recordedSheetRun(map[string]interface{}{"matricula": "123", "valor_pendencia": "R$ 10"}))
	f.values.rows["ws1|wf-pay|u1"] = map[string]string{"matricula": "salvo"}

	analysis, err := f.uc.Analyze(context.Background(), AnalyzeNodeInput{WorkflowID: "wf-pay", WorkspaceID: "ws1", NodeID: "n8", FromRun: &TestFromRun{RunID: "run-7", StepID: "s-n8"}})
	if err != nil {
		t.Fatal(err)
	}
	fields := mockFieldsByKey(analysis)
	want := map[string]interface{}{"_node_n1_observacoes": "cliente pediu PIX", "campvars.matricula": "123", "campvars.valor_pendencia": "R$ 10"}
	for key, value := range want {
		if fields[key].Value != value {
			t.Fatalf("%s = %v, want %v from the run", key, fields[key].Value, value)
		}
	}
	if f.runs.viewer.UserID != "u1" || f.runs.pick == nil || !reflect.DeepEqual(*f.runs.pick, workflow.TestSeedPick("s-n8")) {
		t.Fatalf("read as %+v with pick %+v; the seed reads the step and what ran before it as the viewer", f.runs.viewer, f.runs.pick)
	}
}

func TestTheNodeTestFromARunReproducesTheStepWithTheRunsValues(t *testing.T) {
	f := newSeedFixture(t, recordedSheetRun(map[string]interface{}{"matricula": "123", "valor_pendencia": "R$ 10"}))

	result, err := f.uc.Execute(context.Background(), TestNodeInput{WorkflowID: "wf-pay", WorkspaceID: "ws1", NodeID: "n8", FromRun: &TestFromRun{RunID: "run-7", StepID: "s-n8"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("result = %+v, want the step to run with the run's values", result)
	}
	if f.row.seen["matricula"] != "123" || f.row.seen["valor"] != "R$ 10" || f.row.seen["obs"] != "cliente pediu PIX" {
		t.Fatalf("the step saw %v", f.row.seen)
	}
	if f.ai.ran != 0 || len(result.UpstreamSteps) != 0 {
		t.Fatalf("ai ran %d times, upstream %d; the run already holds what came before the step", f.ai.ran, len(result.UpstreamSteps))
	}
}

func TestAnEditedValueWinsOverTheRecordedOne(t *testing.T) {
	f := newSeedFixture(t, recordedSheetRun(map[string]interface{}{"matricula": "123", "valor_pendencia": "R$ 10"}))

	if _, err := f.uc.Execute(context.Background(), TestNodeInput{WorkflowID: "wf-pay", WorkspaceID: "ws1", NodeID: "n8", FromRun: &TestFromRun{RunID: "run-7", StepID: "s-n8"},
		MockedState: map[string]interface{}{"campvars.matricula": "999"}}); err != nil {
		t.Fatal(err)
	}
	if f.row.seen["matricula"] != "999" || f.row.seen["valor"] != "R$ 10" {
		t.Fatalf("the step saw %v", f.row.seen)
	}
}

func TestTheNodeTestFromARunStillFailsClosedOnAValueTheRunNeverHad(t *testing.T) {
	f := newSeedFixture(t, recordedSheetRun(map[string]interface{}{"matricula": "123"}))

	result, err := f.uc.Execute(context.Background(), TestNodeInput{WorkflowID: "wf-pay", WorkspaceID: "ws1", NodeID: "n8", FromRun: &TestFromRun{RunID: "run-7", StepID: "s-n8"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || f.row.ran != 0 || result.ErrorKind != workflow.ErrorKindMissingMocks || !strings.Contains(result.Error, "campvars.valor_pendencia") {
		t.Fatalf("result = %+v, ran %d; a missing value is asked for, never sent blank", result, f.row.ran)
	}
}

func TestThePlainNodeTestRunsTheStepWithTheValuesTyped(t *testing.T) {
	f := newSeedFixture(t, nil)

	result, err := f.uc.Execute(context.Background(), TestNodeInput{WorkflowID: "wf-pay", WorkspaceID: "ws1", NodeID: "n8", MockedState: map[string]interface{}{
		"_node_n1_observacoes": "digitado", "campvars.matricula": "1", "campvars.valor_pendencia": "2",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || f.row.seen["matricula"] != "1" || f.ai.ran != 0 {
		t.Fatalf("result = %+v saw %v ai %d", result, f.row.seen, f.ai.ran)
	}
}

func TestTheNodeTestFromARunIsRefusedWhenTheRunCannotBeUsed(t *testing.T) {
	otherFlow := recordedSheetRun(nil)
	otherFlow.Run.WorkflowID = "wf-other"
	cases := []struct {
		name   string
		detail *workflow.RunDetail
		from   TestFromRun
		viewer string
	}{
		{"a run of another workflow", otherFlow, TestFromRun{RunID: "run-7", StepID: "s-n8"}, "u1"},
		{"a hidden or missing run", nil, TestFromRun{RunID: "run-7", StepID: "s-n8"}, "u1"},
		{"a step of another node", recordedSheetRun(nil), TestFromRun{RunID: "run-7", StepID: "s-n1"}, "u1"},
		{"no viewer to read the run as", recordedSheetRun(nil), TestFromRun{RunID: "run-7", StepID: "s-n8"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSeedFixture(t, tc.detail)
			f.viewerID = tc.viewer
			from := tc.from
			if _, err := f.uc.Analyze(context.Background(), AnalyzeNodeInput{WorkflowID: "wf-pay", WorkspaceID: "ws1", NodeID: "n8", FromRun: &from}); !errors.Is(err, workflow.ErrRunNotFound) {
				t.Fatalf("analyze err = %v, want ErrRunNotFound", err)
			}
			if _, err := f.uc.Execute(context.Background(), TestNodeInput{WorkflowID: "wf-pay", WorkspaceID: "ws1", NodeID: "n8", FromRun: &from}); !errors.Is(err, workflow.ErrRunNotFound) {
				t.Fatalf("execute err = %v, want ErrRunNotFound", err)
			}
		})
	}
}
