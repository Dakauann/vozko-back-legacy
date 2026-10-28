package workflow_repository

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/buttonaction"
	mldomain "vozko/domain/mercadolivre"
	"vozko/domain/sitesubmission"
	"vozko/domain/workflow"
	"vozko/infra/database/sqlexpr"
)

type entryPointRepository struct {
	db *gorm.DB
}

func NewEntryPointRepository(db *gorm.DB) workflow.EntryPointReader {
	return &entryPointRepository{db: db}
}

const emptyJSONArray = "CAST('[]' AS jsonb)"

func jsonArrayOf(expr string) string {
	return "CASE WHEN jsonb_typeof(" + expr + ") = 'array' THEN " + expr + " ELSE " + emptyJSONArray + " END"
}

var publishedTriggersSQL = `SELECT v.workflow_id, CAST(n.node AS text) AS node
FROM workflows w
JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = w.published_version
CROSS JOIN LATERAL jsonb_array_elements(` + jsonArrayOf("v.graph -> 'nodes'") + `) WITH ORDINALITY AS n(node, position)
WHERE w.id IN ? AND starts_with(n.node ->> 'type', 'trigger_')
ORDER BY v.workflow_id, n.position`

const webhooksSQL = `SELECT workflow_id, active, auth_mode, method FROM workflow_webhooks WHERE workflow_id IN ?`

const lastDeliveriesSQL = `SELECT w.id AS workflow_id, d.received_at, d.outcome, d.reason
FROM workflows w
CROSS JOIN LATERAL (
	SELECT received_at, outcome, reason FROM workflow_webhook_deliveries
	WHERE workflow_id = w.id
	ORDER BY received_at DESC
	LIMIT 1
) d
WHERE w.id IN ?`

var lastStartsSQL = `SELECT w.id AS workflow_id, o.origin, r.started_at
FROM workflows w
CROSS JOIN (VALUES ` + originValues() + `) AS o(origin)
CROSS JOIN LATERAL (
	SELECT started_at FROM workflow_runs
	WHERE workflow_id = w.id AND ` + sqlexpr.WorkflowRunOrigin("") + ` = o.origin
	ORDER BY started_at DESC
	LIMIT 1
) r
WHERE w.id IN ?`

func originValues() string {
	values := make([]string, 0, len(workflow.RunOrigins()))
	for _, origin := range workflow.RunOrigins() {
		values = append(values, "("+sqlexpr.Literal(origin)+")")
	}
	return strings.Join(values, ", ")
}

const stagesSQL = `SELECT id, name FROM stages WHERE workspace_id = ? AND deleted_at IS NULL AND id IN ?`

const trunksSQL = `SELECT CAST(id AS text) AS id, name, enabled FROM sip_trunks WHERE workspace_id = ? AND deleted_at IS NULL AND id IN ?`

var activeTriggersSQL = `SELECT w.id AS workflow_id, w.name AS workflow_name, COALESCE(CAST(w.trigger_config AS text), '') AS legacy_config, CAST(n.node AS text) AS node
FROM workflows w
JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = w.published_version
CROSS JOIN LATERAL jsonb_array_elements(` + jsonArrayOf("v.graph -> 'nodes'") + `) WITH ORDINALITY AS n(node, position)
WHERE w.workspace_id = ? AND w.deleted_at IS NULL AND w.status = 'active' AND n.node ->> 'type' IN ?
ORDER BY w.name, w.id, n.position`

const formsSQL = `SELECT id, name, archived_at IS NULL AND published_version_id IS NOT NULL AS active
FROM whatsapp_forms WHERE workspace_id = ? AND id IN ?`

const pageFormsSQL = `SELECT k.form_key AS id, p.name || ': ' || k.form_id AS name,
EXISTS (SELECT 1 FROM site_form_manifests m WHERE m.form_key = k.form_key AND m.active) AS active
FROM site_form_keys k JOIN site_projects p ON p.id = k.project_id
WHERE k.workspace_id = ? AND k.form_key IN ?`

const agentBoundWorkflowSQL = `t.config ->> 'workflow_id'`

var agentRunWorkflowBindingsSQL = `FROM agent_tools t
JOIN agents a ON a.id = t.agent_id AND a.deleted_at IS NULL AND a.archived IS NOT TRUE
WHERE t.workspace_id = ? AND t.deleted_at IS NULL AND t.type = ?`

