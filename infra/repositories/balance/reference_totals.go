package balance_repository

import (
	"gorm.io/gorm"

	"vozko/domain/balance"
	"vozko/infra/database/sqlexpr"
)

func NewReferenceTotals(db *gorm.DB) balance.ReferenceTotaler {
	return &BalanceRepositoryImpl{db: db}
}

const referenceTotalsSQL = `
SELECT
	COALESCE(SUM(CASE WHEN type = 'debit' THEN amount ELSE -amount END), 0) AS ledger_micros,
	COALESCE(ROUND(SUM((CASE WHEN type = 'debit' THEN amount ELSE -amount END)::numeric * exchange_rate_micros / 1000000)), 0)::bigint AS billing_micros,
	COUNT(*) AS transactions,
	COUNT(*) FILTER (WHERE type = 'debit') AS debits,
	COALESCE(BOOL_OR(exchange_rate_micros <= 0), false) AS missing_rate,
	COALESCE(ROUND(SUM((CASE WHEN type = 'debit' THEN amount ELSE -amount END)::numeric * exchange_rate_micros / 1000000) FILTER (WHERE EXISTS (
		SELECT 1 FROM ai_usage_records usage
		WHERE usage.workspace_id = balance_transactions.workspace_id
			AND usage.input_tokens + usage.output_tokens = 0
			AND usage.reference_id = CASE WHEN balance_transactions.is_refund THEN substr(balance_transactions.reference_id, 8) ELSE balance_transactions.reference_id END
	))), 0)::bigint AS generation_billing_micros
FROM balance_transactions
WHERE workspace_id = ?
	AND (
		(type = 'debit' AND is_refund = false AND reference_id LIKE ? ESCAPE '\')
		OR (type = 'credit' AND is_refund = true AND reference_id LIKE ? ESCAPE '\')
	)`

type referenceTotalsRow struct {
	LedgerMicros  int64
	BillingMicros int64
	Transactions  int
	Debits        int
	MissingRate   bool

	GenerationBillingMicros int64
}

func (r *BalanceRepositoryImpl) TotalsUnder(workspaceID, referencePrefix string) (balance.ReferenceTotals, error) {
	var row referenceTotalsRow
	err := r.db.Raw(referenceTotalsSQL, workspaceID, sqlexpr.LikePrefix(referencePrefix), sqlexpr.LikePrefix("refund:"+referencePrefix)).Scan(&row).Error
	if err != nil {
		return balance.ReferenceTotals{}, err
	}
	return balance.ReferenceTotals{
		LedgerMicros:  row.LedgerMicros,
		BillingMicros: row.BillingMicros,
		Transactions:  row.Transactions,
		Debits:        row.Debits,
		MissingRate:   row.MissingRate,

		GenerationBillingMicros: row.GenerationBillingMicros,
	}, nil
}

var _ balance.ReferenceTotaler = (*BalanceRepositoryImpl)(nil)
