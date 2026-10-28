package workflow

import (
	"strconv"
	"strings"

	sm "vozko/domain/scheduled_message"
)

const LintScheduleInvalid LintIssueCode = "NODE_SCHEDULE_INVALID"

func ScheduleIssues(g *Graph) []LintIssue {
	var issues []LintIssue
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.Type != NodeTypeActionScheduleMessage {
			continue
		}
		mode, _ := n.Config[string(sm.FieldWhenMode)].(string)
		fields := sm.FollowUpFieldsOf(mode)
		if fields == nil {
			issues = append(issues, scheduleIssue(n, sm.FieldWhenMode))
			continue
		}
		for _, field := range fields {
			text, literal := literalScheduleText(n.Config[string(field)])
			if !literal {
				continue
			}
			if err := sm.CheckFollowUpField(field, text); err != nil {
				issues = append(issues, scheduleIssue(n, field))
			}
		}
	}
	return issues
}

func literalScheduleText(raw interface{}) (string, bool) {
	switch v := raw.(type) {
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case string:
		return v, strings.TrimSpace(v) != "" && !strings.Contains(v, "{{")
	}
	return "", false
}

func scheduleIssue(n *Node, field sm.FollowUpField) LintIssue {
	return LintIssue{
		Code: LintScheduleInvalid, Severity: SeverityBlocking, NodeID: n.ID, Field: string(field),
		Message: field.Label() + " " + field.Rule() + ".",
		Hint:    "Corrija o valor ou escolha uma variável que traga esse dado.",
		Values:  map[string]string{"label": field.Label()},
	}
}
