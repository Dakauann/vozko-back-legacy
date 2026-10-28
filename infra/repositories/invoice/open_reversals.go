package invoice_repository

import (
	"context"

	"gorm.io/gorm"

	"vozko/domain/invoice"
)

const openReversalSQL = `SELECT EXISTS (SELECT 1 FROM invoices WHERE workspace_id = ? AND status = ?)`

type OpenReversals struct {
	db *gorm.DB
}

func NewOpenReversals(db *gorm.DB) *OpenReversals {
	return &OpenReversals{db: db}
}

func (r *OpenReversals) HasOpenReversal(ctx context.Context, workspaceID string) (bool, error) {
	var open bool
	if err := r.db.WithContext(ctx).Raw(openReversalSQL, workspaceID, string(invoice.StatusReversed)).Scan(&open).Error; err != nil {
		return false, err
	}
	return open, nil
}
