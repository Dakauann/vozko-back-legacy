package agenttool_repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/agent"
	"vozko/infra/database/schema"
)

type ToolNames struct {
	db *gorm.DB
}

func NewToolNames(db *gorm.DB) *ToolNames {
	return &ToolNames{db: db}
}

func (n *ToolNames) FindToolsByIDs(ids []string) ([]agent.ToolBinding, error) {
	valid := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err == nil {
			valid = append(valid, id)
		}
	}
	if len(valid) == 0 {
		return []agent.ToolBinding{}, nil
	}
	var rows []schema.AgentTool
	if err := n.db.Unscoped().Select("id", "agent_id", "type", "name").Where("id IN ?", valid).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]agent.ToolBinding, 0, len(rows))
	for _, row := range rows {
		out = append(out, agent.ToolBinding{ID: row.ID, Name: row.Type, Function: row.Name})
	}
	return out, nil
}
