package automationneeds

import (
	"errors"
	"reflect"
	"testing"

	"vozko/domain/workflow"
)

func TestOfNodeNamesOnlyWhatThatNodeReads(t *testing.T) {
	draft := &workflow.Workflow{ID: "rascunho", WorkspaceID: "ws", Name: "Rascunho", Graph: nodes(
		textNode("linha", "{{sys.date}} {{campvars.matricula}} {{ campvars.valor_pendencia }} {{campvars.lead_name}}"),
		aiNode("ia", "escola"),
		subFlowNode("sub", "cobranca"),
		textNode("outro", "{{campvars.nome_cliente_rascunho}}"),
	)}
	cases := []struct {
		name string
		node string
		want []string
	}{
		{"a step reads its own campvars, never system keys", "linha", []string{"matricula", "valor_pendencia"}},
		{"an AI node needs what its agent needs", "ia", []string{"escola", "turma"}},
		{"a sub-flow node needs what the sub-flow needs", "sub", []string{"nome_cliente", "valor"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := OfNode(source(), "ws", draft, tc.node)
			if err != nil {
				t.Fatalf("OfNode: %v", err)
			}
			if names := got.Names(); !reflect.DeepEqual(names, tc.want) {
				t.Fatalf("names = %v, want %v", names, tc.want)
			}
		})
	}
}

func TestOfNodeCreditsTheWorkflowThatHoldsTheNode(t *testing.T) {
	draft := &workflow.Workflow{ID: "rascunho", WorkspaceID: "ws", Name: "Rascunho", Graph: nodes(textNode("linha", "{{campvars.matricula}}"), aiNode("ia", "escola"))}
	own, err := OfNode(source(), "ws", draft, "linha")
	if err != nil {
		t.Fatal(err)
	}
	if owner := own.Find("matricula").Owner(); owner.Automation != workflowRef("rascunho") || owner.Name != "Rascunho" {
		t.Fatalf("owner = %+v, want the workflow", owner)
	}
	agentNeeds, err := OfNode(source(), "ws", draft, "ia")
	if err != nil {
		t.Fatal(err)
	}
	if owner := agentNeeds.Find("escola").Owner(); owner.Automation != agentRef("escola") {
		t.Fatalf("owner = %+v, want the agent", owner)
	}
}

func TestOfNodeFailsClosed(t *testing.T) {
	draft := &workflow.Workflow{ID: "rascunho", WorkspaceID: "ws", Name: "Rascunho", Graph: nodes(aiNode("ia", "sumiu"))}
	if _, err := OfNode(source(), "ws", draft, "ia"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound for an agent that no longer exists", err)
	}
	if _, err := OfNode(source(), "ws", draft, "nenhum"); !errors.Is(err, workflow.ErrNodeNotFound) {
		t.Fatalf("err = %v, want ErrNodeNotFound", err)
	}
	if _, err := OfNode(source(), "ws", nil, "ia"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound without a workflow", err)
	}
}
