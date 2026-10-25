package agent_usecase

import (
	"errors"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	toolsdomain "vozko/domain/tools"
)

type boundRegistry struct{ toolsdomain.Service }

func (boundRegistry) Definitions() []toolsdomain.Definition {
	return []toolsdomain.Definition{
		{Name: "http_request", Instances: toolsdomain.Many()},
		{
			Name:           "run_workflow",
			Instances:      toolsdomain.PerTarget("workflow_id"),
			ConfigSchema:   map[string]toolsdomain.ConfigParameter{"workflow_id": {Type: "string", Required: true}},
			RequiredConfig: []string{"workflow_id"},
		},
	}
}

func workflowBinding(id string) agent.ToolBinding {
	return agent.ToolBinding{Name: "run_workflow", Config: map[string]interface{}{"workflow_id": id}}
}

func TestAnAgentKeepsEveryWorkflowAndHTTPInstanceInRequestOrder(t *testing.T) {
	saved, err := resolveInternalToolSelection(boundRegistry{}, []agent.ToolBinding{
		workflowBinding("wf-b"), {Name: "http_request", Function: "pedido"}, workflowBinding("wf-a"), {Name: "http_request", Function: "ticket"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 4 || saved[0].Function != "run_workflow_wf-b" || saved[1].Function != "pedido" || saved[2].Function != "run_workflow_wf-a" || saved[3].Function != "ticket" {
		t.Fatalf("saved = %+v", saved)
	}
	for _, b := range saved {
		if b.ID == "" {
			t.Fatalf("every instance gets an id: %+v", b)
		}
	}
}

func TestTheSameWorkflowTwiceIsRefusedInsteadOfDropped(t *testing.T) {
	_, err := resolveInternalToolSelection(boundRegistry{}, []agent.ToolBinding{workflowBinding("wf-a"), workflowBinding("wf-a")}, nil)
	if !errors.Is(err, agenttool.ErrTargetTaken) {
		t.Fatalf("err = %v", err)
	}
}

func (boundRegistry) Handler(string) (toolsdomain.Handler, bool) { return nil, false }
