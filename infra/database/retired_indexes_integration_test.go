package database

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func throwawaySchemaDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	db := openTestPostgres(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	name := fmt.Sprintf("retire_%d", time.Now().UnixNano())
	if err := db.Exec("CREATE SCHEMA " + name).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DROP SCHEMA IF EXISTS " + name + " CASCADE") })
	if err := db.Exec("SET search_path TO " + name).Error; err != nil {
		t.Fatal(err)
	}
	return db, name
}

func execAll(t *testing.T, db *gorm.DB, statements ...string) {
	t.Helper()
	for _, s := range statements {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func relationPresent(t *testing.T, db *gorm.DB, name string) bool {
	t.Helper()
	var present bool
	if err := db.Raw("SELECT to_regclass(?) IS NOT NULL", name).Scan(&present).Error; err != nil {
		t.Fatal(err)
	}
	return present
}

func campaignEntryConstraintPresent(t *testing.T, db *gorm.DB) bool {
	t.Helper()
	var present bool
	err := db.Raw(`SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = ? AND conrelid = to_regclass('whatsapp_campaign_entries'))`,
		"whatsapp_campaign_entries_campaign_id_lead_id_key").Scan(&present).Error
	if err != nil {
		t.Fatal(err)
	}
	return present
}

const campaignEntryFixture = `CREATE TABLE whatsapp_campaign_entries (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	campaign_id uuid NOT NULL,
	lead_id uuid NOT NULL,
	status text NOT NULL DEFAULT 'PENDING',
	created_at timestamptz NOT NULL DEFAULT now(),
	deleted_at timestamptz,
	CONSTRAINT whatsapp_campaign_entries_campaign_id_lead_id_key UNIQUE (campaign_id, lead_id)
)`

func TestRetiringTheCampaignEntryTwinsKeepsTheConflictTarget(t *testing.T) {
	db, _ := throwawaySchemaDB(t)
	execAll(t, db,
		campaignEntryFixture,
		`CREATE UNIQUE INDEX idx_wce_campaign_lead ON whatsapp_campaign_entries (campaign_id, lead_id)`,
		`CREATE UNIQUE INDEX idx_whatsapp_campaign_entries_unique ON whatsapp_campaign_entries (campaign_id, lead_id) WHERE deleted_at IS NULL`,
		`CREATE INDEX idx_whatsapp_campaign_entries_campaign_id ON whatsapp_campaign_entries (campaign_id)`,
		`CREATE INDEX idx_whatsapp_campaign_entries_lead_id ON whatsapp_campaign_entries (lead_id)`,
		`CREATE INDEX idx_wce_lead_created ON whatsapp_campaign_entries (lead_id, created_at)`,
	)

	retireIndexes(db)
	retireIndexes(db)

	for _, gone := range []string{
		"whatsapp_campaign_entries_campaign_id_lead_id_key",
		"idx_whatsapp_campaign_entries_unique",
		"idx_whatsapp_campaign_entries_campaign_id",
		"idx_whatsapp_campaign_entries_lead_id",
	} {
		if relationPresent(t, db, gone) {
			t.Fatalf("%s was not retired", gone)
		}
	}
	if campaignEntryConstraintPresent(t, db) {
		t.Fatalf("the duplicate unique constraint is still on whatsapp_campaign_entries")
	}
	for _, kept := range []string{"idx_wce_campaign_lead", "idx_wce_lead_created"} {
		if !relationPresent(t, db, kept) {
			t.Fatalf("%s was dropped with its twins", kept)
		}
	}

	insert := `INSERT INTO whatsapp_campaign_entries (campaign_id, lead_id)
		VALUES ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222')
		ON CONFLICT (campaign_id, lead_id) DO NOTHING`
	execAll(t, db, insert, insert)
	var count int64
	if err := db.Raw("SELECT count(*) FROM whatsapp_campaign_entries").Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("ON CONFLICT (campaign_id, lead_id) kept %d rows, want 1", count)
	}
}

func TestARetiredIndexStaysUntilItsReplacementIsValid(t *testing.T) {
	db, _ := throwawaySchemaDB(t)
	execAll(t, db,
		`CREATE TABLE whatsapp_campaign_entries (id uuid PRIMARY KEY, campaign_id uuid NOT NULL, lead_id uuid NOT NULL, status text NOT NULL, deleted_at timestamptz)`,
		`CREATE INDEX idx_wce_campaign_status_live ON whatsapp_campaign_entries (campaign_id, status) INCLUDE (lead_id) WHERE deleted_at IS NULL`,
	)
	old := retiredIndex{name: "idx_wce_campaign_status_live", coveredBy: []string{CampaignEntryLiveStatusIndex}}

	if err := retireIndex(db, old, quickRetry); err == nil {
		t.Fatalf("retireIndex() dropped %s before %s existed", old.name, CampaignEntryLiveStatusIndex)
	}
	if !relationPresent(t, db, old.name) {
		t.Fatalf("%s was dropped while nothing replaced it", old.name)
	}

	if err := createIndexConcurrently(db, declaredConcurrentIndex(t, CampaignEntryLiveStatusIndex)); err != nil {
		t.Fatalf("build %s: %v", CampaignEntryLiveStatusIndex, err)
	}
	if err := retireIndex(db, old, quickRetry); err != nil {
		t.Fatalf("retireIndex() after the replacement was built: %v", err)
	}
	if relationPresent(t, db, old.name) {
		t.Fatalf("%s was kept after its replacement was built", old.name)
	}
}

func TestTheConstraintDropGivesUpOnABusyTableAndSucceedsLater(t *testing.T) {
	db, schemaName := throwawaySchemaDB(t)
	execAll(t, db,
		campaignEntryFixture,
		`CREATE UNIQUE INDEX idx_wce_campaign_lead ON whatsapp_campaign_entries (campaign_id, lead_id)`,
	)
	constraint := retiredIndex{
		name:         "whatsapp_campaign_entries_campaign_id_lead_id_key",
		constraintOf: "whatsapp_campaign_entries",
		coveredBy:    []string{"idx_wce_campaign_lead"},
	}

	holder := openTestPostgres(t).Begin()
	if err := holder.Exec("LOCK TABLE " + schemaName + ".whatsapp_campaign_entries IN ACCESS SHARE MODE").Error; err != nil {
		holder.Rollback()
		t.Fatal(err)
	}
	busy := lockRetry{lockTimeout: "200ms", attempts: 2}
	err := retireIndex(db, constraint, busy)
	holder.Rollback()

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != lockNotAvailableCode {
		t.Fatalf("retireIndex() on a busy table error = %v, want a lock timeout", err)
	}
	if !campaignEntryConstraintPresent(t, db) {
		t.Fatalf("the constraint was dropped while the table was busy")
	}

	if err := retireIndex(db, constraint, busy); err != nil {
		t.Fatalf("retireIndex() once the table is free: %v", err)
	}
	if campaignEntryConstraintPresent(t, db) {
		t.Fatalf("the constraint is still there")
	}
	if !relationPresent(t, db, "idx_wce_campaign_lead") {
		t.Fatalf("the kept unique index went with the constraint")
	}
}
