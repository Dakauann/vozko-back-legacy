package balance_repository

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"vozko/domain/balance"
	"vozko/infra/database/schema"
	"vozko/infra/database/sqlexpr"
)

var errChargeCampaignUnresolved = errors.New("balance: the campaign of a WhatsApp campaign charge could not be resolved")

var chargeCampaignOfReferenceSQL = `SELECT ` + sqlexpr.ChargeCampaignOf("movement.reference_id") + ` FROM (SELECT CAST(? AS text) AS reference_id) movement`

func stampChargeCampaign(tx *gorm.DB, movement *schema.BalanceTransaction) error {
	if movement.ServiceType != string(balance.ServiceWhatsAppCampaign) {
		return nil
	}
	var campaignID string
	if err := tx.Raw(chargeCampaignOfReferenceSQL, movement.ReferenceID).Scan(&campaignID).Error; err != nil {
		return err
	}
	if campaignID == "" {
		return errChargeCampaignUnresolved
	}
	movement.ChargeCampaignID = &campaignID
	return nil
}

const templateChargeCategoryExpr = `CASE
				WHEN position('marketing' IN lower(bt.description)) > 0 THEN 'MARKETING'
				WHEN position('authentication' IN lower(bt.description)) > 0 THEN 'AUTHENTICATION'
				WHEN position('utility' IN lower(bt.description)) > 0 THEN 'UTILITY'
				ELSE 'UNKNOWN'
			END`

type chargeQuery struct {
	sql  string
	args []interface{}
}

type templateChargeRow struct {
	Category   string
	Net        int64
	Unresolved int64
}

func campaignScoped(filter balance.WhatsAppChargeFilter) bool {
	return strings.TrimSpace(filter.CampaignType) != "" || len(filter.DepartmentIDs) > 0
}

func campaignScope(filter balance.WhatsAppChargeFilter, ws string) (string, []interface{}) {
	conds := []string{`c.workspace_id = ?`, `c.deleted_at IS NULL`}
	args := []interface{}{ws}
	if t := strings.TrimSpace(filter.CampaignType); t != "" {
		conds = append(conds, `c.type = ?`)
		args = append(args, t)
	}
	if len(filter.DepartmentIDs) > 0 {
		conds = append(conds, `c.department_id IN ?`)
		args = append(args, filter.DepartmentIDs)
	}
	return strings.Join(conds, ` AND `), args
}

func templateChargeRowsWhere(filter balance.WhatsAppChargeFilter, ws string) (string, []interface{}) {
	where := `bt.workspace_id = ?
		  AND bt.service_type = ?
		  AND ` + templateSendRowsFilter
	args := []interface{}{ws, string(balance.ServiceWhatsAppCampaign)}
	if filter.From != nil {
		where += ` AND bt.created_at >= ?`
		args = append(args, *filter.From)
	}
	if filter.To != nil {
		where += ` AND bt.created_at <= ?`
		args = append(args, *filter.To)
	}
	return where, args
}

func chargesByOwner(filter balance.WhatsAppChargeFilter, ws string) chargeQuery {
	where, args := templateChargeRowsWhere(filter, ws)
	sql := `
		SELECT ` + templateChargeCategoryExpr + ` AS category,
			COALESCE(SUM(` + netTemplateSendExpr + `), 0)::bigint AS net
		FROM balance_transactions bt
		WHERE ` + where
	if campaignScoped(filter) {
		whereCamp, campArgs := campaignScope(filter, ws)
		sql += `
		  AND ` + chargeOwnerKey + ` IN (
		    SELECT c.id::text FROM whatsapp_campaigns c WHERE ` + whereCamp + `
		    UNION ALL
		    SELECT e.id::text FROM whatsapp_campaign_entries e
		    JOIN whatsapp_campaigns c ON c.id = e.campaign_id
		    WHERE e.deleted_at IS NULL AND ` + whereCamp + `
		    UNION ALL
		    SELECT 'waba:' || s.id FROM whatsapp_template_sends s
		    JOIN whatsapp_campaigns c ON c.id = s.campaign_id
		    WHERE s.deleted_at IS NULL AND ` + whereCamp + `
		  )
		`
		for i := 0; i < 3; i++ {
			args = append(args, campArgs...)
		}
	}
	sql += ` GROUP BY 1`
	return chargeQuery{sql: sql, args: args}
}

func chargesByCampaign(filter balance.WhatsAppChargeFilter, ws string) chargeQuery {
	whereCamp, args := campaignScope(filter, ws)
	where, rowArgs := templateChargeRowsWhere(filter, ws)
	sql := `
		WITH matched AS MATERIALIZED (
			SELECT c.id FROM whatsapp_campaigns c WHERE ` + whereCamp + `
		),
		gone AS MATERIALIZED (
			SELECT e.campaign_id, e.id::text AS owner_key FROM whatsapp_campaign_entries e
			WHERE e.deleted_at IS NOT NULL AND e.campaign_id IN (SELECT id FROM matched)
			UNION ALL
			SELECT s.campaign_id, 'waba:' || s.id FROM whatsapp_template_sends s
			WHERE s.deleted_at IS NOT NULL AND s.campaign_id IN (SELECT id FROM matched)
		)
		SELECT ` + templateChargeCategoryExpr + ` AS category,
			COALESCE(SUM(CASE
				WHEN bt.charge_campaign_id IN (SELECT id FROM matched)
					AND NOT COALESCE(bt.charge_campaign_id IN (SELECT campaign_id FROM gone) AND ` + chargeOwnerKey + ` IN (SELECT owner_key FROM gone), false)
				THEN ` + netTemplateSendExpr + `
				ELSE 0
			END), 0)::bigint AS net,
			COUNT(*) FILTER (WHERE bt.charge_campaign_id IS NULL) AS unresolved
		FROM balance_transactions bt
		WHERE ` + where + `
		GROUP BY 1`
	return chargeQuery{sql: sql, args: append(args, rowArgs...)}
}

func scanTemplateCharges(tx *gorm.DB, q chargeQuery) ([]templateChargeRow, error) {
	var rows []templateChargeRow
	if err := tx.Raw(q.sql, q.args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func anyUnresolved(rows []templateChargeRow) bool {
	for _, row := range rows {
		if row.Unresolved > 0 {
			return true
		}
	}
	return false
}

func templateCharges(tx *gorm.DB, filter balance.WhatsAppChargeFilter, ws string) ([]templateChargeRow, error) {
	if !campaignScoped(filter) {
		return scanTemplateCharges(tx, chargesByOwner(filter, ws))
	}
	rows, err := scanTemplateCharges(tx, chargesByCampaign(filter, ws))
	if err != nil {
		return nil, err
	}
	if !anyUnresolved(rows) {
		return rows, nil
	}
	return scanTemplateCharges(tx, chargesByOwner(filter, ws))
}

func foldTemplateCharges(rows []templateChargeRow) *balance.WhatsAppChargeStats {
	stats := &balance.WhatsAppChargeStats{}
	var total int64
	for _, rw := range rows {
		n := rw.Net
		if n < 0 {
			n = 0
		}
		switch strings.ToUpper(strings.TrimSpace(rw.Category)) {
		case "MARKETING":
			stats.Marketing = n
			total += n
		case "UTILITY":
			stats.Utility = n
			total += n
		case "AUTHENTICATION":
			stats.Authentication = n
			total += n
		default:
			total += n
		}
	}
	stats.NetDispatches = total
	return stats
}
