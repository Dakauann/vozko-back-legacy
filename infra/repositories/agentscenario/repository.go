package agentscenario_repository

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/agentscenario"
	"vozko/infra/database/schema"
)

const (
	latestRunsSQL = `SELECT id, scenario_id, passed, finished_at FROM (
	SELECT id, scenario_id, passed, finished_at,
		ROW_NUMBER() OVER (PARTITION BY scenario_id ORDER BY started_at DESC) AS position
	FROM agent_scenario_runs
	WHERE scenario_id IN ?
) ranked WHERE position <= 2 ORDER BY scenario_id, position`
	deleteRunsBeforeSQL = `DELETE FROM agent_scenario_runs WHERE id IN (SELECT id FROM agent_scenario_runs WHERE started_at < ? ORDER BY started_at LIMIT ?)`
)

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) agentscenario.Repository {
	return &repository{db: db}
}

func (r *repository) List(workspaceID, agentID string) ([]*agentscenario.Scenario, error) {
	var rows []schema.AgentScenarioSchema
	if err := r.db.Where("workspace_id = ? AND agent_id = ?", workspaceID, agentID).Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*agentscenario.Scenario, 0, len(rows))
	ids := make([]string, 0, len(rows))
	for i := range rows {
		s, err := scenarioOf(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, s)
		ids = append(ids, s.ID)
	}
	if len(ids) == 0 {
		return out, nil
	}
	summaries, err := r.latestRuns(ids)
	if err != nil {
		return nil, err
	}
	for _, s := range out {
		latest := summaries[s.ID]
		if len(latest) > 0 {
			s.Last = latest[0]
		}
		if len(latest) > 1 {
			s.Previous = latest[1]
		}
	}
	return out, nil
}

type summaryRow struct {
	ID         string
	ScenarioID string
	Passed     bool
	FinishedAt time.Time
}

func (r *repository) latestRuns(ids []string) (map[string][]*agentscenario.RunSummary, error) {
	var rows []summaryRow
	if err := r.db.Raw(latestRunsSQL, ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string][]*agentscenario.RunSummary{}
	for _, row := range rows {
		out[row.ScenarioID] = append(out[row.ScenarioID], &agentscenario.RunSummary{ID: row.ID, Passed: row.Passed, FinishedAt: row.FinishedAt})
	}
	return out, nil
}

func (r *repository) Find(workspaceID, agentID, scenarioID string) (*agentscenario.Scenario, error) {
	if _, err := uuid.Parse(scenarioID); err != nil {
		return nil, agentscenario.ErrNotFound
	}
	var row schema.AgentScenarioSchema
	err := r.db.Where("id = ? AND workspace_id = ? AND agent_id = ?", scenarioID, workspaceID, agentID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, agentscenario.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s, err := scenarioOf(&row)
	if err != nil {
		return nil, err
	}
	summaries, err := r.latestRuns([]string{s.ID})
	if err != nil {
		return nil, err
	}
	if latest := summaries[s.ID]; len(latest) > 0 {
		s.Last = latest[0]
		if len(latest) > 1 {
			s.Previous = latest[1]
		}
	}
	return s, nil
}

func (r *repository) Create(s *agentscenario.Scenario) error {
	row, err := rowOf(s)
	if err != nil {
		return err
	}
	return r.db.Create(&row).Error
}

func (r *repository) Update(s *agentscenario.Scenario) error {
	row, err := rowOf(s)
	if err != nil {
		return err
	}
	result := r.db.Model(&schema.AgentScenarioSchema{}).
		Where("id = ? AND workspace_id = ? AND agent_id = ?", s.ID, s.WorkspaceID, s.AgentID).
		Updates(map[string]interface{}{"name": row.Name, "messages": row.Messages, "mocks": row.Mocks, "assertions": row.Assertions, "updated_at": row.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return agentscenario.ErrNotFound
	}
	return nil
}

func (r *repository) Delete(workspaceID, agentID, scenarioID string) error {
	if _, err := uuid.Parse(scenarioID); err != nil {
		return agentscenario.ErrNotFound
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND workspace_id = ? AND agent_id = ?", scenarioID, workspaceID, agentID).Delete(&schema.AgentScenarioSchema{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return agentscenario.ErrNotFound
		}
		return tx.Where("scenario_id = ?", scenarioID).Delete(&schema.AgentScenarioRunSchema{}).Error
	})
}

