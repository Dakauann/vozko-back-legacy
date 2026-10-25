package agenttool_repository

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"vozko/domain/agent"
	"vozko/infra/database/schema"
)

const agentsWithToolsAfterSQL = `SELECT DISTINCT agent_id, workspace_id FROM agent_tools
WHERE agent_id > ?::uuid AND deleted_at IS NULL
ORDER BY agent_id LIMIT ?`

const agentsWithRevisionsAfterSQL = `SELECT DISTINCT agent_id, workspace_id FROM agent_tool_revisions
WHERE agent_id > ?::uuid
ORDER BY agent_id LIMIT ?`

type agentRef struct {
	AgentID     string
	WorkspaceID string
}

type LiveToolScanner struct {
	db      *gorm.DB
	onSaved func(agentID string)
}

func NewLiveToolScanner(db *gorm.DB, onSaved func(agentID string)) *LiveToolScanner {
	return &LiveToolScanner{db: db, onSaved: onSaved}
}

func (s *LiveToolScanner) AgentToolsAfter(ctx context.Context, afterID string, limit int) ([]agent.StoredAgentTools, error) {
	var refs []agentRef
	if err := s.db.WithContext(ctx).Raw(agentsWithToolsAfterSQL, afterID, limit).Scan(&refs).Error; err != nil {
		return nil, err
	}
	out := make([]agent.StoredAgentTools, 0, len(refs))
	for _, ref := range refs {
		bindings, err := LoadBindings(s.db.WithContext(ctx), ref.AgentID)
		if err != nil {
			return nil, err
		}
		out = append(out, agent.StoredAgentTools{AgentID: ref.AgentID, WorkspaceID: ref.WorkspaceID, Bindings: bindings})
	}
	return out, nil
}

func (s *LiveToolScanner) SaveAgentTools(ctx context.Context, agentID string, bindings []agent.ToolBinding) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, b := range bindings {
			config, err := encodeJSON(b.Config, "{}")
			if err != nil {
				return err
			}
			if err := tx.Model(&schema.AgentTool{}).Where("id = ? AND agent_id = ?", b.ID, agentID).UpdateColumn("config", config).Error; err != nil {
				return err
			}
		}
		live, err := LoadBindings(tx, agentID)
		if err != nil {
			return err
		}
		return mirrorLegacyBindings(tx, agentID, live)
	})
	if err == nil && s.onSaved != nil {
		s.onSaved(agentID)
	}
	return err
}

type RevisionScanner struct {
	db *gorm.DB
}

func NewRevisionScanner(db *gorm.DB) *RevisionScanner {
	return &RevisionScanner{db: db}
}

func (s *RevisionScanner) AgentToolsAfter(ctx context.Context, afterID string, limit int) ([]agent.StoredAgentTools, error) {
	var refs []agentRef
	if err := s.db.WithContext(ctx).Raw(agentsWithRevisionsAfterSQL, afterID, limit).Scan(&refs).Error; err != nil {
		return nil, err
	}
	out := make([]agent.StoredAgentTools, 0, len(refs))
	for _, ref := range refs {
		var rows []schema.AgentToolRevision
		if err := s.db.WithContext(ctx).Where("agent_id = ?", ref.AgentID).Order("tool_id, revision").Find(&rows).Error; err != nil {
			return nil, err
		}
		bindings := make([]agent.ToolBinding, 0, len(rows))
		for _, row := range rows {
			b := agent.ToolBinding{ID: row.ToolID, Name: row.Type, Function: row.Name, Revision: row.Revision}
			_ = json.Unmarshal([]byte(row.Config), &b.Config)
			bindings = append(bindings, b)
		}
		out = append(out, agent.StoredAgentTools{AgentID: ref.AgentID, WorkspaceID: ref.WorkspaceID, Bindings: bindings})
	}
	return out, nil
}

func (s *RevisionScanner) SaveAgentTools(ctx context.Context, agentID string, bindings []agent.ToolBinding) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, b := range bindings {
			config, err := encodeJSON(b.Config, "{}")
			if err != nil {
				return err
			}
			if err := tx.Model(&schema.AgentToolRevision{}).Where("tool_id = ? AND revision = ? AND agent_id = ?", b.ID, b.Revision, agentID).UpdateColumn("config", config).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

var (
	_ agent.StoredToolScanner = (*LiveToolScanner)(nil)
	_ agent.StoredToolScanner = (*RevisionScanner)(nil)
)
