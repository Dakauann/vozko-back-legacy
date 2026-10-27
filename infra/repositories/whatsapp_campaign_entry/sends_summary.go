package whatsapp_campaign_entry

import (
	"context"
	"sort"
	"strings"

	"gorm.io/gorm"

	"vozko/domain/campaign"
	"vozko/domain/conversation"
	wc "vozko/domain/whatsapp_campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
	"vozko/infra/database"
	"vozko/infra/database/sqlexpr"
	"vozko/infra/repositories/campaignstamp"
)

const (
	sendsSummaryTimeout = "30s"
	placedFilter        = "{placed}"
	undatedFilter       = "{undated}"
	windowFilter        = "{window}"
	departmentFilter    = "{departments}"
)

var campaignSendsSQL = `
SELECT e.status AS status, UPPER(TRIM(COALESCE(NULLIF(TRIM(e.metadata->'template_info'->>'category'), ''), t.category, ''))) AS category,
	COUNT(*) FILTER (WHERE ` + placedFilter + `) AS placed,
	COUNT(*) FILTER (WHERE ` + undatedFilter + `) AS undated
FROM whatsapp_campaign_entries e
JOIN whatsapp_campaigns c ON c.id = e.campaign_id
LEFT JOIN whatsapp_templates t ON t.id = c.template_id
WHERE c.workspace_id = @workspace AND c.deleted_at IS NULL AND e.deleted_at IS NULL
	AND COALESCE(c.type, '') <> @organic` + departmentFilter + `
GROUP BY 1, 2`

var oneOffSendsSQL = `
SELECT ` + oneOffStatus("s", "receipt.delivery_status") + ` AS status, UPPER(TRIM(COALESCE(s.category, ''))) AS category,
	COUNT(*) AS placed, 0 AS undated
FROM whatsapp_template_sends s
LEFT JOIN whatsapp_campaign_entries conversation_entry ON conversation_entry.id = s.entry_id
LEFT JOIN whatsapp_campaigns c ON c.id = COALESCE(s.campaign_id, conversation_entry.campaign_id)
LEFT JOIN LATERAL (
	SELECT m.delivery_status FROM conversation_messages m
	WHERE s.provider_message_id <> '' AND m.whatsapp_message_id = s.provider_message_id AND m.deleted_at IS NULL
	ORDER BY m.created_at LIMIT 1
) receipt ON true
WHERE s.workspace_id = @workspace AND s.deleted_at IS NULL AND s.status IN @attempts` + windowFilter + departmentFilter + `
GROUP BY 1, 2`

func oneOffStatus(attempt, receipt string) string {
	byReceipt := wce.OneOffReceiptStatuses()
	receipts := make([]string, 0, len(byReceipt))
	for r := range byReceipt {
		receipts = append(receipts, string(r))
	}
	sort.Strings(receipts)

	var b strings.Builder
	b.WriteString("CASE WHEN " + attempt + ".status IN (" + sqlexpr.LiteralList(attemptStrings(wce.OneOffFailedAttempts())) + ") THEN " + sqlexpr.Literal(string(wce.SendStatusFailed)))
	for _, r := range receipts {
		b.WriteString(" WHEN " + receipt + " = " + sqlexpr.Literal(r) + " THEN " + sqlexpr.Literal(string(byReceipt[conversation.DeliveryStatus(r)])))
	}
	b.WriteString(" ELSE " + sqlexpr.Literal(string(wce.SendStatusSent)) + " END")
	return b.String()
}

type sendsRow struct {
	Status   string
	Category string
	Placed   int64
	Undated  int64
}

func (r *repository) TallySends(f wce.WorkspaceSummaryFilter) (*campaign.SendsTally, error) {
	tally := &campaign.SendsTally{}
	if strings.TrimSpace(f.WorkspaceID) == "" {
		return tally, nil
	}

	campaignSQL, campaignArgs := campaignSendsQuery(f)
	oneOffSQL, oneOffArgs := oneOffSendsQuery(f)
	var campaignRows, oneOffRows []sendsRow
	err := database.InReadSession(context.Background(), r.db, []string{database.StatementTimeout(sendsSummaryTimeout)}, func(tx *gorm.DB) error {
		if err := tx.Raw(campaignSQL, campaignArgs).Scan(&campaignRows).Error; err != nil {
			return err
		}
		return tx.Raw(oneOffSQL, oneOffArgs).Scan(&oneOffRows).Error
	})
	if err != nil {
		return nil, err
	}

	for _, row := range campaignRows {
		tally.Add(campaign.SendSourceCampaign, campaign.SendStatus(row.Status), row.Category, row.Placed)
		tally.Undated += row.Undated
	}
	for _, row := range oneOffRows {
		tally.Add(campaign.SendSourceOneOff, campaign.SendStatus(row.Status), row.Category, row.Placed)
	}
	return tally, nil
}

func campaignSendsQuery(f wce.WorkspaceSummaryFilter) (string, map[string]interface{}) {
	args := map[string]interface{}{
		"workspace": f.WorkspaceID,
		"organic":   string(wc.CampaignTypeOrganic),
	}
	placed, undated := "TRUE", "FALSE"
	if f.Window.Bounded() {
		moment := campaignstamp.OutcomeMoment("e")
		placed = windowOn(moment, f.Window, args)
		undated = "e.status <> @pending AND " + moment + " IS NULL"
		args["pending"] = string(wce.SendStatusPending)
	}
	sql := strings.NewReplacer(
		placedFilter, placed,
		undatedFilter, undated,
		departmentFilter, departmentsOn(f.DepartmentIDs, args),
	).Replace(campaignSendsSQL)
	return sql, args
}

func oneOffSendsQuery(f wce.WorkspaceSummaryFilter) (string, map[string]interface{}) {
	args := map[string]interface{}{
		"workspace": f.WorkspaceID,
		"attempts":  attemptStrings(wce.OneOffDispatchedAttempts()),
	}
	window := ""
	if f.Window.Bounded() {
		window = " AND " + windowOn("s.created_at", f.Window, args)
	}
	sql := strings.NewReplacer(
		windowFilter, window,
		departmentFilter, departmentsOn(f.DepartmentIDs, args),
	).Replace(oneOffSendsSQL)
	return sql, args
}

func windowOn(moment string, w campaign.SendWindow, args map[string]interface{}) string {
	var bounds []string
	if w.From != nil {
		bounds = append(bounds, moment+" >= @from")
		args["from"] = w.From.UTC()
	}
	if w.To != nil {
		bounds = append(bounds, moment+" <= @to")
		args["to"] = w.To.UTC()
	}
	return strings.Join(bounds, " AND ")
}

func departmentsOn(departmentIDs []string, args map[string]interface{}) string {
	if len(departmentIDs) == 0 {
		return ""
	}
	args["departments"] = departmentIDs
	return " AND c.department_id IN @departments"
}

func attemptStrings[T ~string](statuses []T) []string {
	out := make([]string, len(statuses))
	for i, s := range statuses {
		out[i] = string(s)
	}
	return out
}
