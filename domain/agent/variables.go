package agent

import (
	"errors"
	"fmt"
	"strings"

	"vozko/domain/variabledecl"
)

var useRefusals = []struct {
	rule  error
	agent error
}{
	{variabledecl.ErrNameReserved, ErrAgentVariableNameReserved},
	{variabledecl.ErrNameRepeated, ErrAgentVariableDuplicate},
	{variabledecl.ErrNameInvalid, ErrAgentVariableNameInvalid},
}

func VariableProblems(vars []AgentVariable) map[string]error {
	problems := variabledecl.UseProblems(variableNames(vars))
	if len(problems) == 0 {
		return nil
	}
	out := make(map[string]error, len(problems))
	for _, problem := range problems {
		if _, seen := out[problem.Name]; !seen {
			out[problem.Name] = agentRefusal(problem)
		}
	}
	return out
}

func firstVariableProblem(vars []AgentVariable) error {
	if problems := variabledecl.UseProblems(variableNames(vars)); len(problems) > 0 {
		return agentRefusal(problems[0])
	}
	return nil
}

func variableNames(vars []AgentVariable) []string {
	names := make([]string, len(vars))
	for i, v := range vars {
		names[i] = v.Name
	}
	return names
}

func agentRefusal(problem variabledecl.UseProblem) error {
	for _, r := range useRefusals {
		if errors.Is(problem.Err, r.rule) {
			return fmt.Errorf("%w: %q", r.agent, problem.Name)
		}
	}
	return fmt.Errorf("%w: %q", ErrAgentVariableNameInvalid, problem.Name)
}

func (a *Agent) VariableDefaults() map[string]string {
	out := map[string]string{}
	for _, v := range a.Variables {
		if _, seen := out[v.Name]; seen {
			continue
		}
		if fallback := strings.TrimSpace(v.DefaultValue); fallback != "" {
			out[v.Name] = fallback
		}
	}
	return out
}

func WithoutPlatformVariables(vars []AgentVariable) ([]AgentVariable, []string) {
	var removed []string
	kept := make([]AgentVariable, 0, len(vars))
	for _, v := range vars {
		name := strings.TrimSpace(v.Name)
		if errors.Is(variabledecl.CheckName(name), variabledecl.ErrNameReserved) {
			removed = append(removed, name)
			continue
		}
		kept = append(kept, v)
	}
	return kept, removed
}
