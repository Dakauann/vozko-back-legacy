package googlesheet

import (
	"context"

	"vozko/domain/googlesheet"
)

const ledgerClaimPage = 500

func (m *Manager) changeUnique(ctx context.Context, sheet *googlesheet.Sheet, rule googlesheet.UniqueRule) error {
	before := sheet.Unique
	if err := sheet.ChangeUnique(rule); err != nil {
		return err
	}
	if !sheet.Unique.Active() || sheet.Unique.Same(before) {
		return nil
	}
	return m.claimLedger(ctx, sheet)
}

func (m *Manager) claimLedger(ctx context.Context, sheet *googlesheet.Sheet) error {
	after := googlesheet.LedgerCursor{}
	for {
		page, err := m.rows.LedgerPage(ctx, sheet.ID, after, ledgerClaimPage)
		if err != nil {
			return err
		}
		if _, err := m.rows.ClaimUnique(ctx, sheet.ClaimsOf(page)); err != nil {
			return err
		}
		if len(page) < ledgerClaimPage {
			return nil
		}
		after = page[len(page)-1].LedgerCursor()
	}
}
