package conversation

import "testing"

func TestResponsibleWorkflow(t *testing.T) {
	toWorkflow := &Automation{Kind: AutomationWorkflow, ID: "wf-delegate"}
	toAgent := &Automation{Kind: AutomationAgent, ID: "agent-delegate"}
	cases := []struct {
		name    string
		profile AutomationProfile
		wantID  string
		wantOK  bool
	}{
		{"the container's enabled workflow is responsible", AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true}, "wf-1", true},
		{"a workflow switched off on the container is not responsible", AutomationProfile{WorkflowID: "wf-1"}, "", false},
		{"workflows on with no workflow chosen names nobody", AutomationProfile{WorkflowEnabled: true}, "", false},
		{"blank workflow id names nobody", AutomationProfile{WorkflowID: "  ", WorkflowEnabled: true}, "", false},
		{"an agent container has no responsible workflow", AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true}, "", false},
		{"a delegation to a workflow wins over the container", AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true, Delegate: toWorkflow}, "wf-delegate", true},
		{"a delegation to an agent leaves no workflow responsible", AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true, Delegate: toAgent}, "", false},
		{"a paused conversation keeps its responsible workflow", AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true, AutomationEnabled: boolPtr(false)}, "wf-1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, ok := tc.profile.ResponsibleWorkflow()
			if id != tc.wantID || ok != tc.wantOK {
				t.Fatalf("ResponsibleWorkflow() = %q, %v; want %q, %v", id, ok, tc.wantID, tc.wantOK)
			}
		})
	}
}
