package workflow_infra

import (
	"errors"

	"vozko/domain/shared"
	"vozko/domain/workflow"
	conversation_usecase "vozko/usecases/conversation"
)

var errResponsibilityRuleMissing = errors.New("workflow responsibility: no conversation automation gate wired")

type conversationResponsibility struct {
	gate *conversation_usecase.AutomationGate
}

func NewConversationResponsibility(gate *conversation_usecase.AutomationGate) workflow.ConversationResponsibility {
	return &conversationResponsibility{gate: gate}
}

func (r *conversationResponsibility) ResponsibleWorkflow(workspaceID, entryID, entryType string) (string, bool, error) {
	if r == nil || r.gate == nil {
		return "", false, errResponsibilityRuleMissing
	}
	return r.gate.ResponsibleWorkflow(workspaceID, entryID, shared.EntryType(entryType))
}
