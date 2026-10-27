package automationneeds_repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/infra/database/schema"
	"vozko/infra/database/sqlexpr"
)

type slotTable struct {
	kind  domain.SlotKind
	table string
	name  string
}

var slotTables = []slotTable{
	{domain.SlotUnofficial, "unofficial_whatsapp_instances", "COALESCE(NULLIF(display_name, ''), NULLIF(profile_name, ''), phone_number, '')"},
	{domain.SlotInstagram, "instagram_accounts", "COALESCE(NULLIF(username, ''), name, '')"},
	{domain.SlotFacebook, "facebook_pages", "COALESCE(NULLIF(name, ''), username, '')"},
	{domain.SlotTelegram, "telegram_accounts", "COALESCE(NULLIF(bot_username, ''), bot_name, '')"},
	{domain.SlotTikTok, "tiktok_accounts", "COALESCE(NULLIF(username, ''), display_name, '')"},
	{domain.SlotMercadoLivre, "mercadolivre_accounts", "COALESCE(NULLIF(display_name, ''), nickname, '')"},
	{domain.SlotWebchat, "webchat_widgets", "name"},
	{domain.SlotEmail, "email_inboxes", "name"},
	{domain.SlotRCS, "rcs_senders", "brand_name"},
	{domain.SlotSMS, "sms_routes", "COALESCE(NULLIF(number, ''), brand_prefix)"},
}

func tableOf(kind domain.SlotKind) (slotTable, bool) {
	for _, t := range slotTables {
		if t.kind == kind {
			return t, true
		}
	}
	return slotTable{}, false
}

const enabledPredicate = "((enable_agent_responses AND agent_id IS NOT NULL) OR (enable_workflow AND workflow_id IS NOT NULL))"

func channelSlotsSQL() string {
	branches := make([]string, 0, len(slotTables))
	for _, t := range slotTables {
		branches = append(branches, "SELECT "+sqlexpr.Literal(string(t.kind))+" AS kind, CAST(id AS text) AS id, "+t.name+" AS name, "+
			"COALESCE(CAST(agent_id AS text), '') AS agent_id, COALESCE(CAST(workflow_id AS text), '') AS workflow_id, enable_agent_responses, enable_workflow "+
			"FROM "+t.table+" WHERE workspace_id = @workspace AND deleted_at IS NULL AND "+enabledPredicate)
	}
	return strings.Join(branches, " UNION ALL ")
}

const officialSlotsSQL = `SELECT kind, id, name, agent_id, workflow_id, enable_agent_responses, enable_workflow FROM (
	SELECT DISTINCT ON (wc.business_phone_id) 'whatsapp_number' AS kind, CAST(wc.business_phone_id AS text) AS id,
		COALESCE(NULLIF(bp.display_phone_number, ''), NULLIF(bp.verified_name, ''), wc.name) AS name,
		COALESCE(CAST(wc.agent_id AS text), '') AS agent_id, COALESCE(CAST(wc.workflow_id AS text), '') AS workflow_id,
		wc.enable_agent_responses, wc.enable_workflow
	FROM whatsapp_campaigns wc
	LEFT JOIN whatsapp_business_phone_numbers bp ON CAST(bp.id AS text) = CAST(wc.business_phone_id AS text)
	WHERE wc.workspace_id = @workspace AND wc.type = 'organic' AND wc.archived = false AND wc.deleted_at IS NULL AND wc.business_phone_id IS NOT NULL
	ORDER BY wc.business_phone_id, wc.created_at DESC, wc.id DESC
) latest WHERE ((enable_agent_responses AND agent_id <> '') OR (enable_workflow AND workflow_id <> ''))`

type slotRow struct {
	Kind                 string
	ID                   string
	Name                 string
	AgentID              string
	WorkflowID           string
	EnableAgentResponses bool
	EnableWorkflow       bool
}

func (r slotRow) slot(workspaceID string) domain.Slot {
	agentID, workflowID := r.AgentID, r.WorkflowID
	return domain.Slot{
		Kind: domain.SlotKind(r.Kind), ID: r.ID, Name: r.Name, WorkspaceID: workspaceID,
		Running: domain.RunningOf(conversation.ChannelAutomation{
			AgentID: &agentID, WorkflowID: &workflowID, EnableAgentResponses: r.EnableAgentResponses, EnableWorkflow: r.EnableWorkflow,
		}),
	}
}

type Slots struct {
	db *gorm.DB
}

func NewSlots(db *gorm.DB) *Slots {
	return &Slots{db: db}
}

func (s *Slots) InWorkspace(ctx context.Context, workspaceID string) ([]domain.Slot, error) {
	var rows []slotRow
	params := map[string]interface{}{"workspace": workspaceID}
	if err := s.db.WithContext(ctx).Raw(officialSlotsSQL, params).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("official receptive slots: %w", err)
	}
	var others []slotRow
	if err := s.db.WithContext(ctx).Raw(channelSlotsSQL(), params).Scan(&others).Error; err != nil {
		return nil, fmt.Errorf("channel receptive slots: %w", err)
	}
	rows = append(rows, others...)
	out := make([]domain.Slot, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.slot(workspaceID))
	}
	return out, nil
}

