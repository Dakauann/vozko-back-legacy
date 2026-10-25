package database

import (
	"fmt"

	"gorm.io/gorm"

	"vozko/domain/balance"
	"vozko/infra/database/sqlexpr"
)

const (
	moneyTotalsBackfillChunk = 500
	firstUUID                = "00000000-0000-0000-0000-000000000000"
)

type chargeCampaignBackfill struct {
	batch  int
	perRun int
}

var chargeCampaignBackfillPerBoot = chargeCampaignBackfill{batch: 5000, perRun: 200000}

func backfillBalanceReadModel(tx *gorm.DB) error {
	if err := backfillMoneyTotals(tx); err != nil {
		return fmt.Errorf("money totals: %w", err)
	}
	if err := backfillChargeCampaigns(tx, chargeCampaignBackfillPerBoot); err != nil {
		return fmt.Errorf("charge campaigns: %w", err)
	}
	return nil
}

const staleMoneyTotalsSQL = `SELECT id FROM balances WHERE money_totals_at IS DISTINCT FROM updated_at ORDER BY id FOR UPDATE`

const moneyTotalsBackfillSQL = `
UPDATE balances b SET
	total_money_credits = counted.credits,
	total_money_debits = counted.debits,
	money_totals_at = b.updated_at
FROM (
	SELECT l.id,
		COALESCE(SUM(bt.amount) FILTER (WHERE bt.type = 'credit'), 0) AS credits,
		COALESCE(SUM(bt.amount) FILTER (WHERE bt.type = 'debit'), 0) AS debits,
		COUNT(bt.id) FILTER (WHERE bt.type NOT IN ('credit', 'debit')) AS unplaced
	FROM balances l
	LEFT JOIN balance_transactions bt ON bt.balance_id = l.id AND bt.resource_type = 'money'
	WHERE l.id IN ?
	GROUP BY l.id
) counted
WHERE b.id = counted.id AND counted.unplaced = 0`

func backfillMoneyTotals(tx *gorm.DB) error {
	var stale []string
	if err := tx.Raw(staleMoneyTotalsSQL).Scan(&stale).Error; err != nil {
		return err
	}
	for start := 0; start < len(stale); start += moneyTotalsBackfillChunk {
		end := min(start+moneyTotalsBackfillChunk, len(stale))
		if err := tx.Exec(moneyTotalsBackfillSQL, stale[start:end]).Error; err != nil {
			return err
		}
	}
	return nil
}

var unresolvedChargeCampaignPredicate = `service_type = '` + string(balance.ServiceWhatsAppCampaign) + `' AND charge_campaign_id IS NULL`

var unresolvedChargeCampaignsSQL = `SELECT id FROM balance_transactions WHERE id > ? AND ` + unresolvedChargeCampaignPredicate + ` ORDER BY id LIMIT ?`

var chargeCampaignBackfillSQL = `UPDATE balance_transactions bt SET charge_campaign_id = ` + sqlexpr.ChargeCampaignOf("bt.reference_id") + ` WHERE bt.id IN ?`

func backfillChargeCampaigns(tx *gorm.DB, share chargeCampaignBackfill) error {
	after := firstUUID
	for left := share.perRun; left > 0; {
		var ids []string
		if err := tx.Raw(unresolvedChargeCampaignsSQL, after, min(share.batch, left)).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := tx.Exec(chargeCampaignBackfillSQL, ids).Error; err != nil {
			return err
		}
		after = ids[len(ids)-1]
		left -= len(ids)
	}
	return nil
}
