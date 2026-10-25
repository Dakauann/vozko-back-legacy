package agentrun_repository

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/agentrun"
	"vozko/domain/ai"
	aa "vozko/domain/ai_attendance"
	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/workflow"
	"vozko/infra/database/schema"
	ai_attendance_repository "vozko/infra/repositories/ai_attendance"
	conversation_repository "vozko/infra/repositories/conversation"
)

const (
	pruneSpanPayloadsSQL = `UPDATE ai_tool_calls SET args = NULL, model_result = NULL, http = NULL, mapped_output = NULL, error_chain = NULL
WHERE id IN (
	SELECT id FROM ai_tool_calls
	WHERE created_at < ? AND (args IS NOT NULL OR model_result IS NOT NULL OR http IS NOT NULL OR mapped_output IS NOT NULL OR error_chain IS NOT NULL)
	ORDER BY created_at LIMIT ?
)`
	pruneRunTextsSQL = `UPDATE agent_runs SET user_message = '', reply = ''
WHERE id IN (
	SELECT id FROM agent_runs
	WHERE started_at < ? AND (user_message <> '' OR reply <> '')
	ORDER BY started_at LIMIT ?
)`
	deleteRunsBeforeSQL = `DELETE FROM agent_runs WHERE id IN (SELECT id FROM agent_runs WHERE started_at < ? ORDER BY started_at LIMIT ?)`
	runColumns          = `agent_runs.id, agent_runs.workspace_id, agent_runs.agent_id, agent_runs.agent_version, agent_runs.entry_id, agent_runs.entry_type, agent_runs.source, agent_runs.model, agent_runs.status, agent_runs.error_kind, agent_runs.error, agent_runs.duration_ms, agent_runs.input_tokens, agent_runs.output_tokens, agent_runs.cached_tokens, agent_runs.reasoning_tokens, agent_runs.cost_micros, agent_runs.tool_calls, agent_runs.failed_tool_calls, agent_runs.handled_failures, agent_runs.model_calls, agent_runs.tools, agent_runs.started_at`
)

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) agentrun.Repository {
	return &repository{db: db}
}

func (r *repository) Record(run *agentrun.Run, spans []aa.ToolCall) error {
	if run == nil && len(spans) == 0 {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if run != nil {
			row, err := rowOf(run)
			if err != nil {
				return err
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return ai_attendance_repository.NewToolCallLog(tx).Record(spans)
	})
}

func (r *repository) List(workspaceID, agentID string, filter agentrun.Filter, visibility conversation.VisibilityScope, pagination shared.Pagination) (*shared.PaginatedResult[*agentrun.Run], error) {
	pagination = shared.NormalizePagination(pagination)
	where, args := listConditions(workspaceID, agentID, filter, visibility)
	var total int64
	if err := r.db.Raw("SELECT COUNT(*) FROM agent_runs WHERE "+where, args...).Scan(&total).Error; err != nil {
		return nil, err
	}
	var rows []schema.AgentRunSchema
	page := "SELECT " + runColumns + " FROM agent_runs WHERE " + where + " ORDER BY agent_runs.started_at DESC LIMIT ? OFFSET ?"
	if err := r.db.Raw(page, append(args, pagination.PageSize, pagination.Offset())...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]*agentrun.Run, 0, len(rows))
	for i := range rows {
		run, err := runOf(&rows[i])
		if err != nil {
			return nil, err
		}
		items = append(items, run)
	}
	return shared.NewPaginatedResult(items, pagination, total), nil
}

func listConditions(workspaceID, agentID string, f agentrun.Filter, visibility conversation.VisibilityScope) (string, []interface{}) {
	clauses := []string{"agent_runs.workspace_id = ?", "agent_runs.agent_id = ?"}
	args := []interface{}{workspaceID, agentID}
	add := func(clause string, values ...interface{}) {
		clauses = append(clauses, clause)
		args = append(args, values...)
	}
	if !f.From.IsZero() {
		add("agent_runs.started_at >= ?", f.From)
	}
	if !f.To.IsZero() {
		add("agent_runs.started_at <= ?", f.To)
	}
	if f.Source != "" {
		add("agent_runs.source = ?", string(f.Source))
	}
	if f.Status != "" {
		add("agent_runs.status = ?", string(f.Status))
	}
	if f.EntryID != "" {
		add("agent_runs.entry_id = ?", f.EntryID)
	}
	if f.HandledFailure {
		add("agent_runs.handled_failures > 0")
	}
	if f.ErrorKind == agentrun.ErrorKindModel {
		add("agent_runs.error_kind = ?", agentrun.ErrorKindModel)
	}
	if span, spanArgs := spanCondition(f); span != "" {
		add("EXISTS (SELECT 1 FROM ai_tool_calls c WHERE c.agent_run_id = agent_runs.id AND c.workspace_id = agent_runs.workspace_id"+span+")", spanArgs...)
	}
	if visible, visibleArgs := visibleRuns(workspaceID, visibility); visible != "" {
		add(visible, visibleArgs...)
	}
	return strings.Join(clauses, " AND "), args
}

func spanCondition(f agentrun.Filter) (string, []interface{}) {
	var clause string
	var args []interface{}
	if f.ToolInstanceID != "" {
		clause += " AND c.tool_instance_id = ?"
		args = append(args, f.ToolInstanceID)
	}
	if f.ErrorKind != "" && f.ErrorKind != agentrun.ErrorKindModel {
		clause += " AND c.error_kind = ?"
		args = append(args, f.ErrorKind)
	}
	return clause, args
}