func flagOf(kind conversation.AutomationKind) (flag, column string) {
	if kind == conversation.AutomationWorkflow {
		return "enable_workflow", "workflow_id"
	}
	return "enable_agent_responses", "agent_id"
}

func SwitchOffIn(tx *gorm.DB, workspaceID string, offs []domain.SwitchOff, by string, now time.Time) ([]string, error) {
	var containers []string
	for _, off := range offs {
		flag, column := flagOf(off.Automation.Kind)
		if off.Slot.Kind == domain.SlotWhatsAppNumber {
			var ids []string
			err := tx.Raw("UPDATE whatsapp_campaigns SET "+flag+" = false, updated_at = ? WHERE workspace_id = ? AND CAST(business_phone_id AS text) = ? AND type = 'organic' AND archived = false AND deleted_at IS NULL AND CAST("+column+" AS text) = ? AND "+flag+" RETURNING CAST(id AS text)",
				now, workspaceID, off.Slot.ID, off.Automation.ID).Scan(&ids).Error
			if err != nil {
				return nil, fmt.Errorf("switch off number %s: %w", off.Slot.ID, err)
			}
			containers = append(containers, ids...)
		} else {
			table, ok := tableOf(off.Slot.Kind)
			if !ok {
				return nil, fmt.Errorf("switch off %s %s: unknown channel", off.Slot.Kind, off.Slot.ID)
			}
			if err := tx.Exec("UPDATE "+table.table+" SET "+flag+" = false, updated_at = ? WHERE CAST(id AS text) = ? AND workspace_id = ? AND deleted_at IS NULL AND CAST("+column+" AS text) = ?",
				now, off.Slot.ID, workspaceID, off.Automation.ID).Error; err != nil {
				return nil, fmt.Errorf("switch off %s %s: %w", off.Slot.Kind, off.Slot.ID, err)
			}
		}
		if err := tx.Model(&schema.AutomationSwitchOff{}).Where("workspace_id = ? AND slot_kind = ? AND slot_id = ? AND resolved_at IS NULL", workspaceID, string(off.Slot.Kind), off.Slot.ID).
			Update("resolved_at", now).Error; err != nil {
			return nil, fmt.Errorf("close the previous switch off of %s %s: %w", off.Slot.Kind, off.Slot.ID, err)
		}
		record := schema.AutomationSwitchOff{
			ID: uuid.NewString(), WorkspaceID: workspaceID, SlotKind: string(off.Slot.Kind), SlotID: off.Slot.ID, SlotName: off.Slot.Name,
			AutomationKind: string(off.Automation.Kind), AutomationID: off.Automation.ID, AutomationName: off.Automation.Name,
			Variables: pq.StringArray(off.Needs.Names()), SwitchedOffBy: by, SwitchedOffAt: now,
		}
		if err := tx.Create(&record).Error; err != nil {
			return nil, fmt.Errorf("record switch off of %s %s: %w", off.Slot.Kind, off.Slot.ID, err)
		}
	}
	return containers, nil
}

type SwitchOffRecords struct {
	db *gorm.DB
}

func NewSwitchOffRecords(db *gorm.DB) *SwitchOffRecords {
	return &SwitchOffRecords{db: db}
}

func (r *SwitchOffRecords) Open(ctx context.Context, workspaceID string, kind domain.SlotKind, slotID string) (*domain.SwitchOffRecord, error) {
	var rows []schema.AutomationSwitchOff
	err := r.db.WithContext(ctx).Where("workspace_id = ? AND slot_kind = ? AND slot_id = ? AND resolved_at IS NULL", workspaceID, string(kind), slotID).
		Order("switched_off_at DESC").Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	row := rows[0]
	return &domain.SwitchOffRecord{
		ID: row.ID, WorkspaceID: row.WorkspaceID, SlotKind: domain.SlotKind(row.SlotKind), SlotID: row.SlotID, SlotName: row.SlotName,
		Automation: domain.Link{Automation: conversation.Automation{Kind: conversation.AutomationKind(row.AutomationKind), ID: row.AutomationID}, Name: row.AutomationName},
		Variables:  []string(row.Variables), SwitchedOffBy: row.SwitchedOffBy, SwitchedOffAt: row.SwitchedOffAt, ResolvedAt: row.ResolvedAt,
	}, nil
}

func (r *SwitchOffRecords) Resolve(ctx context.Context, workspaceID string, kind domain.SlotKind, slotID string, at time.Time) (int, error) {
	res := r.db.WithContext(ctx).Model(&schema.AutomationSwitchOff{}).
		Where("workspace_id = ? AND slot_kind = ? AND slot_id = ? AND resolved_at IS NULL", workspaceID, string(kind), slotID).
		Update("resolved_at", at)
	return int(res.RowsAffected), res.Error
}
