package workflow

import (
	"errors"
	"fmt"
	"strings"

	"vozko/domain/variabledecl"
)

const (
	LintGraphVariableInvalid        LintIssueCode = "GRAPH_VARIABLE_INVALID"
	LintGraphVariableDefaultInvalid LintIssueCode = "GRAPH_VARIABLE_DEFAULT_INVALID"
)

type VariableUse struct {
	Name    string `json:"name"`
	Default string `json:"default,omitempty"`
}

func ConversationRef(name string) string {
	return DataKeyCampaignVars + "." + name
}

func (u VariableUse) Optional() bool {
	return strings.TrimSpace(u.Default) != ""
}

func (g Graph) VariableUse(name string) (VariableUse, bool) {
	for _, use := range g.Variables {
		if use.Name == name {
			return use, true
		}
	}
	return VariableUse{}, false
}

func (g Graph) CampaignVarNodes() map[string][]string {
	out := map[string][]string{}
	for _, node := range g.Nodes {
		for _, name := range ExtractCampaignVars(Graph{Nodes: []Node{node}}) {
			out[name] = append(out[name], node.ID)
		}
	}
	return out
}

func VariableUseIssues(g *Graph) []LintIssue {
	names := make([]string, len(g.Variables))
	for i, use := range g.Variables {
		names[i] = use.Name
	}
	var issues []LintIssue
	for _, problem := range variabledecl.UseProblems(names) {
		message := fmt.Sprintf("O dado %q do fluxo tem um nome inválido", problem.Name)
		if errors.Is(problem.Err, variabledecl.ErrNameRepeated) {
			message = fmt.Sprintf("O dado %q aparece duas vezes nos dados do fluxo", problem.Name)
		}
		issues = append(issues, LintIssue{
			Code: LintGraphVariableInvalid, Severity: SeverityBlocking, Path: problem.Name,
			Message: message, Hint: "Abra Dados do fluxo e corrija ou remova esse dado.",
			Values: map[string]string{"name": problem.Name},
		})
	}
	return issues
}
