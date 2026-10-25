package agenttool_repository

import (
	"gorm.io/gorm"

	"vozko/domain/agent"
	"vozko/infra/database/schema"
)

func mirrorLegacyBindings(tx *gorm.DB, agentID string, bindings []agent.ToolBinding) error {
	return tx.Model(&schema.Agent{}).Where("id = ?", agentID).UpdateColumn("tool_bindings", agent.EncodeLegacyBindings(bindings)).Error
}
