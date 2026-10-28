package workflow_usecase

import (
	"testing"

	"vozko/domain/workflow"
)

func mustTestValues(t *testing.T, store *memoryTestValues) *TestValuesUseCase {
	t.Helper()
	uc, err := NewTestValuesUseCase(store, needsSourceStub{})
	if err != nil {
		t.Fatal(err)
	}
	return uc
}

func campaignVarsWorkflow() *workflow.Workflow {
	return &workflow.Workflow{
		ID: "wf-cv", WorkspaceID: "ws1", Name: "Cobrança", Type: workflow.WorkflowTypeMessages, Status: workflow.WorkflowStatusDraft,
		Graph: workflow.Graph{
			Nodes: []workflow.Node{
				{ID: "hook", Type: workflow.NodeTypeTriggerWebhook},
				{ID: "copia", Type: workflow.NodeTypeActionSetVariable, Config: map[string]interface{}{"variable": "resumo", "value": "{{campvars.matricula}}|{{campvars.lead_name}}"}},
				{ID: "end", Type: workflow.NodeTypeEnd},
			},
			Edges: []workflow.Edge{{Source: "hook", Target: "copia"}, {Source: "copia", Target: "end"}},
		},
	}
}

func TestASimulationDoesNotStartUntilTheTestDataTheWorkflowNeedsIsFilled(t *testing.T) {
	sim := startDebugSimulation(t, debugSimOptions{workflow: campaignVarsWorkflow(), awaitStart: true})
	readUntil(t, sim.conn, "sim_ready")

	sim.send(t, "start", map[string]interface{}{"triggerNodeId": "hook", "input": map[string]interface{}{}})
	refused := decode[refusedFrame](t, firstFrame(t, sim, "command_refused"))
	if refused.Command != "start" || refused.Code != "automation_test_values_missing" || refused.Error == "" {
		t.Fatalf("command_refused = %+v", refused)
	}
}

func TestASimulationSeedsTheSavedTestDataWhereTheRuntimeGatePutsCampaignValues(t *testing.T) {
	store := newMemoryTestValues()
	store.rows["ws1|wf-cv|u-1"] = map[string]string{"matricula": "123"}
	sim := startDebugSimulation(t, debugSimOptions{workflow: campaignVarsWorkflow(), awaitStart: true, testValues: store})
	readUntil(t, sim.conn, "sim_ready")

	sim.send(t, "start", map[string]interface{}{"triggerNodeId": "hook", "input": map[string]interface{}{}})
	completed, _ := readUntil(t, sim.conn, "run_completed")
	vars := decode[statePayload](t, completed).Vars
	if vars["resumo"] != "123|"+simLeadName {
		t.Fatalf("resumo = %v, want the saved value and the platform lead name", vars["resumo"])
	}
}
