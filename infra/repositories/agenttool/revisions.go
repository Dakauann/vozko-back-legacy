package agenttool_repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/infra/database/schema"
)

type RevisionReader struct {
	db *gorm.DB
}

func NewRevisionReader(db *gorm.DB) *RevisionReader {
	return &RevisionReader{db: db}
}

func (r *RevisionReader) ToolRevisions(ctx context.Context, agentID, toolID string) ([]agent.ToolRevision, error) {
	var rows []schema.AgentToolRevision
	if err := r.db.WithContext(ctx).Where("agent_id = ? AND tool_id = ?", agentID, toolID).Order("revision DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: %s", agenttool.ErrToolNotFound, toolID)
	}
	out := make([]agent.ToolRevision, 0, len(rows))
	for _, row := range rows {
		out = append(out, revisionFrom(row))
	}
	return out, nil
}

func (r *RevisionReader) ToolRevision(ctx context.Context, agentID, toolID string, revision int) (agent.ToolRevision, error) {
	var row schema.AgentToolRevision
	err := r.db.WithContext(ctx).Where("agent_id = ? AND tool_id = ? AND revision = ?", agentID, toolID, revision).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return agent.ToolRevision{}, fmt.Errorf("%w: %s revision %d", agenttool.ErrRevisionNotFound, toolID, revision)
	}
	if err != nil {
		return agent.ToolRevision{}, err
	}
	return revisionFrom(row), nil
}

func revisionFrom(row schema.AgentToolRevision) agent.ToolRevision {
	out := agent.ToolRevision{
		ToolID: row.ToolID, Revision: row.Revision, Type: row.Type, Name: row.Name, Description: row.Description,
		SavedBy: row.SavedBy, SavedAt: row.SavedAt,
	}
	_ = json.Unmarshal([]byte(row.Config), &out.Config)
	_ = json.Unmarshal([]byte(row.Visibility), &out.Visibility)
	return out
}

var _ agent.ToolRevisionReader = (*RevisionReader)(nil)

func LoadPinned(db *gorm.DB, agentID string, pins []agent.ToolPin) ([]agent.ToolBinding, error) {
	if len(pins) == 0 {
		return []agent.ToolBinding{}, nil
	}
	ids := make([]string, 0, len(pins))
	for _, p := range pins {
		ids = append(ids, p.ID)
	}
	var rows []schema.AgentToolRevision
	if err := db.Where("agent_id = ? AND tool_id IN ?", agentID, ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	byPin := make(map[agent.ToolPin]schema.AgentToolRevision, len(rows))
	for _, row := range rows {
		byPin[agent.ToolPin{ID: row.ToolID, Revision: row.Revision}] = row
	}
	out := make([]agent.ToolBinding, 0, len(pins))
	for _, p := range pins {
		row, ok := byPin[p]
		if !ok {
			return nil, fmt.Errorf("%w: tool %s revision %d", agenttool.ErrRevisionNotFound, p.ID, p.Revision)
		}
		b := agent.ToolBinding{ID: row.ToolID, Name: row.Type, Function: row.Name, Description: row.Description, Revision: row.Revision}
		var config map[string]interface{}
		if json.Unmarshal([]byte(row.Config), &config) == nil && len(config) > 0 {
			b.Config = config
		}
		var visibility []agent.ToolVisibility
		if json.Unmarshal([]byte(row.Visibility), &visibility) == nil && len(visibility) > 0 {
			b.Visibility = visibility
		}
		out = append(out, b)
	}
	return out, nil
}
