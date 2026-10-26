package workflow_repository

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"vozko/domain/shared"
	"vozko/domain/workflow"
)

const sentPromptsSQL = `SELECT l.id AS step_id, r.workflow_id, r.entry_id, r.entry_type,
	l.output->>'message_id' AS provider_message_id, l.input::text AS config
FROM workflow_run_logs l JOIN workflow_runs r ON r.id = l.run_id
WHERE r.entry_type = ? AND l.node_type IN ? AND l.id > ?::uuid
	AND l.input IS NOT NULL AND COALESCE(l.output->>'message_id', '') <> ''
ORDER BY l.id
LIMIT ?`

type sentPromptRepository struct {
	db *gorm.DB
}

func NewSentPromptRepository(db *gorm.DB) workflow.SentPromptSource {
	return &sentPromptRepository{db: db}
}

type sentPromptRow struct {
	StepID            string
	WorkflowID        string
	EntryID           string
	EntryType         string
	ProviderMessageID string
	Config            string
}

func (r *sentPromptRepository) SentPrompts(ctx context.Context, entryType shared.EntryType, nodeTypes []workflow.NodeType, after string, limit int) ([]workflow.SentPrompt, error) {
	types := make([]string, len(nodeTypes))
	for i, nodeType := range nodeTypes {
		types[i] = string(nodeType)
	}
	var rows []sentPromptRow
	if err := r.db.WithContext(ctx).Raw(sentPromptsSQL, string(entryType), types, after, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	prompts := make([]workflow.SentPrompt, 0, len(rows))
	for _, row := range rows {
		var config map[string]interface{}
		if err := json.Unmarshal([]byte(row.Config), &config); err != nil {
			return nil, fmt.Errorf("step %s keeps an unreadable input: %w", row.StepID, err)
		}
		prompts = append(prompts, workflow.SentPrompt{
			StepID: row.StepID, WorkflowID: row.WorkflowID, EntryID: row.EntryID, EntryType: shared.EntryType(row.EntryType),
			ProviderMessageID: row.ProviderMessageID, Config: config,
		})
	}
	return prompts, nil
}
