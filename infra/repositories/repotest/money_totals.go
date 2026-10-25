package repotest

import (
	"testing"

	"gorm.io/gorm"
)

const todaysMoneyTotalsSQL = `SELECT resource_type, type, COALESCE(SUM(amount), 0) AS total FROM balance_transactions WHERE balance_id = ? GROUP BY resource_type, type`

func TodaysMoneyTotals(t testing.TB, db *gorm.DB, balanceID string) (credits, debits int64) {
	t.Helper()
	var rows []struct {
		ResourceType string
		Type         string
		Total        int64
	}
	if err := db.Raw(todaysMoneyTotalsSQL, balanceID).Scan(&rows).Error; err != nil {
		t.Fatalf("today's money totals: %v", err)
	}
	for _, row := range rows {
		if row.ResourceType != "money" {
			continue
		}
		if row.Type == "credit" {
			credits = row.Total
		} else {
			debits = row.Total
		}
	}
	return credits, debits
}