func visibleRuns(workspaceID string, visibility conversation.VisibilityScope) (string, []interface{}) {
	clause, args := conversation_repository.VisibleEntrySQL("agent_runs.entry_id", "agent_runs.entry_type", workspaceID, visibility)
	if clause == "" {
		return "", nil
	}
	return "(agent_runs.entry_id IS NULL OR agent_runs.entry_type IN ('" + workflow.EntryTypeSIPCall + "', '" + workflow.EntryTypeLead + "') OR " + clause + ")", args
}

func (r *repository) Find(workspaceID, agentID, runID string) (*agentrun.Detail, error) {
	if _, err := uuid.Parse(runID); err != nil {
		return nil, agentrun.ErrRunNotFound
	}
	var row schema.AgentRunSchema
	err := r.db.Where("id = ? AND workspace_id = ? AND agent_id = ?", runID, workspaceID, agentID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, agentrun.ErrRunNotFound
	}
	if err != nil {
		return nil, err
	}
	run, err := runOf(&row)
	if err != nil {
		return nil, err
	}
	var spanRows []schema.AIToolCallSchema
	if err := r.db.Where("agent_run_id = ? AND workspace_id = ?", runID, workspaceID).Order("sequence").Find(&spanRows).Error; err != nil {
		return nil, err
	}
	spans := make([]aa.ToolCall, 0, len(spanRows))
	for i := range spanRows {
		span, err := ai_attendance_repository.ToolCallFromRow(&spanRows[i])
		if err != nil {
			return nil, err
		}
		spans = append(spans, *span)
	}
	return &agentrun.Detail{Run: *run, Spans: spans}, nil
}

func (r *repository) PrunePayloads(before time.Time, limit int) (int64, error) {
	spans := r.db.Exec(pruneSpanPayloadsSQL, before, limit)
	if spans.Error != nil {
		return 0, spans.Error
	}
	runs := r.db.Exec(pruneRunTextsSQL, before, limit)
	return spans.RowsAffected + runs.RowsAffected, runs.Error
}

func (r *repository) DeleteStartedBefore(before time.Time, limit int) (int64, error) {
	result := r.db.Exec(deleteRunsBeforeSQL, before, limit)
	return result.RowsAffected, result.Error
}

func rowOf(run *agentrun.Run) (schema.AgentRunSchema, error) {
	modelCalls, err := json.Marshal(nonNilModelCalls(run.ModelCalls))
	if err != nil {
		return schema.AgentRunSchema{}, err
	}
	pins, err := json.Marshal(nonNilPins(run.Tools))
	if err != nil {
		return schema.AgentRunSchema{}, err
	}
	row := schema.AgentRunSchema{
		ID:              run.ID,
		WorkspaceID:     run.WorkspaceID,
		AgentID:         run.AgentID,
		AgentVersion:    run.AgentVersion,
		EntryType:       run.EntryType,
		Source:          string(run.Source),
		Model:           run.Model,
		Status:          string(run.Status),
		ErrorKind:       run.ErrorKind,
		Error:           run.Error,
		DurationMs:      run.DurationMs,
		InputTokens:     run.InputTokens,
		OutputTokens:    run.OutputTokens,
		CachedTokens:    run.CachedTokens,
		ReasoningTokens: run.ReasoningTokens,
		CostMicros:      run.CostMicros,
		ToolCalls:       run.ToolCalls,
		FailedToolCalls: run.FailedToolCalls,
		HandledFailures: run.HandledFailures,
		ModelCalls:      string(modelCalls),
		Tools:           string(pins),
		UserMessage:     run.UserMessage,
		Reply:           run.Reply,
		StartedAt:       run.StartedAt,
	}
	if _, err := uuid.Parse(run.EntryID); err == nil {
		entryID := run.EntryID
		row.EntryID = &entryID
	}
	return row, nil
}

func runOf(row *schema.AgentRunSchema) (*agentrun.Run, error) {
	run := &agentrun.Run{
		ID:              row.ID,
		WorkspaceID:     row.WorkspaceID,
		AgentID:         row.AgentID,
		AgentVersion:    row.AgentVersion,
		EntryType:       row.EntryType,
		Source:          agentrun.Source(row.Source),
		Model:           row.Model,
		Status:          agentrun.Status(row.Status),
		ErrorKind:       row.ErrorKind,
		Error:           row.Error,
		DurationMs:      row.DurationMs,
		InputTokens:     row.InputTokens,
		OutputTokens:    row.OutputTokens,
		CachedTokens:    row.CachedTokens,
		ReasoningTokens: row.ReasoningTokens,
		CostMicros:      row.CostMicros,
		ToolCalls:       row.ToolCalls,
		FailedToolCalls: row.FailedToolCalls,
		HandledFailures: row.HandledFailures,
		UserMessage:     row.UserMessage,
		Reply:           row.Reply,
		StartedAt:       row.StartedAt,
		ModelCalls:      []ai.ModelCall{},
		Tools:           []agentrun.ToolPin{},
	}
	if row.EntryID != nil {
		run.EntryID = *row.EntryID
	}
	if row.ModelCalls != "" {
		if err := json.Unmarshal([]byte(row.ModelCalls), &run.ModelCalls); err != nil {
			return nil, err
		}
	}
	if row.Tools != "" {
		if err := json.Unmarshal([]byte(row.Tools), &run.Tools); err != nil {
			return nil, err
		}
	}
	return run, nil
}

func nonNilModelCalls(calls []ai.ModelCall) []ai.ModelCall {
	if calls == nil {
		return []ai.ModelCall{}
	}
	return calls
}

func nonNilPins(pins []agentrun.ToolPin) []agentrun.ToolPin {
	if pins == nil {
		return []agentrun.ToolPin{}
	}
	return pins
}
