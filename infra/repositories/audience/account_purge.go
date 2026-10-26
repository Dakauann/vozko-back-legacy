package audience_repository

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"gorm.io/gorm"

	ca "vozko/domain/audience"
	"vozko/infra/repositories/leadsentiment"
)

var errPurgeAccountRequired = errors.New("audience account purge: the source and the account are required")

var accountTables = []string{
	"audience_analyses",
	"audience_authors",
	"audience_batches",
	"audience_backfills",
	"audience_container_settings",
	"audience_rollups",
	"audience_settings",
	"audience_alert_rules",
}

type accountPurger struct {
	db *gorm.DB
}

func NewAccountPurger(db *gorm.DB) ca.AccountPurger {
	if db == nil {
		panic("audience account purge: the database is required")
	}
	return &accountPurger{db: db}
}

func (p *accountPurger) PurgeAccount(ctx context.Context, source ca.Source, accountID string) error {
	if strings.TrimSpace(string(source)) == "" || strings.TrimSpace(accountID) == "" {
		return errPurgeAccountRequired
	}
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var coloured []struct{ WorkspaceID, LeadID string }
		if err := tx.Raw(colouredByAccountSQL, string(source), accountID).Scan(&coloured).Error; err != nil {
			return fmt.Errorf("audience account purge: the leads it coloured: %w", err)
		}
		for _, table := range accountTables {
			if err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE source = ? AND account_id = ?", table), string(source), accountID).Error; err != nil {
				return fmt.Errorf("audience account purge: %s: %w", table, err)
			}
		}
		byWorkspace := map[string][]string{}
		for _, c := range coloured {
			byWorkspace[c.WorkspaceID] = append(byWorkspace[c.WorkspaceID], c.LeadID)
		}
		for _, ws := range slices.Sorted(maps.Keys(byWorkspace)) {
			if err := leadsentiment.RefreshLeads(tx, ws, byWorkspace[ws]); err != nil {
				return fmt.Errorf("audience account purge: %w", err)
			}
		}
		return nil
	})
}

const colouredByAccountSQL = "SELECT ls.workspace_id::text AS workspace_id, ls.lead_id::text AS lead_id FROM lead_sentiments ls" +
	" JOIN audience_analyses aa ON aa.id = ls.analysis_id WHERE aa.source = ? AND aa.account_id = ?"
