package workflow_infra

import (
	"errors"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/workflow"
	conversation_usecase "vozko/usecases/conversation"
)

var (
	errAutomationReaderMissing = errors.New("workflow automation gate: no conversation automation reader wired")
	errConversationRuleMissing = errors.New("workflow automation gate: no conversation automation rule wired")
)

type automationGate struct {
	entries       conversation.EntryAutomationReader
	conversations *conversation_usecase.AutomationGate
}

func NewAutomationGate(entries conversation.EntryAutomationReader, conversations *conversation_usecase.AutomationGate) workflow.AutomationGate {
	return &automationGate{entries: entries, conversations: conversations}
}

func (g *automationGate) AutomationEnabled(entryID, entryType string) (bool, error) {
	if g == nil || g.entries == nil {
		return false, errAutomationReaderMissing
	}
	profile, err := g.entries.EntryAutomation(entryID, entryType)
	if err != nil {
		return false, err
	}
	return conversation.AutomationAllowed(profile.AutomationEnabled), nil
}

func (g *automationGate) AutomationMayAct(workspaceID, entryID, entryType string) (bool, error) {
	if g == nil || g.conversations == nil {
		return false, errConversationRuleMissing
	}
	return g.conversations.MayAct(workspaceID, entryID, shared.EntryType(entryType))
}
