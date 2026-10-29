package variabledecl_repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"vozko/domain/agent"
	"vozko/domain/workflow"
)

const draftAgentsSQL = `SELECT id, name, COALESCE(department_id::text, '') AS department_id, messaging_prompt AS prompt, COALESCE(variables::text, '') AS body
FROM agents WHERE workspace_id = ? AND deleted_at IS NULL`

const publishedAgentsSQL = `SELECT a.id, a.name, COALESCE(a.department_id::text, '') AS department_id, '' AS prompt, v.snapshot::text AS body
FROM agents a JOIN agent_versions v ON v.agent_id = a.id AND v.version = a.published_version
WHERE a.workspace_id = ? AND a.deleted_at IS NULL`

const agentToolsSQL = `SELECT t.agent_id AS id, t.config::text AS body FROM agent_tools t
JOIN agents a ON a.id = t.agent_id AND a.deleted_at IS NULL
WHERE t.workspace_id = ? AND t.type = ? AND t.deleted_at IS NULL`

const draftWorkflowsSQL = `SELECT id, name, COALESCE(department_id::text, '') AS department_id, COALESCE(graph::text, '') AS body
FROM workflows WHERE workspace_id = ? AND deleted_at IS NULL`

const publishedWorkflowsSQL = `SELECT w.id, w.name, COALESCE(w.department_id::text, '') AS department_id, v.graph::text AS body
FROM workflows w JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = w.published_version
WHERE w.workspace_id = ? AND w.deleted_at IS NULL`

type automationRow struct {
	ID           string
	Name         string
	DepartmentID string
	Prompt       string
	Body         string
}

type Automations struct {
	db *gorm.DB
}

func NewAutomations(db *gorm.DB) *Automations {
	return &Automations{db: db}
}

func (a *Automations) Automations(ctx context.Context, workspaceID string) ([]*agent.Agent, []*workflow.Workflow, error) {
	agents, err := a.agents(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	workflows, err := a.workflows(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	return agents, workflows, nil
}

func (a *Automations) rows(ctx context.Context, query string, args ...interface{}) ([]automationRow, error) {
	var rows []automationRow
	if err := a.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (a *Automations) agents(ctx context.Context, workspaceID string) ([]*agent.Agent, error) {
	drafts, err := a.rows(ctx, draftAgentsSQL, workspaceID)
	if err != nil {
		return nil, err
	}
	tools, err := a.rows(ctx, agentToolsSQL, workspaceID, workflow.AgentSheetRowTool)
	if err != nil {
		return nil, err
	}
	bindings := map[string][]agent.ToolBinding{}
	for _, tool := range tools {
		var config map[string]interface{}
		if err := json.Unmarshal([]byte(tool.Body), &config); err != nil {
			return nil, fmt.Errorf("agent %s: unreadable sheet tool: %w", tool.ID, err)
		}
		bindings[tool.ID] = append(bindings[tool.ID], agent.ToolBinding{Name: workflow.AgentSheetRowTool, Config: config})
	}
	out := make([]*agent.Agent, 0, len(drafts))
	for _, row := range drafts {
		var vars []agent.AgentVariable
		if strings.TrimSpace(row.Body) != "" {
			if err := json.Unmarshal([]byte(row.Body), &vars); err != nil {
				return nil, fmt.Errorf("agent %s: unreadable variables: %w", row.ID, err)
			}
		}
		out = append(out, &agent.Agent{ID: row.ID, Name: row.Name, DepartmentID: row.DepartmentID, MessagingPrompt: row.Prompt, Variables: vars, InternalTools: bindings[row.ID]})
	}
	published, err := a.rows(ctx, publishedAgentsSQL, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, row := range published {
		var snapshot agent.Snapshot
		if err := json.Unmarshal([]byte(row.Body), &snapshot); err != nil {
			return nil, fmt.Errorf("agent %s: unreadable published version: %w", row.ID, err)
		}
		out = append(out, &agent.Agent{ID: row.ID, Name: row.Name, DepartmentID: row.DepartmentID, MessagingPrompt: snapshot.MessagingPrompt, Variables: snapshot.Variables})
	}
	return out, nil
}

func (a *Automations) workflows(ctx context.Context, workspaceID string) ([]*workflow.Workflow, error) {
	var out []*workflow.Workflow
	for _, query := range []string{draftWorkflowsSQL, publishedWorkflowsSQL} {
		rows, err := a.rows(ctx, query, workspaceID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			var graph workflow.Graph
			if strings.TrimSpace(row.Body) != "" {
				if err := json.Unmarshal([]byte(row.Body), &graph); err != nil {
					return nil, fmt.Errorf("workflow %s: unreadable graph: %w", row.ID, err)
				}
			}
			out = append(out, &workflow.Workflow{ID: row.ID, Name: row.Name, DepartmentID: row.DepartmentID, Graph: graph})
		}
	}
	return out, nil
}