var boundAgentsSQL = `SELECT ` + agentBoundWorkflowSQL + ` AS workflow_id, a.id, a.name, COALESCE(a.is_active, false) AS active
` + agentRunWorkflowBindingsSQL + `
	AND ` + agentBoundWorkflowSQL + ` IN ?
ORDER BY a.name, a.id`

const templateExitWorkflowSQL = `params ->> 'workflowId'`

var templateExitsFromSQL = `FROM template_button_actions
WHERE workspace_id = ? AND kind = ` + sqlexpr.Literal(string(buttonaction.KindStartWorkflow))

var templateExitsSQL = `SELECT ` + templateExitWorkflowSQL + ` AS workflow_id, template_name, language, button_text
` + templateExitsFromSQL + ` AND ` + templateExitWorkflowSQL + ` IN ?
ORDER BY template_name, language, button_index`

const campaignExitWorkflowSQL = `e.route -> 'params' ->> 'workflowId'`

var campaignExitsFromSQL = `FROM whatsapp_campaign_button_routes r
JOIN whatsapp_campaigns c ON c.id = r.campaign_id AND c.deleted_at IS NULL AND c.archived IS NOT TRUE
CROSS JOIN LATERAL jsonb_array_elements(` + jsonArrayOf("r.routes") + `) AS e(route)
WHERE r.workspace_id = ? AND e.route ->> 'kind' = ` + sqlexpr.Literal(string(buttonaction.KindStartWorkflow))

var campaignExitsSQL = `SELECT ` + campaignExitWorkflowSQL + ` AS workflow_id, r.campaign_id, c.name, e.route ->> 'buttonText' AS button_text
` + campaignExitsFromSQL + ` AND ` + campaignExitWorkflowSQL + ` IN ?
ORDER BY c.name, r.campaign_id`

const listedWorkflowIDSQL = `CAST(workflows.id AS text)`

var runningGraphNodesSQL = jsonArrayOf(`(CASE WHEN workflows.published_version IS NULL THEN workflows.graph ELSE (
	SELECT v.graph FROM workflow_versions v WHERE v.workflow_id = workflows.id AND v.version = workflows.published_version
) END) -> 'nodes'`)

var runningTriggerExistsSQL = `EXISTS (SELECT 1 FROM jsonb_array_elements(` + runningGraphNodesSQL + `) AS n(node) WHERE n.node ->> 'type' IN ?)`

const activeWebhookExistsSQL = `EXISTS (SELECT 1 FROM workflow_webhooks wh WHERE wh.workflow_id = workflows.id AND wh.active)`

var activeAgentBindingExistsSQL = `EXISTS (SELECT 1 ` + agentRunWorkflowBindingsSQL + `
	AND COALESCE(a.is_active, false) AND ` + agentBoundWorkflowSQL + ` = ` + listedWorkflowIDSQL + `)`

var leadExitExistsSQL = `EXISTS (SELECT 1 ` + campaignExitsFromSQL + ` AND ` + campaignExitWorkflowSQL + ` = ` + listedWorkflowIDSQL + `)
	OR EXISTS (SELECT 1 ` + templateExitsFromSQL + ` AND ` + templateExitWorkflowSQL + ` = ` + listedWorkflowIDSQL + `)`

func entryKindFilter(workspaceID string, kind workflow.EntryPointKind) (string, []interface{}) {
	var clauses []string
	var args []interface{}
	if triggers := workflow.TriggerTypesOf(kind); len(triggers) > 0 {
		clauses = append(clauses, runningTriggerExistsSQL)
		args = append(args, triggers)
	}
	switch kind {
	case workflow.EntryWebhook:
		clauses = append(clauses, activeWebhookExistsSQL)
	case workflow.EntryAgentCall:
		clauses = append(clauses, activeAgentBindingExistsSQL)
		args = append(args, workspaceID, workflow.AgentRunWorkflowTool)
	case workflow.EntryLeadExit:
		clauses = append(clauses, leadExitExistsSQL)
		args = append(args, workspaceID, workspaceID)
	}
	if len(clauses) == 0 {
		return "FALSE", nil
	}
	return "(" + strings.Join(clauses, "\n\tOR ") + ")", args
}

type channelTable struct {
	kind     workflow.EntryChannelKind
	table    string
	name     string
	archived bool
}

