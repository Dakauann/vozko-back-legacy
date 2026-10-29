package automationneeds

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"vozko/domain/agent"
	domain "vozko/domain/automationneeds"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

func agentCandidate() *agent.Agent {
	return &agent.Agent{
		ID: "matriculas", WorkspaceID: "ws", Name: "Matrículas",
		MessagingPrompt: "Oi {{lead_name}}, matrícula {{matricula}}, turno {{turno}}, curso {{curso}}",
		Variables: []agent.AgentVariable{
			{Name: "turno", DefaultValue: "Manhã"},
			{Name: "curso", Description: "Curso escolhido"},
			{Name: "reserva"},
			{Name: "lead_number", DefaultValue: "0"},
		},
		InternalTools: []agent.ToolBinding{{Name: workflow.AgentRunWorkflowTool, Config: map[string]interface{}{"workflow_id": "cobranca"}}},
	}
}

func agentRowsByName(rows []AgentVariable) map[string]AgentVariable {
	out := map[string]AgentVariable{}
	for _, row := range rows {
		out[row.Need.Name] = row
	}
	return out
}

func TestTheAgentPanelListsEveryVariableTheCandidateUsesWithItsDeclaration(t *testing.T) {
	q := newQueries(t, &memRecords{}, &memValueStore{}, true)
	q.d.Service.declarations = fakeDeclarations{rows: []variabledecl.Declaration{
		{Name: "matricula", Label: "Matrícula", Description: "Número do aluno", Type: variabledecl.Type{Kind: variabledecl.KindNumber}},
		{Name: "turno", Label: "Turno", Type: variabledecl.Type{Kind: variabledecl.KindChoice, Options: []string{"Manhã", "Tarde"}}},
	}}
	rows, err := q.AgentVariables(context.Background(), "ws", agentCandidate())
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Need.Name)
	}
	if !reflect.DeepEqual(names, []string{"curso", "matricula", "reserva", "valor", "turno", "lead_number"}) {
		t.Fatalf("names = %v", names)
	}
	got := agentRowsByName(rows)
	matricula := got["matricula"]
	if !matricula.Required || matricula.InAgent || !matricula.InPrompt || !matricula.Used || !matricula.Need.Declared || matricula.Need.Shape != variabledecl.ShapeNumber || matricula.Need.Label != "Matrícula" || matricula.Problem != nil {
		t.Fatalf("matricula = %+v", matricula)
	}
	if curso := got["curso"]; !curso.Required || !curso.InAgent || !curso.InPrompt || curso.Need.Description != "Curso escolhido" || curso.Need.Declared {
		t.Fatalf("curso = %+v", curso)
	}
	if turno := got["turno"]; turno.Required || !turno.InAgent || turno.Need.Default != "Manhã" || turno.Need.Shape != variabledecl.ShapeChoice || !hasAgentFiller(turno, FilledByDefault, "Manhã") {
		t.Fatalf("turno = %+v", turno)
	}
	if reserva := got["reserva"]; !reserva.Required || !reserva.InAgent || reserva.InPrompt || !reserva.Used {
		t.Fatalf("reserva = %+v", reserva)
	}
	if valor := got["valor"]; !valor.Required || valor.InAgent || valor.InPrompt || valor.Need.Owner().Name != "Cobrança" {
		t.Fatalf("valor = %+v", valor)
	}
	if platform := got["lead_number"]; platform.Used || !platform.InAgent || !errors.Is(platform.Problem, agent.ErrAgentVariableNameReserved) || platform.Need.Label != "Número do contato" {
		t.Fatalf("lead_number = %+v", platform)
	}
	if _, listed := got["lead_name"]; listed {
		t.Fatal("platform data the prompt reads is never a row")
	}
	for _, kind := range []FillerKind{FilledByCampaign, FilledByHandOff, FilledByTransfer} {
		if !hasAgentFiller(matricula, kind, "") {
			t.Fatalf("matricula fillers = %+v, missing %s", matricula.FilledBy, kind)
		}
	}
}

func TestTheAgentPanelReadsANewAgentWithoutAnID(t *testing.T) {
	q := newQueries(t, &memRecords{}, &memValueStore{}, true)
	rows, err := q.AgentVariables(context.Background(), "ws", &agent.Agent{WorkspaceID: "ws", MessagingPrompt: "Oi {{apelido}}"})
	if err != nil || len(rows) != 1 || rows[0].Need.Name != "apelido" || !rows[0].Required || rows[0].InAgent {
		t.Fatalf("rows = %+v, err = %v", rows, err)
	}
}

func TestTheAgentPanelFailsClosed(t *testing.T) {
	q := newQueries(t, &memRecords{}, &memValueStore{}, true)
	if _, err := q.AgentVariables(context.Background(), "outro", agentCandidate()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other workspace = %v", err)
	}
	if _, err := q.AgentVariables(context.Background(), "ws", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no candidate = %v", err)
	}
	q.d.Service.declarations = fakeDeclarations{err: errors.New("down")}
	if _, err := q.AgentVariables(context.Background(), "ws", agentCandidate()); !errors.Is(err, domain.ErrUnreadable) {
		t.Fatalf("unreadable declarations = %v", err)
	}
}

func hasAgentFiller(row AgentVariable, kind FillerKind, value string) bool {
	for _, f := range row.FilledBy {
		if f.Kind == kind && f.Value == value {
			return true
		}
	}
	return false
}
