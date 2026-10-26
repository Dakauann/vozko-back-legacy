package workflow_usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	"vozko/domain/workflow"
	"vozko/usecases/workflow/node_executors"
)

var (
	ErrPromptRepairChannelUnsupported = errors.New("prompt transcript repair: this channel has no shaper for its options")
	ErrPromptRepairPageInvalid        = errors.New("prompt transcript repair: the page size must be positive")
)

type PromptTranscriptRepairDeps struct {
	Prompts  workflow.SentPromptSource
	Messages conversation.InteractiveBackfill
	Shapers  map[shared.EntryType]conversation.InteractiveShaper
}

type PromptTranscriptRepair struct {
	prompts  workflow.SentPromptSource
	messages conversation.InteractiveBackfill
	shapers  map[shared.EntryType]conversation.InteractiveShaper
}

type PromptRepairTally struct {
	Scanned    int
	Repaired   int
	Untouched  int
	Unreadable int
}

func NewPromptTranscriptRepair(d PromptTranscriptRepairDeps) (*PromptTranscriptRepair, error) {
	if err := deps.Require("prompt transcript repair", map[string]any{
		"sent prompts": d.Prompts, "messages": d.Messages, "shapers": d.Shapers,
	}); err != nil {
		return nil, err
	}
	return &PromptTranscriptRepair{prompts: d.Prompts, messages: d.Messages, shapers: d.Shapers}, nil
}

func (r *PromptTranscriptRepair) Run(ctx context.Context, entryType shared.EntryType, page int) (PromptRepairTally, error) {
	var tally PromptRepairTally
	shape, ok := r.shapers[entryType]
	if !ok || shape == nil {
		return tally, fmt.Errorf("%w: %s", ErrPromptRepairChannelUnsupported, entryType)
	}
	if page <= 0 {
		return tally, ErrPromptRepairPageInvalid
	}
	after := workflow.SentPromptCursorStart
	for {
		if err := ctx.Err(); err != nil {
			return tally, err
		}
		prompts, err := r.prompts.SentPrompts(ctx, entryType, workflow.InteractivePromptNodeTypes(), after, page)
		if err != nil {
			return tally, fmt.Errorf("sent prompts after %s: %w", after, err)
		}
		for _, prompt := range prompts {
			after = prompt.StepID
			tally.Scanned++
			if err := r.repair(ctx, shape, prompt, &tally); err != nil {
				return tally, fmt.Errorf("step %s: %w", prompt.StepID, err)
			}
		}
		if len(prompts) < page {
			return tally, nil
		}
	}
}

func (r *PromptTranscriptRepair) repair(ctx context.Context, shape conversation.InteractiveShaper, prompt workflow.SentPrompt, tally *PromptRepairTally) error {
	req, err := node_executors.InteractivePromptRequest(prompt.Config, func(text string) string { return text })
	if err != nil || strings.TrimSpace(req.Body) == "" {
		tally.Unreadable++
		return nil
	}
	repaired, err := r.messages.AttachMissingInteractive(ctx, conversation.SentMessageRef{
		EntryType: prompt.EntryType, EntryID: prompt.EntryID, ProviderMessageID: prompt.ProviderMessageID,
		SentBy: conversation.SentByWorkflow(prompt.WorkflowID),
	}, shape(req))
	if err != nil {
		return err
	}
	if repaired {
		tally.Repaired++
	} else {
		tally.Untouched++
	}
	return nil
}
