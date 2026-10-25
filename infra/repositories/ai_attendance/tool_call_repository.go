package ai_attendance_repository

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"

	aa "vozko/domain/ai_attendance"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
)

const deleteToolCallsBeforeSQL = `DELETE FROM ai_tool_calls WHERE id IN (SELECT id FROM ai_tool_calls WHERE created_at < ? ORDER BY created_at LIMIT ?)`

type toolCallLog struct {
	db *gorm.DB
}

func NewToolCallLog(db *gorm.DB) aa.ToolCallLog {
	return &toolCallLog{db: db}
}

func (r *toolCallLog) Record(calls []aa.ToolCall) error {
	if len(calls) == 0 {
		return nil
	}
	rows := make([]schema.AIToolCallSchema, len(calls))
	for i, c := range calls {
		rows[i] = ToolCallRow(c)
	}
	return r.db.Create(&rows).Error
}

const toolCallRunJoinSQL = `LEFT JOIN workflow_runs wr ON wr.id = ai_tool_calls.run_id AND wr.workspace_id = ai_tool_calls.workspace_id`

type toolCallWithWorkflow struct {
	schema.AIToolCallSchema
	RunWorkflowID *string
}

func (r *toolCallLog) ListByEntry(workspaceID, entryID, entryType string, pagination shared.Pagination) (*shared.PaginatedResult[*aa.ToolCall], error) {
	pagination = shared.NormalizePagination(pagination)
	var total int64
	if err := r.db.Model(&schema.AIToolCallSchema{}).Where("workspace_id = ? AND entry_id = ? AND entry_type = ?", workspaceID, entryID, entryType).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []toolCallWithWorkflow
	err := r.db.Table("ai_tool_calls").
		Select(listedToolCallColumns+", wr.workflow_id AS run_workflow_id").
		Joins(toolCallRunJoinSQL).
		Where("ai_tool_calls.workspace_id = ? AND ai_tool_calls.entry_id = ? AND ai_tool_calls.entry_type = ?", workspaceID, entryID, entryType).
		Order("ai_tool_calls.created_at DESC").Limit(pagination.PageSize).Offset(pagination.Offset()).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]*aa.ToolCall, 0, len(rows))
	for i := range rows {
		call, err := ToolCallFromRow(&rows[i].AIToolCallSchema)
		if err != nil {
			return nil, err
		}
		if rows[i].RunWorkflowID != nil && call.RunID != "" {
			call.WorkflowID = *rows[i].RunWorkflowID
		}
		items = append(items, call)
	}
	return shared.NewPaginatedResult(items, pagination, total), nil
}

func (r *toolCallLog) DeleteCreatedBefore(cutoff time.Time, limit int) (int64, error) {
	result := r.db.Exec(deleteToolCallsBeforeSQL, cutoff, limit)
	return result.RowsAffected, result.Error
}

func encodedArgs(args map[string]interface{}) *string {
	if len(args) == 0 {
		return nil
	}
	return encodedJSON(args)
}

func encodedJSON(v interface{}) *string {
	encoded, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	s := string(encoded)
	return &s
}

func optionalText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ToolCallFromRow(row *schema.AIToolCallSchema) (*aa.ToolCall, error) {
	call := &aa.ToolCall{
		ID:             row.ID,
		WorkspaceID:    row.WorkspaceID,
		EntryID:        row.EntryID,
		EntryType:      row.EntryType,
		AgentID:        row.AgentID,
		SessionID:      row.SessionID,
		ToolName:       row.ToolName,
		Outcome:        aa.ToolCallOutcome(row.Outcome),
		Error:          row.Error,
		ErrorCode:      row.ErrorCode,
		DurationMs:     row.DurationMs,
		CreatedAt:      row.CreatedAt,
		Source:         row.Source,
		ToolInstanceID: row.ToolInstanceID,
		ToolType:       row.ToolType,
		ToolRevision:   row.ToolRevision,
		Sequence:       row.Sequence,
		ModelCallIndex: row.ModelCallIndex,
		ErrorKind:      row.ErrorKind,
		FollowedBy:     row.FollowedBy,
		Simulated:      row.Simulated,
	}
	if row.AgentRunID != nil {
		call.AgentRunID = *row.AgentRunID
	}
	if row.ModelResult != nil {
		call.ModelResult = *row.ModelResult
	}
	if err := decodeSpanPayloads(row, call); err != nil {
		return nil, err
	}
	if row.RunID != nil {
		call.RunID = *row.RunID
	}
	if row.ToolCallID != nil {
		call.ToolCallID = *row.ToolCallID
	}
	if row.Args != nil && *row.Args != "" {
		if err := json.Unmarshal([]byte(*row.Args), &call.Args); err != nil {
			return nil, err
		}
	}
	return call, nil
}

