package database

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"vozko/domain/agent"
	"vozko/domain/variabledecl"
)

const agentsWithPlatformVariablesSQL = `SELECT id, variables::text AS variables FROM agents
WHERE id > ?::uuid
AND EXISTS (SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(variables) = 'array' THEN variables ELSE '[]'::jsonb END) v WHERE btrim(v->>'name') IN ?)
ORDER BY id LIMIT ?`

const versionsWithPlatformVariablesSQL = `SELECT id, agent_id, version, (snapshot->'variables')::text AS variables FROM agent_versions
WHERE id > ?::uuid
AND EXISTS (SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(snapshot->'variables') = 'array' THEN snapshot->'variables' ELSE '[]'::jsonb END) v WHERE btrim(v->>'name') IN ?)
ORDER BY id LIMIT ?`

const updateAgentVariablesSQL = `UPDATE agents SET variables = ?::jsonb WHERE id = ?`

const updateVersionVariablesSQL = `UPDATE agent_versions SET snapshot = jsonb_set(snapshot, '{variables}', ?::jsonb) WHERE id = ?`

type platformVariablesRow struct {
	ID        string
	AgentID   string
	Version   int
	Variables string
}

func removePlatformAgentVariables(tx *gorm.DB) error {
	for _, table := range []string{"agents", "agent_versions"} {
		if !tx.Migrator().HasTable(table) {
			return nil
		}
	}
	reserved := variabledecl.SystemKeys()
	if err := repairPlatformVariables(tx, agentsWithPlatformVariablesSQL, updateAgentVariablesSQL, reserved, func(row platformVariablesRow, removed []string) {
		slog.Info("agent.variables.platform_names_removed", slog.String("agent_id", row.ID), slog.String("names", strings.Join(removed, ",")))
	}); err != nil {
		return fmt.Errorf("agents: %w", err)
	}
	if err := repairPlatformVariables(tx, versionsWithPlatformVariablesSQL, updateVersionVariablesSQL, reserved, func(row platformVariablesRow, removed []string) {
		slog.Info("agent.version.variables.platform_names_removed", slog.String("agent_id", row.AgentID), slog.Int("version", row.Version), slog.String("names", strings.Join(removed, ",")))
	}); err != nil {
		return fmt.Errorf("agent versions: %w", err)
	}
	return nil
}

func repairPlatformVariables(tx *gorm.DB, selectSQL, updateSQL string, reserved []string, logRemoved func(platformVariablesRow, []string)) error {
	after := workflowVersionRepairFirstKey
	for {
		var rows []platformVariablesRow
		if err := tx.Raw(selectSQL, after, reserved, agentToolRepairBatch).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		after = rows[len(rows)-1].ID
		for _, row := range rows {
			var vars []agent.AgentVariable
			if err := json.Unmarshal([]byte(row.Variables), &vars); err != nil {
				return fmt.Errorf("%s: unreadable variables: %w", row.ID, err)
			}
			kept, removed := agent.WithoutPlatformVariables(vars)
			if len(removed) == 0 {
				return fmt.Errorf("%s: the query found platform names the domain rule keeps", row.ID)
			}
			encoded, err := json.Marshal(kept)
			if err != nil {
				return err
			}
			if err := tx.Exec(updateSQL, string(encoded), row.ID).Error; err != nil {
				return fmt.Errorf("%s: %w", row.ID, err)
			}
			logRemoved(row, removed)
		}
	}
}
