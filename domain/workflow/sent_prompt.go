package workflow

import (
	"context"
	"slices"

	"vozko/domain/shared"
)

const SentPromptCursorStart = "00000000-0000-0000-0000-000000000000"

type SentPrompt struct {
	StepID            string
	WorkflowID        string
	EntryID           string
	EntryType         shared.EntryType
	ProviderMessageID string
	Config            map[string]interface{}
}

type SentPromptSource interface {
	SentPrompts(ctx context.Context, entryType shared.EntryType, nodeTypes []NodeType, after string, limit int) ([]SentPrompt, error)
}

func InteractivePromptNodeTypes() []NodeType {
	types := []NodeType{NodeTypeActionSendInteractive}
	for _, legacy := range LegacyNodeTypes() {
		if legacy.IsInteractivePrompt() {
			types = append(types, legacy)
		}
	}
	slices.Sort(types)
	return types
}
