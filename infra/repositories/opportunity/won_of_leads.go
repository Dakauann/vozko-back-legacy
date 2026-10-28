package opportunity_repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"vozko/domain/opportunity"
)

var ErrWonOfLeadsInvalid = errors.New("opportunity: the wins of a lead set need a workspace and a period")

func NewWonOfLeadsReader(db *gorm.DB) opportunity.WonOfLeadsReader {
	return &repository{db: db}
}

func wonOfLeadsSQL(condition string) string {
	return "SELECT COALESCE(NULLIF(" + oppAlias + ".currency, ''), 'BRL') AS currency, COUNT(*)::bigint AS won_count, COALESCE(SUM(" + oppAlias + ".value_cents), 0)::bigint AS value_cents" +
		" FROM opportunities " + oppAlias + " WHERE " + condition +
		" AND " + oppAlias + ".status = 'won' AND " + oppAlias + ".close_date IS NOT NULL AND " + oppAlias + ".close_date >= ? AND " + oppAlias + ".close_date < ?" +
		" GROUP BY 1 ORDER BY 3 DESC, 1"
}

func (r *repository) WonDealsOfLeads(ctx context.Context, workspaceID string, leadIDs []string, from, to time.Time, scope opportunity.DealScope) ([]opportunity.WonTotal, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" || !to.After(from) {
		return nil, ErrWonOfLeadsInvalid
	}
	if len(leadIDs) == 0 {
		return []opportunity.WonTotal{}, nil
	}
	membership, args := leadSetDealsMembership(oppAlias, workspaceID, leadIDs)
	condition := oppAlias + ".workspace_id = ? AND " + oppAlias + ".deleted_at IS NULL AND " + membership
	args = append([]interface{}{workspaceID}, args...)
	if cond, condArgs := DealScopeCondition(oppAlias, scope); cond != "" {
		condition += " AND " + cond
		args = append(args, condArgs...)
	}
	args = append(args, from, to)
	type row struct {
		Currency   string `gorm:"column:currency"`
		WonCount   int64  `gorm:"column:won_count"`
		ValueCents int64  `gorm:"column:value_cents"`
	}
	var rows []row
	if err := r.db.WithContext(ctx).Raw(wonOfLeadsSQL(condition), args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]opportunity.WonTotal, 0, len(rows))
	for _, x := range rows {
		out = append(out, opportunity.WonTotal{Currency: x.Currency, Count: x.WonCount, ValueCents: x.ValueCents})
	}
	return out, nil
}
