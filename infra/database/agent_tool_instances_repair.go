package database

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/agent"
	"vozko/domain/tools"
)

const agentToolRepairBatch = 200

var agentToolInstanceNamespace = uuid.MustParse("6f0a7c1e-4b5d-5e9a-8c2f-3d1b9e7a4c60")

var legacyBoundTypes = map[string]string{
	"run_workflow":  "workflow_id",
	"add_sheet_row": "sheet_id",
}

var legacyManyTypes = map[string]bool{"http_request": true}

var retiredToolTypes = map[string]bool{"query_knowledge_base": true, "conversation_analysis": true}

const agentsWithLegacyToolsSQL = `SELECT id, workspace_id, tool_bindings FROM agents
WHERE id > ?::uuid AND deleted_at IS NULL AND tool_bindings IS NOT NULL
	AND NOT EXISTS (SELECT 1 FROM agent_tools t WHERE t.agent_id = agents.id)
ORDER BY id LIMIT ?`

const insertAgentToolSQL = `INSERT INTO agent_tools (id, workspace_id, agent_id, type, name, description, config, visibility, position, revision, target_key, created_by, updated_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, '', ?, ?, ?, 1, ?, '', '', now(), now())
ON CONFLICT (id) DO NOTHING`

const insertAgentToolRevisionSQL = `INSERT INTO agent_tool_revisions (tool_id, revision, workspace_id, agent_id, type, name, description, config, visibility, saved_by, saved_at)
VALUES (?, 1, ?, ?, ?, ?, '', ?, ?, '', now())
ON CONFLICT (tool_id, revision) DO NOTHING`

type legacyToolsRow struct {
	ID           string
	WorkspaceID  string
	ToolBindings string
}

type legacyInstance struct {
	ID         string
	Type       string
	Name       string
	Config     string
	Visibility string
	Position   int
	TargetKey  string
}

func copyAgentToolInstances(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("agent_tools") || !tx.Migrator().HasTable("agent_tool_revisions") {
		return nil
	}
	after := workflowVersionRepairFirstKey
	for {
		var rows []legacyToolsRow
		if err := tx.Raw(agentsWithLegacyToolsSQL, after, agentToolRepairBatch).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			after = row.ID
			for _, inst := range legacyInstancesOf(row.ID, row.ToolBindings) {
				if err := tx.Exec(insertAgentToolSQL, inst.ID, row.WorkspaceID, row.ID, inst.Type, inst.Name, inst.Config, inst.Visibility, inst.Position, inst.TargetKey).Error; err != nil {
					return fmt.Errorf("agent %s tool %s: %w", row.ID, inst.Name, err)
				}
				if err := tx.Exec(insertAgentToolRevisionSQL, inst.ID, row.WorkspaceID, row.ID, inst.Type, inst.Name, inst.Config, inst.Visibility).Error; err != nil {
					return fmt.Errorf("agent %s tool %s revision: %w", row.ID, inst.Name, err)
				}
			}
		}
	}
}

func legacyInstancesOf(agentID, raw string) []legacyInstance {
	seen := map[string]bool{}
	var out []legacyInstance
	for _, b := range agent.DecodeLegacyBindings(raw) {
		toolType := strings.ToLower(strings.TrimSpace(b.Name))
		if toolType == "" || retiredToolTypes[toolType] {
			continue
		}
		name, targetKey := legacyIdentity(toolType, b.Config)
		if name == "" {
			slog.Warn("agent.tool_instances_repair.unnamed", slog.String("agent_id", agentID), slog.String("type", toolType))
			continue
		}
		if seen[name] {
			slog.Warn("agent.tool_instances_repair.duplicate", slog.String("agent_id", agentID), slog.String("name", name))
			continue
		}
		seen[name] = true
		config, _ := json.Marshal(nonNilConfig(b.Config))
		visibility, _ := json.Marshal(nonNilVisibility(b.Visibility))
		out = append(out, legacyInstance{
			ID:         uuid.NewSHA1(agentToolInstanceNamespace, []byte(agentID+"\x00"+name)).String(),
			Type:       toolType,
			Name:       name,
			Config:     string(config),
			Visibility: string(visibility),
			Position:   len(out),
			TargetKey:  targetKey,
		})
	}
	return out
}

func legacyIdentity(toolType string, config map[string]interface{}) (string, string) {
	if key, bound := legacyBoundTypes[toolType]; bound {
		target, _ := config[key].(string)
		target = strings.TrimSpace(target)
		return tools.BoundToolName(toolType, target), "target:" + target
	}
	if legacyManyTypes[toolType] {
		return toolType, ""
	}
	return toolType, "single"
}

func nonNilConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return map[string]interface{}{}
	}
	return config
}

func nonNilVisibility(visibility []agent.ToolVisibility) []agent.ToolVisibility {
	if visibility == nil {
		return []agent.ToolVisibility{}
	}
	return visibility
}

func dropAgentBindingsIndex(tx *gorm.DB) error {
	return tx.Exec("DROP INDEX IF EXISTS idx_agents_run_workflow_bindings").Error
}
