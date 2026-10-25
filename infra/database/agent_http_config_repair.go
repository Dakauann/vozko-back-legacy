package database

import (
	"encoding/json"
	"log/slog"

	"gorm.io/gorm"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
)

const legacyHTTPToolsSQL = `SELECT id, agent_id, config FROM agent_tools
WHERE id > ?::uuid AND type = 'http_request' AND deleted_at IS NULL
	AND (jsonb_exists(config, 'headers') OR jsonb_exists(config, 'path_params') OR jsonb_exists(config, 'query_schema') OR jsonb_exists(config, 'body_schema'))
ORDER BY id LIMIT ?`

const liveAgentToolsSQL = `SELECT type, config, visibility FROM agent_tools
WHERE agent_id = ? AND deleted_at IS NULL ORDER BY position, created_at, id`

type legacyHTTPToolRow struct {
	ID      string
	AgentID string
	Config  string
}

func canonicalizeAgentHTTPConfigs(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("agent_tools") {
		return nil
	}
	after := workflowVersionRepairFirstKey
	for {
		var rows []legacyHTTPToolRow
		if err := tx.Raw(legacyHTTPToolsSQL, after, agentToolRepairBatch).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		touched := map[string]bool{}
		for _, row := range rows {
			after = row.ID
			canonical, ok := canonicalHTTPConfig(row)
			if !ok {
				continue
			}
			if err := tx.Exec("UPDATE agent_tools SET config = ? WHERE id = ?", canonical, row.ID).Error; err != nil {
				return err
			}
			touched[row.AgentID] = true
		}
		for agentID := range touched {
			if err := remirrorAgentTools(tx, agentID); err != nil {
				return err
			}
		}
	}
}

func canonicalHTTPConfig(row legacyHTTPToolRow) (string, bool) {
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(row.Config), &config); err != nil {
		slog.Error("agent.http_config_repair.unreadable", slog.String("tool_id", row.ID), slog.String("error", err.Error()))
		return "", false
	}
	canonical, err := agenttool.CanonicalHTTPConfig(config)
	if err != nil {
		slog.Error("agent.http_config_repair.unconvertible", slog.String("tool_id", row.ID), slog.String("error", err.Error()))
		return "", false
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

func remirrorAgentTools(tx *gorm.DB, agentID string) error {
	var rows []struct {
		Type       string
		Config     string
		Visibility string
	}
	if err := tx.Raw(liveAgentToolsSQL, agentID).Scan(&rows).Error; err != nil {
		return err
	}
	bindings := make([]agent.ToolBinding, 0, len(rows))
	for _, row := range rows {
		b := agent.ToolBinding{Name: row.Type}
		_ = json.Unmarshal([]byte(row.Config), &b.Config)
		_ = json.Unmarshal([]byte(row.Visibility), &b.Visibility)
		bindings = append(bindings, b)
	}
	return tx.Exec("UPDATE agents SET tool_bindings = ? WHERE id = ?", agent.EncodeLegacyBindings(bindings), agentID).Error
}
