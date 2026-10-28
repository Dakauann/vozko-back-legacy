package automationneeds

import (
	"context"
	"errors"
	"reflect"
	"testing"

	domain "vozko/domain/automationneeds"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

func flowDraft() *workflow.Workflow {
	return &workflow.Workflow{ID: "matriculas", WorkspaceID: "ws", Name: "Matrículas", Type: workflow.WorkflowTypeMessages, Graph: workflow.Graph{
		Nodes: []workflow.Node{
			{ID: "t", Type: workflow.NodeTypeTriggerMessageReceived},
			{ID: "boas", Type: workflow.NodeTypeActionSendText, Config: map[string]interface{}{"text": "Oi {{campvars.matricula}} {{campvars.turno}}", "display_name": "Boas-vindas"}},
			{ID: "ia", Type: workflow.NodeTypeActionAIAgent, Config: map[string]interface{}{"source": "agent", "agent_id": "escola"}},
		},
		Variables: []workflow.VariableUse{{Name: "turno", Default: "Manhã"}, {Name: "reserva"}},
	}}
}

func TestTheFlowPanelListsEveryVariableWithItsUsesFillersAndDeclaration(t *testing.T) {
	q := newQueriesWith(t, &memRecords{}, &memValueStore{}, true, func(d *QueriesDeps) {
		d.Workflows = fakeWorkflows{"matriculas": flowDraft()}
	})
	q.d.Service.declarations = fakeDeclarations{rows: []variabledecl.Declaration{{Name: "matricula", Label: "Matrícula", Description: "Número do aluno", Type: variabledecl.Type{Kind: variabledecl.KindNumber}}}}
	rows, err := q.FlowVariables(context.Background(), "ws", "matriculas")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Need.Name)
	}
	if !reflect.DeepEqual(names, []string{"matricula", "turma", "turno", "reserva"}) {
		t.Fatalf("names = %v", names)
	}
	matricula, turma, turno, reserva := rows[0], rows[1], rows[2], rows[3]
	if !matricula.Required || !matricula.Need.Declared || matricula.Need.Label != "Matrícula" || matricula.Need.Shape != variabledecl.ShapeNumber || !reflect.DeepEqual(matricula.Nodes, []NodeRef{{ID: "boas", Name: "Boas-vindas"}}) {
		t.Fatalf("matricula = %+v", matricula)
	}
	if !turma.Required || turma.Need.Declared || turma.Need.Owner().Name != "Escola" || len(turma.Nodes) != 0 {
		t.Fatalf("turma = %+v", turma)
	}
	if turno.Required || !turno.InFlow || turno.Need.Default != "Manhã" || !hasFiller(turno, FilledByDefault, "Manhã") {
		t.Fatalf("turno = %+v", turno)
	}
	if reserva.Used || !reserva.InFlow || !reserva.Required {
		t.Fatalf("reserva = %+v", reserva)
	}
	for _, kind := range []FillerKind{FilledByCampaign, FilledByHandOff, FilledByTransfer} {
		if !hasFiller(matricula, kind, "") {
			t.Fatalf("matricula fillers = %+v, missing %s", matricula.FilledBy, kind)
		}
	}
}

func TestAVoiceFlowIsFilledByNobody(t *testing.T) {
	draft := flowDraft()
	draft.Type = workflow.WorkflowTypeVoice
	q := newQueriesWith(t, &memRecords{}, &memValueStore{}, true, func(d *QueriesDeps) { d.Workflows = fakeWorkflows{"matriculas": draft} })
	rows, err := q.FlowVariables(context.Background(), "ws", "matriculas")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows[0].FilledBy) != 0 {
		t.Fatalf("fillers = %+v", rows[0].FilledBy)
	}
}

func TestTheFlowPanelFailsClosed(t *testing.T) {
	q := newQueriesWith(t, &memRecords{}, &memValueStore{}, true, func(d *QueriesDeps) { d.Workflows = fakeWorkflows{"matriculas": flowDraft()} })
	if _, err := q.FlowVariables(context.Background(), "outro", "matriculas"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other workspace = %v", err)
	}
	q.d.Service.declarations = fakeDeclarations{err: errors.New("down")}
	if _, err := q.FlowVariables(context.Background(), "ws", "matriculas"); !errors.Is(err, domain.ErrUnreadable) {
		t.Fatalf("unreadable declarations = %v", err)
	}
}

func hasFiller(row FlowVariable, kind FillerKind, value string) bool {
	for _, f := range row.FilledBy {
		if f.Kind == kind && f.Value == value {
			return true
		}
	}
	return false
}