const listedToolCallColumns = "ai_tool_calls.id, ai_tool_calls.tool_call_id, ai_tool_calls.workspace_id, ai_tool_calls.entry_id, ai_tool_calls.entry_type, ai_tool_calls.agent_id, ai_tool_calls.session_id, ai_tool_calls.tool_name, ai_tool_calls.args, ai_tool_calls.outcome, ai_tool_calls.error, ai_tool_calls.error_code, ai_tool_calls.duration_ms, ai_tool_calls.run_id, ai_tool_calls.created_at, ai_tool_calls.agent_run_id, ai_tool_calls.source, ai_tool_calls.tool_instance_id, ai_tool_calls.tool_type, ai_tool_calls.tool_revision, ai_tool_calls.sequence, ai_tool_calls.model_call_index, ai_tool_calls.error_kind, ai_tool_calls.followed_by, ai_tool_calls.simulated"

func ToolCallRow(c aa.ToolCall) schema.AIToolCallSchema {
	row := schema.AIToolCallSchema{
		ID:             c.ID,
		ToolCallID:     optionalText(c.ToolCallID),
		WorkspaceID:    c.WorkspaceID,
		EntryID:        c.EntryID,
		EntryType:      c.EntryType,
		AgentID:        c.AgentID,
		SessionID:      c.SessionID,
		ToolName:       c.ToolName,
		Args:           encodedArgs(c.Args),
		Outcome:        string(c.Outcome),
		Error:          c.Error,
		ErrorCode:      c.ErrorCode,
		DurationMs:     c.DurationMs,
		RunID:          optionalText(c.RunID),
		CreatedAt:      c.CreatedAt,
		AgentRunID:     optionalText(c.AgentRunID),
		Source:         c.Source,
		ToolInstanceID: c.ToolInstanceID,
		ToolType:       c.ToolType,
		ToolRevision:   c.ToolRevision,
		Sequence:       c.Sequence,
		ModelCallIndex: c.ModelCallIndex,
		ErrorKind:      c.ErrorKind,
		ModelResult:    optionalText(c.ModelResult),
		FollowedBy:     c.FollowedBy,
		Simulated:      c.Simulated,
	}
	if len(c.ErrorChain) > 0 {
		row.ErrorChain = encodedJSON(c.ErrorChain)
	}
	if len(c.HTTP) > 0 {
		row.HTTP = encodedJSON(c.HTTP)
	}
	if c.MappedOutput != nil {
		row.MappedOutput = encodedJSON(c.MappedOutput)
	}
	return row
}

func decodeSpanPayloads(row *schema.AIToolCallSchema, call *aa.ToolCall) error {
	if row.ErrorChain != nil && *row.ErrorChain != "" {
		if err := json.Unmarshal([]byte(*row.ErrorChain), &call.ErrorChain); err != nil {
			return err
		}
	}
	if row.HTTP != nil && *row.HTTP != "" {
		if err := json.Unmarshal([]byte(*row.HTTP), &call.HTTP); err != nil {
			return err
		}
	}
	if row.MappedOutput != nil && *row.MappedOutput != "" {
		if err := json.Unmarshal([]byte(*row.MappedOutput), &call.MappedOutput); err != nil {
			return err
		}
	}
	return nil
}
