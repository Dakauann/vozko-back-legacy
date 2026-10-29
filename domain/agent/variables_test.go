package agent

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func validAgent(vars ...AgentVariable) *Agent {
	return &Agent{Name: "Matrículas", MessagingPrompt: "Olá", Provider: AgentProviderAI, MessagingModel: "m", InternalTools: []ToolBinding{}, Variables: vars}
}

func TestValidateRefusesAgentVariablesByTheSharedNameRule(t *testing.T) {
	cases := []struct {
		name string
		vars []AgentVariable
		want error
	}{
		{"blank name", []AgentVariable{{Name: " "}}, ErrAgentVariableNameInvalid},
		{"spaces", []AgentVariable{{Name: "minha var"}}, ErrAgentVariableNameInvalid},
		{"accent", []AgentVariable{{Name: "matrícula"}}, ErrAgentVariableNameInvalid},
		{"too long", []AgentVariable{{Name: strings.Repeat("a", 65)}}, ErrAgentVariableNameInvalid},
		{"platform data", []AgentVariable{{Name: "lead_name", DefaultValue: "cliente"}}, ErrAgentVariableNameReserved},
		{"repeated", []AgentVariable{{Name: "turno"}, {Name: " turno ", DefaultValue: "Manhã"}}, ErrAgentVariableDuplicate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := validAgent(tc.vars...)
			a.Normalize()
			if err := a.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNormalizeKeepsEveryAgentVariableSoValidateSeesRepeats(t *testing.T) {
	a := validAgent(AgentVariable{Name: " turno ", Description: " Período ", DefaultValue: " Manhã "}, AgentVariable{Name: "Turno"})
	a.Normalize()
	if len(a.Variables) != 2 || a.Variables[0] != (AgentVariable{Name: "turno", Description: "Período", DefaultValue: "Manhã"}) {
		t.Fatalf("variables = %+v", a.Variables)
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("names differing only in case are two variables, like in the catalog: %v", err)
	}
}

func TestValidateRefusesMoreThanFiftyAgentVariables(t *testing.T) {
	vars := make([]AgentVariable, 51)
	for i := range vars {
		vars[i] = AgentVariable{Name: fmt.Sprintf("v%d", i)}
	}
	if err := validAgent(vars...).Validate(); !errors.Is(err, ErrAgentVariableTooMany) {
		t.Fatalf("err = %v", err)
	}
	if err := validAgent(vars[:50]...).Validate(); err != nil {
		t.Fatalf("fifty variables are allowed: %v", err)
	}
}

func TestVariableProblemsNamesEachBadAgentVariable(t *testing.T) {
	problems := VariableProblems([]AgentVariable{{Name: "turno"}, {Name: "lead_number"}, {Name: "turno"}, {Name: "ok"}})
	if len(problems) != 2 || problems["lead_number"] == nil || !errors.Is(problems["lead_number"], ErrAgentVariableNameReserved) || !errors.Is(problems["turno"], ErrAgentVariableDuplicate) {
		t.Fatalf("problems = %v", problems)
	}
	if VariableProblems([]AgentVariable{{Name: "a"}}) != nil {
		t.Fatal("a clean list has problems")
	}
}

func TestWithoutPlatformVariablesDropsOnlyTheNamesThePlatformFills(t *testing.T) {
	kept, removed := WithoutPlatformVariables([]AgentVariable{{Name: "turno", DefaultValue: "Manhã"}, {Name: " lead_name ", DefaultValue: "cliente"}, {Name: "campaign_name"}, {Name: "minha var"}})
	if len(kept) != 2 || kept[0].Name != "turno" || kept[1].Name != "minha var" {
		t.Fatalf("kept = %+v", kept)
	}
	if strings.Join(removed, ",") != "lead_name,campaign_name" {
		t.Fatalf("removed = %v", removed)
	}
	if kept, removed := WithoutPlatformVariables([]AgentVariable{{Name: "a"}}); removed != nil || len(kept) != 1 {
		t.Fatalf("a clean list = %+v %v", kept, removed)
	}
}