var channelTables = []channelTable{
	{kind: workflow.ChannelWhatsAppCampaign, table: "whatsapp_campaigns", name: "name", archived: true},
	{kind: workflow.ChannelUnofficialWhatsAppCampaign, table: "unofficial_whatsapp_campaigns", name: "name", archived: true},
	{kind: workflow.ChannelUnofficialWhatsAppInstance, table: "unofficial_whatsapp_instances", name: "COALESCE(NULLIF(display_name, ''), NULLIF(profile_name, ''), phone_number)"},
	{kind: workflow.ChannelFacebookPage, table: "facebook_pages", name: "name"},
	{kind: workflow.ChannelInstagramAccount, table: "instagram_accounts", name: "COALESCE(NULLIF(username, ''), name)"},
	{kind: workflow.ChannelTelegramAccount, table: "telegram_accounts", name: "COALESCE(NULLIF(bot_name, ''), bot_username)"},
	{kind: workflow.ChannelTikTokAccount, table: "tiktok_accounts", name: "COALESCE(NULLIF(display_name, ''), username)"},
	{kind: workflow.ChannelMercadoLivreAccount, table: "mercadolivre_accounts", name: mldomain.AccountNameSQL("")},
	{kind: workflow.ChannelWebchatWidget, table: "webchat_widgets", name: "name"},
	{kind: workflow.ChannelEmailInbox, table: "email_inboxes", name: "COALESCE(NULLIF(name, ''), address)"},
	{kind: workflow.ChannelRCSSender, table: "rcs_senders", name: "COALESCE(NULLIF(brand_name, ''), id::text)"},
	{kind: workflow.ChannelSMSRoute, table: "sms_routes", name: "COALESCE(NULLIF(brand_prefix, ''), number)"},
}

func (c channelTable) selectLive() string {
	live := "SELECT " + sqlexpr.Literal(string(c.kind)) + " AS kind, CAST(id AS text) AS id, COALESCE(" + c.name + ", '') AS name, CAST(workflow_id AS text) AS workflow_id, enable_workflow, updated_at" +
		" FROM " + c.table + " WHERE workspace_id = ? AND deleted_at IS NULL"
	if c.archived {
		live += " AND archived IS NOT TRUE"
	}
	return live
}

func (c channelTable) linked() string {
	return c.selectLive() + " AND workflow_id IN ?"
}

var linkedChannelsSQL = linkedChannelUnion()

func linkedChannelUnion() string {
	linked := make([]string, len(channelTables))
	for i, c := range channelTables {
		linked[i] = c.linked()
	}
	return strings.Join(linked, "\nUNION ALL\n") + "\nORDER BY kind, name, id"
}

func linkedChannelArgs(workspaceID string, workflowIDs []string) []interface{} {
	var args []interface{}
	for range channelTables {
		args = append(args, workspaceID, workflowIDs)
	}
	return args
}

func pageFormKeysOf(ids []string) []string {
	var out []string
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if _, err := uuid.Parse(trimmed); err != nil && sitesubmission.ValidFormKey(trimmed) {
			out = append(out, trimmed)
		}
	}
	return out
}

func uuidsOf(ids []string) []string {
	var out []string
	for _, id := range ids {
		if _, err := uuid.Parse(strings.TrimSpace(id)); err == nil {
			out = append(out, strings.TrimSpace(id))
		}
	}
	return out
}

func (r *entryPointRepository) PublishedTriggers(workflowIDs []string) (map[string][]workflow.Node, error) {
	out := map[string][]workflow.Node{}
	ids := uuidsOf(workflowIDs)
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		WorkflowID string
		Node       string
	}
	if err := r.db.Raw(publishedTriggersSQL, ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		var node workflow.Node
		if err := json.Unmarshal([]byte(row.Node), &node); err != nil {
			return nil, fmt.Errorf("decode a published trigger of workflow %s: %w", row.WorkflowID, err)
		}
		out[row.WorkflowID] = append(out[row.WorkflowID], node)
	}
	return out, nil
}

