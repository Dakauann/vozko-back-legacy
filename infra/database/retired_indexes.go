package database

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const lockNotAvailableCode = "55P03"

type retiredIndex struct {
	name         string
	constraintOf string
	coveredBy    []string
}

type lockRetry struct {
	lockTimeout string
	attempts    int
	pause       time.Duration
}

var retiredConstraintRetry = lockRetry{lockTimeout: "3s", attempts: 5, pause: 2 * time.Second}

func retiredIndexes() []retiredIndex {
	return []retiredIndex{
		{name: "whatsapp_campaign_entries_campaign_id_lead_id_key", constraintOf: "whatsapp_campaign_entries", coveredBy: []string{"idx_wce_campaign_lead"}},
		{name: "idx_whatsapp_campaign_entries_unique", coveredBy: []string{"idx_wce_campaign_lead"}},
		{name: "idx_whatsapp_campaign_entries_campaign_id", coveredBy: []string{"idx_wce_campaign_lead"}},
		{name: "idx_whatsapp_campaign_entries_lead_id", coveredBy: []string{"idx_wce_lead_created"}},
		{name: "idx_wce_campaign_status_live", coveredBy: []string{CampaignEntryLiveStatusIndex}},
		{name: "idx_wce_campaign_created_active", coveredBy: []string{CampaignEntryCreatedCoverIndex}},

		{name: "idx_entry_tags_tag_entry", coveredBy: []string{"idx_entry_stages_stage_entry"}},
		{name: "idx_entry_tags_tag_ws_del", coveredBy: []string{"idx_entry_stages_stage_ws_del"}},
		{name: "idx_entry_tags_entry", coveredBy: []string{"idx_entry_stages_entry"}},
		{name: "idx_entry_tags_ws_del", coveredBy: []string{"idx_entry_stages_ws_del"}},
		{name: "idx_entry_tags_deleted_at", coveredBy: []string{"idx_entry_stages_deleted_at"}},
		{name: "idx_entry_tag_entry", coveredBy: []string{"idx_entry_stages_entry"}},
		{name: "idx_entry_tag_tag", coveredBy: []string{"idx_entry_stages_stage_entry"}},

		{name: "idx_balance_transactions_user_id", coveredBy: []string{"idx_balance_transactions_workspace_id"}},
		{name: "idx_balance_transactions_created_at", coveredBy: []string{"idx_bt_reporting"}},
		{name: "idx_bt_ws_created_svc", coveredBy: []string{"idx_balance_transactions_workspace_id"}},

		{name: "idx_message_entry", coveredBy: []string{"idx_cm_entry_composite"}},
		{name: "idx_cm_entry_del_created", coveredBy: []string{"idx_cm_lateral_join", "idx_cm_entry_composite"}},
	}
}

func (r retiredIndex) dropSQL() string {
	if r.constraintOf != "" {
		return "ALTER TABLE " + r.constraintOf + " DROP CONSTRAINT IF EXISTS " + r.name
	}
	return "DROP INDEX CONCURRENTLY IF EXISTS " + r.name
}

func retireIndexes(db *gorm.DB) {
	for _, idx := range retiredIndexes() {
		if err := retireIndex(db, idx, retiredConstraintRetry); err != nil {
			log.Printf("[indexes] Warning: failed to retire %s: %v", idx.name, err)
		}
	}
}

func retireIndex(db *gorm.DB, idx retiredIndex, retry lockRetry) error {
	var present bool
	if err := db.Raw("SELECT to_regclass(?) IS NOT NULL", idx.name).Scan(&present).Error; err != nil {
		return err
	}
	if !present {
		return nil
	}
	for _, cover := range idx.coveredBy {
		ready, err := indexReady(db, cover)
		if err != nil {
			return err
		}
		if !ready {
			return fmt.Errorf("kept until %s is built and valid", cover)
		}
	}
	if idx.constraintOf == "" {
		return db.Exec(idx.dropSQL()).Error
	}
	return execUnderLockTimeoutWithRetry(db, idx.dropSQL(), retry)
}

func indexReady(db *gorm.DB, name string) (bool, error) {
	var ready bool
	err := db.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM pg_index
			WHERE indexrelid = to_regclass(?) AND indisvalid AND indisready
		)
	`, name).Scan(&ready).Error
	return ready, err
}

func execUnderLockTimeoutWithRetry(db *gorm.DB, sql string, retry lockRetry) error {
	var err error
	for attempt := 1; attempt <= retry.attempts; attempt++ {
		err = execUnderLockTimeout(db, retry.lockTimeout, sql)
		if !isLockNotAvailable(err) {
			return err
		}
		if attempt < retry.attempts {
			time.Sleep(retry.pause)
		}
	}
	return err
}

func execUnderLockTimeout(db *gorm.DB, lockTimeout, sql string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('lock_timeout', ?, true)", lockTimeout).Error; err != nil {
			return err
		}
		return tx.Exec(sql).Error
	})
}

func isLockNotAvailable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == lockNotAvailableCode
}