func (r *repository) RecordRun(run *agentscenario.Run) error {
	results, err := encode(run.Results, "[]")
	if err != nil {
		return err
	}
	turns, err := encode(run.Turns, "[]")
	if err != nil {
		return err
	}
	runIDs, err := encode(run.RunIDs, "[]")
	if err != nil {
		return err
	}
	return r.db.Create(&schema.AgentScenarioRunSchema{
		ID: run.ID, ScenarioID: run.ScenarioID, WorkspaceID: run.WorkspaceID, AgentID: run.AgentID, Passed: run.Passed,
		Results: results, Turns: turns, RunIDs: runIDs, Error: run.Error, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}).Error
}

func (r *repository) Runs(workspaceID, agentID, scenarioID string, limit int) ([]*agentscenario.Run, error) {
	if _, err := uuid.Parse(scenarioID); err != nil {
		return nil, agentscenario.ErrNotFound
	}
	var rows []schema.AgentScenarioRunSchema
	if err := r.db.Where("scenario_id = ? AND workspace_id = ? AND agent_id = ?", scenarioID, workspaceID, agentID).
		Order("started_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*agentscenario.Run, 0, len(rows))
	for i := range rows {
		run, err := runOf(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, nil
}

func (r *repository) DeleteRunsStartedBefore(before time.Time, limit int) (int64, error) {
	result := r.db.Exec(deleteRunsBeforeSQL, before, limit)
	return result.RowsAffected, result.Error
}

func rowOf(s *agentscenario.Scenario) (schema.AgentScenarioSchema, error) {
	messages, err := encode(s.Messages, "[]")
	if err != nil {
		return schema.AgentScenarioSchema{}, err
	}
	mocks, err := encode(s.Mocks, "{}")
	if err != nil {
		return schema.AgentScenarioSchema{}, err
	}
	assertions, err := encode(s.Assertions, "[]")
	if err != nil {
		return schema.AgentScenarioSchema{}, err
	}
	return schema.AgentScenarioSchema{
		ID: s.ID, WorkspaceID: s.WorkspaceID, AgentID: s.AgentID, Name: s.Name, Messages: messages, Mocks: mocks, Assertions: assertions,
		SourceRunID: s.SourceRunID, CreatedBy: s.CreatedBy, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}, nil
}

func scenarioOf(row *schema.AgentScenarioSchema) (*agentscenario.Scenario, error) {
	s := &agentscenario.Scenario{
		ID: row.ID, WorkspaceID: row.WorkspaceID, AgentID: row.AgentID, Name: row.Name, SourceRunID: row.SourceRunID,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		Messages: []agentscenario.Message{}, Mocks: map[string]agentscenario.Mock{}, Assertions: []agentscenario.Assertion{},
	}
	if err := decodeAll(jsonField{row.Messages, &s.Messages}, jsonField{row.Mocks, &s.Mocks}, jsonField{row.Assertions, &s.Assertions}); err != nil {
		return nil, err
	}
	return s, nil
}

func runOf(row *schema.AgentScenarioRunSchema) (*agentscenario.Run, error) {
	run := &agentscenario.Run{
		ID: row.ID, ScenarioID: row.ScenarioID, WorkspaceID: row.WorkspaceID, AgentID: row.AgentID, Passed: row.Passed,
		Error: row.Error, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
		Results: []agentscenario.AssertionResult{}, Turns: []agentscenario.Turn{}, RunIDs: []string{},
	}
	if err := decodeAll(jsonField{row.Results, &run.Results}, jsonField{row.Turns, &run.Turns}, jsonField{row.RunIDs, &run.RunIDs}); err != nil {
		return nil, err
	}
	return run, nil
}

type jsonField struct {
	raw  string
	into interface{}
}

func decodeAll(fields ...jsonField) error {
	for _, field := range fields {
		if field.raw == "" {
			continue
		}
		if err := json.Unmarshal([]byte(field.raw), field.into); err != nil {
			return err
		}
	}
	return nil
}

func encode(v interface{}, empty string) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if string(raw) == "null" {
		return empty, nil
	}
	return string(raw), nil
}