func (r *entryPointRepository) EntryPointSources(q workflow.EntryPointSourceQuery) (*workflow.EntryPointSources, error) {
	ids := uuidsOf(q.WorkflowIDs)
	sources := &workflow.EntryPointSources{
		Webhooks:       map[string]workflow.WebhookEntry{},
		Forms:          map[string]workflow.EntryForm{},
		Agents:         map[string][]workflow.EntryAgent{},
		LeadExits:      map[string][]workflow.LeadExitEntry{},
		LastStarts:     map[string]map[workflow.EntryPointKind]time.Time{},
		LastDeliveries: map[string]workflow.EntryDelivery{},
		Stages:         map[string]string{},
		Trunks:         map[string]workflow.EntryTrunk{},
	}
	if len(ids) == 0 || strings.TrimSpace(q.WorkspaceID) == "" {
		return sources, nil
	}
	for _, read := range []func(workflow.EntryPointSourceQuery, []string, *workflow.EntryPointSources) error{
		r.webhooks, r.lastDeliveries, r.lastStarts, r.forms, r.agents, r.channels, r.leadExits, r.stages, r.trunks,
	} {
		if err := read(q, ids, sources); err != nil {
			return nil, err
		}
	}
	return sources, nil
}

func (r *entryPointRepository) webhooks(_ workflow.EntryPointSourceQuery, ids []string, s *workflow.EntryPointSources) error {
	var rows []struct {
		WorkflowID string
		Active     bool
		AuthMode   string
		Method     string
	}
	if err := r.db.Raw(webhooksSQL, ids).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		s.Webhooks[row.WorkflowID] = workflow.WebhookEntry{Active: row.Active, AuthMode: workflow.WebhookAuthMode(row.AuthMode), Method: row.Method}
	}
	return nil
}

func (r *entryPointRepository) lastDeliveries(_ workflow.EntryPointSourceQuery, ids []string, s *workflow.EntryPointSources) error {
	var rows []struct {
		WorkflowID string
		ReceivedAt time.Time
		Outcome    string
		Reason     string
	}
	if err := r.db.Raw(lastDeliveriesSQL, ids).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		s.LastDeliveries[row.WorkflowID] = workflow.EntryDelivery{At: row.ReceivedAt.UTC(), Outcome: workflow.WebhookOutcome(row.Outcome), Reason: row.Reason}
	}
	return nil
}

func (r *entryPointRepository) lastStarts(_ workflow.EntryPointSourceQuery, ids []string, s *workflow.EntryPointSources) error {
	var rows []struct {
		WorkflowID string
		Origin     string
		StartedAt  time.Time
	}
	if err := r.db.Raw(lastStartsSQL, ids).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		kind, ok := workflow.EntryKindOfRunOrigin(row.Origin)
		if !ok {
			continue
		}
		if s.LastStarts[row.WorkflowID] == nil {
			s.LastStarts[row.WorkflowID] = map[workflow.EntryPointKind]time.Time{}
		}
		if at := row.StartedAt.UTC(); at.After(s.LastStarts[row.WorkflowID][kind]) {
			s.LastStarts[row.WorkflowID][kind] = at
		}
	}
	return nil
}

func (r *entryPointRepository) forms(q workflow.EntryPointSourceQuery, _ []string, s *workflow.EntryPointSources) error {
	for _, read := range []struct {
		sql string
		ids []string
	}{{formsSQL, uuidsOf(q.FormIDs)}, {pageFormsSQL, pageFormKeysOf(q.FormIDs)}} {
		if len(read.ids) == 0 {
			continue
		}
		var rows []struct {
			ID     string
			Name   string
			Active bool
		}
		if err := r.db.Raw(read.sql, q.WorkspaceID, read.ids).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			s.Forms[row.ID] = workflow.EntryForm{ID: row.ID, Name: row.Name, Active: row.Active}
		}
	}
	return nil
}

func (r *entryPointRepository) agents(q workflow.EntryPointSourceQuery, ids []string, s *workflow.EntryPointSources) error {
	var rows []struct {
		WorkflowID string
		ID         string
		Name       string
		Active     bool
	}
	if err := r.db.Raw(boundAgentsSQL, q.WorkspaceID, workflow.AgentRunWorkflowTool, ids).Scan(&rows).Error; err != nil {
		return err
	}
	seen := map[[2]string]bool{}
	for _, row := range rows {
		key := [2]string{row.WorkflowID, row.ID}
		if seen[key] {
			continue
		}
		seen[key] = true
		s.Agents[row.WorkflowID] = append(s.Agents[row.WorkflowID], workflow.EntryAgent{ID: row.ID, Name: row.Name, Active: row.Active})
	}
	return nil
}

type channelRow struct {
	Kind           string
	ID             string
	Name           string
	WorkflowID     *string
	EnableWorkflow bool
}

