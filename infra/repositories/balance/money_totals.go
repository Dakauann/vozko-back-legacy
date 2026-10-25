package balance_repository

import (
	"time"

	"gorm.io/gorm"

	"vozko/domain/balance"
	"vozko/infra/database/schema"
)

func moneyTotalsCurrent(held *schema.Balance) bool {
	return held.MoneyTotalsAt != nil && held.MoneyTotalsAt.Equal(held.UpdatedAt)
}

func recordMovement(tx *gorm.DB, held *schema.Balance, movement *schema.BalanceTransaction) error {
	if err := stampChargeCampaign(tx, movement); err != nil {
		return err
	}
	if err := tx.Create(movement).Error; err != nil {
		return err
	}
	return tx.Model(held).Updates(balanceAfterMovement(held, movement, time.Now())).Error
}

func balanceAfterMovement(held *schema.Balance, movement *schema.BalanceTransaction, now time.Time) map[string]interface{} {
	updates := map[string]interface{}{"amount": movement.BalanceAfter, "updated_at": now}
	if !moneyTotalsCurrent(held) {
		return updates
	}
	credits, debits := held.TotalMoneyCredits, held.TotalMoneyDebits
	if movement.ResourceType == string(balance.ResourceTypeMoney) {
		switch balance.TransactionType(movement.Type) {
		case balance.TransactionTypeCredit:
			credits += movement.Amount
		case balance.TransactionTypeDebit:
			debits += movement.Amount
		default:
			return updates
		}
	}
	updates["total_money_credits"] = credits
	updates["total_money_debits"] = debits
	updates["money_totals_at"] = now
	return updates
}
