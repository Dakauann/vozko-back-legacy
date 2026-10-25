package database

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/agenttool"
)

const mcpSourceToolType = "mcp_source"

const agentsWithMCPCollectionsSQL = `SELECT DISTINCT ac.agent_id FROM agent_mcp_collections ac
JOIN agents a ON a.id = ac.agent_id AND a.deleted_at IS NULL
WHERE ac.agent_id > ?::uuid
ORDER BY ac.agent_id LIMIT ?`

const collectionSourcesSQL = `SELECT DISTINCT a.id AS agent_id, a.workspace_id, m.kind, m.ref_id, COALESCE(r.name, '') AS remote_name
FROM agents a
JOIN agent_mcp_collections ac ON ac.agent_id = a.id
JOIN mcp_collections c ON c.id = ac.collection_id AND c.deleted_at IS NULL AND c.workspace_id = a.workspace_id
JOIN mcp_collection_members m ON m.collection_id = c.id
LEFT JOIN mcp_remote_servers r ON m.kind = 'remote' AND r.id::text = m.ref_id AND r.deleted_at IS NULL
WHERE a.id IN ?
ORDER BY a.id, m.kind, m.ref_id`

const liveToolNamesSQL = `SELECT lower(name) AS name FROM agent_tools WHERE agent_id = ? AND deleted_at IS NULL`

const nextToolPositionSQL = `SELECT COALESCE(MAX(position) + 1, 0) FROM agent_tools WHERE agent_id = ? AND deleted_at IS NULL`

const insertMCPInstanceSQL = `WITH inserted AS (
	INSERT INTO agent_tools (id, workspace_id, agent_id, type, name, description, config, visibility, position, revision, target_key, created_by, updated_by, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, '[]', ?, 1, ?, '', '', now(), now())
	ON CONFLICT DO NOTHING
	RETURNING id, workspace_id, agent_id, type, name, description, config, visibility
)
INSERT INTO agent_tool_revisions (tool_id, revision, workspace_id, agent_id, type, name, description, config, visibility, saved_by, saved_at)
SELECT id, 1, workspace_id, agent_id, type, name, description, config, visibility, '', now() FROM inserted
ON CONFLICT (tool_id, revision) DO NOTHING`

type collectionSourceRow struct {
	AgentID     string
	WorkspaceID string
	Kind        string
	RefID       string
	RemoteName  string
}

func (r collectionSourceRow) sourceID() string {
	return r.Kind + ":" + r.RefID
}

func (r collectionSourceRow) label() string {
	if strings.TrimSpace(r.RemoteName) != "" {
		return r.RemoteName
	}
	return r.RefID
}

func copyAgentMCPInstances(tx *gorm.DB) error {
	for _, table := range []string{"agent_tools", "agent_tool_revisions", "agent_mcp_collections", "mcp_collections", "mcp_collection_members", "mcp_remote_servers"} {
		if !tx.Migrator().HasTable(table) {
			return nil
		}
	}
	after := workflowVersionRepairFirstKey
	for {
		var agentIDs []string
		if err := tx.Raw(agentsWithMCPCollectionsSQL, after, agentToolRepairBatch).Scan(&agentIDs).Error; err != nil {
			return err
		}
		if len(agentIDs) == 0 {
			return nil
		}
		after = agentIDs[len(agentIDs)-1]
		var rows []collectionSourceRow
		if err := tx.Raw(collectionSourcesSQL, agentIDs).Scan(&rows).Error; err != nil {
			return err
		}
		byAgent := map[string][]collectionSourceRow{}
		for _, row := range rows {
			byAgent[row.AgentID] = append(byAgent[row.AgentID], row)
		}
		for _, agentID := range agentIDs {
			if err := copyMCPSourcesOf(tx, agentID, byAgent[agentID]); err != nil {
				return fmt.Errorf("agent %s: %w", agentID, err)
			}
		}
	}
}

func copyMCPSourcesOf(tx *gorm.DB, agentID string, sources []collectionSourceRow) error {
	if len(sources) == 0 {
		return nil
	}
	var names []string
	if err := tx.Raw(liveToolNamesSQL, agentID).Scan(&names).Error; err != nil {
		return err
	}
	taken := map[string]bool{}
	for _, name := range names {
		taken[name] = true
	}
	var position int
	if err := tx.Raw(nextToolPositionSQL, agentID).Scan(&position).Error; err != nil {
		return err
	}
	for _, source := range sources {
		name := mcpInstanceName(source.label(), taken)
		config, err := json.Marshal(map[string]interface{}{"source_id": source.sourceID(), "all_tools": true, "tools": []string{}})
		if err != nil {
			return err
		}
		id := uuid.NewSHA1(agentToolInstanceNamespace, []byte(agentID+"\x00"+mcpSourceToolType+"\x00"+source.sourceID())).String()
		description := fmt.Sprintf("Ferramentas do servidor MCP %s.", source.label())
		result := tx.Exec(insertMCPInstanceSQL, id, source.WorkspaceID, agentID, mcpSourceToolType, name, description, string(config), position, "target:"+source.sourceID())
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			taken[name] = true
			position++
		}
	}
	return nil
}

func mcpInstanceName(label string, taken map[string]bool) string {
	base := "mcp_" + agenttool.Slug(label)
	if len(base) > 60 {
		base = strings.TrimRight(base[:60], "_")
	}
	if base == "mcp_" {
		base = "mcp_servidor"
	}
	name := base
	for n := 2; taken[name]; n++ {
		name = fmt.Sprintf("%s_%d", base, n)
	}
	return name
}