func (row channelRow) workspaceChannel() workflow.WorkspaceChannel {
	c := workflow.WorkspaceChannel{EntryChannel: workflow.EntryChannel{Kind: workflow.EntryChannelKind(row.Kind), ID: row.ID, Name: row.Name, WorkflowEnabled: row.EnableWorkflow}}
	if row.WorkflowID != nil {
		c.WorkflowID = *row.WorkflowID
	}
	return c
}

func (r *entryPointRepository) channels(q workflow.EntryPointSourceQuery, ids []string, s *workflow.EntryPointSources) error {
	var linked []channelRow
	if err := r.db.Raw(linkedChannelsSQL, linkedChannelArgs(q.WorkspaceID, ids)...).Scan(&linked).Error; err != nil {
		return err
	}
	for _, row := range linked {
		s.LinkedChannels = append(s.LinkedChannels, row.workspaceChannel())
	}
	return nil
}

func (r *entryPointRepository) leadExits(q workflow.EntryPointSourceQuery, ids []string, s *workflow.EntryPointSources) error {
	var campaigns []struct {
		WorkflowID string
		CampaignID string
		Name       string
		ButtonText string
	}
	if err := r.db.Raw(campaignExitsSQL, q.WorkspaceID, ids).Scan(&campaigns).Error; err != nil {
		return err
	}
	for _, row := range campaigns {
		s.LeadExits[row.WorkflowID] = append(s.LeadExits[row.WorkflowID], workflow.LeadExitEntry{Source: workflow.LeadExitFromCampaign, ID: row.CampaignID, Name: row.Name, Button: row.ButtonText})
	}
	var templates []struct {
		WorkflowID   string
		TemplateName string
		Language     string
		ButtonText   string
	}
	if err := r.db.Raw(templateExitsSQL, q.WorkspaceID, ids).Scan(&templates).Error; err != nil {
		return err
	}
	for _, row := range templates {
		s.LeadExits[row.WorkflowID] = append(s.LeadExits[row.WorkflowID], workflow.LeadExitEntry{Source: workflow.LeadExitFromTemplate, Name: row.TemplateName, Language: row.Language, Button: row.ButtonText})
	}
	return nil
}

func nonBlank(ids []string) []string {
	var out []string
	for _, id := range ids {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func (r *entryPointRepository) stages(q workflow.EntryPointSourceQuery, _ []string, s *workflow.EntryPointSources) error {
	stageIDs := nonBlank(q.StageIDs)
	if len(stageIDs) == 0 {
		return nil
	}
	var rows []struct {
		ID   string
		Name string
	}
	if err := r.db.Raw(stagesSQL, q.WorkspaceID, stageIDs).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		s.Stages[row.ID] = row.Name
	}
	return nil
}

func (r *entryPointRepository) trunks(q workflow.EntryPointSourceQuery, _ []string, s *workflow.EntryPointSources) error {
	trunkIDs := uuidsOf(q.TrunkIDs)
	if len(trunkIDs) == 0 {
		return nil
	}
	var rows []struct {
		ID      string
		Name    string
		Enabled bool
	}
	if err := r.db.Raw(trunksSQL, q.WorkspaceID, trunkIDs).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		s.Trunks[row.ID] = workflow.EntryTrunk{Name: row.Name, Enabled: row.Enabled}
	}
	return nil
}

func (r *entryPointRepository) ActiveTriggers(workspaceID string, types []workflow.TriggerType) ([]workflow.ActiveTrigger, error) {
	if strings.TrimSpace(workspaceID) == "" || len(types) == 0 {
		return nil, nil
	}
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	var rows []struct {
		WorkflowID   string
		WorkflowName string
		LegacyConfig string
		Node         string
	}
	if err := r.db.Raw(activeTriggersSQL, workspaceID, names).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]workflow.ActiveTrigger, 0, len(rows))
	for _, row := range rows {
		trigger := workflow.ActiveTrigger{WorkflowID: row.WorkflowID, WorkflowName: row.WorkflowName}
		if err := json.Unmarshal([]byte(row.Node), &trigger.Node); err != nil {
			return nil, fmt.Errorf("decode an active trigger of workflow %s: %w", row.WorkflowID, err)
		}
		if strings.TrimSpace(row.LegacyConfig) != "" {
			if err := json.Unmarshal([]byte(row.LegacyConfig), &trigger.LegacyConfig); err != nil {
				return nil, fmt.Errorf("decode the trigger config of workflow %s: %w", row.WorkflowID, err)
			}
		}
		out = append(out, trigger)
	}
	return out, nil
}
