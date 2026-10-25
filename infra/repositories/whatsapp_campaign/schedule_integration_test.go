package whatsapp_campaign_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vozko/domain/campaign/scheduledstart"
	wc "vozko/domain/whatsapp_campaign"
	"vozko/infra/database/schema"
	"vozko/infra/testenv"
)

func scheduleIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testenv.Postgres(t).DSN()
	config := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true}
	admin, err := gorm.Open(postgres.Open(dsn), config)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	schemaName := "wc_schedule_test_" + uuid.New().String()[:8]
	if err := admin.Exec("CREATE SCHEMA " + schemaName).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schemaName), config)
	if err != nil {
		t.Fatalf("open schema: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		_ = admin.Exec("DROP SCHEMA " + schemaName + " CASCADE").Error
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&schema.WhatsAppCampaign{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestTheScheduleColumnsAreAddedToAnExistingCampaignTable(t *testing.T) {
	db := scheduleIntegrationDB(t)
	for _, column := range []string{"scheduled_by", "schedule_fingerprint", "schedule_failure"} {
		if err := db.Exec("ALTER TABLE whatsapp_campaigns DROP COLUMN " + column).Error; err != nil {
			t.Fatalf("drop %s: %v", column, err)
		}
	}
	id := uuid.NewString()
	if err := db.Exec(`INSERT INTO whatsapp_campaigns (id, workspace_id, name, status, scheduled_start) VALUES (?, ?, 'Antiga', 'STOPPED', ?)`, id, uuid.NewString(), time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.AutoMigrate(&schema.WhatsAppCampaign{}); err != nil {
		t.Fatalf("boot migration: %v", err)
	}

	got, err := NewRepository(db).FindByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ScheduledBy != "" || got.ScheduleConfirmedFingerprint != "" || got.ScheduleFailure != "" {
		t.Fatalf("an old campaign read %+v", got)
	}
	var nullable int64
	db.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'whatsapp_campaigns'
		AND column_name IN ('scheduled_by', 'schedule_fingerprint', 'schedule_failure') AND (is_nullable = 'YES' OR column_default IS NULL)`).Scan(&nullable)
	if nullable != 0 {
		t.Fatalf("%d schedule columns are nullable or have no default", nullable)
	}
}

func TestAScheduleRoundTripsThroughPostgres(t *testing.T) {
	ctx := context.Background()
	repo := NewRepository(scheduleIntegrationDB(t)).(*repository)
	due := time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond)
	c := &wc.Campaign{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), Name: "Natal", Type: wc.CampaignTypeStandard, Status: wc.CampaignStatusStopped,
		ScheduledStart: due, ScheduledBy: "u1", ScheduleConfirmedFingerprint: "abc"}
	if err := repo.Create(c); err != nil {
		t.Fatal(err)
	}

	listed, err := repo.ListScheduledToStart(time.Now(), 10)
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed %v, %v", listed, err)
	}
	if listed[0].ScheduledBy != "u1" || listed[0].ScheduleConfirmedFingerprint != "abc" || !listed[0].ScheduledStart.Equal(due) {
		t.Fatalf("listed %+v", listed[0])
	}

	claimed, err := repo.ClaimScheduledStart(c.ID, time.Now())
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	afterClaim, _ := repo.FindByID(c.ID)
	if !afterClaim.ScheduledStart.IsZero() || afterClaim.ScheduledBy != "" || afterClaim.ScheduleConfirmedFingerprint != "" {
		t.Fatalf("the claim left the schedule behind: %+v", afterClaim)
	}

	if err := repo.RecordScheduleFailure(ctx, c.ID, scheduledstart.FailureUnconfirmed); err != nil {
		t.Fatal(err)
	}
	failed, _ := repo.FindByID(c.ID)
	if failed.ScheduleFailure != scheduledstart.FailureUnconfirmed {
		t.Fatalf("failure = %q", failed.ScheduleFailure)
	}

	next := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Microsecond)
	failed.ScheduledStart, failed.ScheduledBy, failed.ScheduleConfirmedFingerprint, failed.ScheduleFailure = next, "u2", "def", ""
	if err := repo.Update(c.ID, failed); err != nil {
		t.Fatal(err)
	}
	rescheduled, _ := repo.FindByID(c.ID)
	if !rescheduled.ScheduledStart.Equal(next) || rescheduled.ScheduledBy != "u2" || rescheduled.ScheduleConfirmedFingerprint != "def" || rescheduled.ScheduleFailure != "" {
		t.Fatalf("rescheduled %+v", rescheduled)
	}

	if err := repo.RecordScheduleFailure(ctx, uuid.NewString(), scheduledstart.FailureBlocked); !errors.Is(err, wc.ErrCampaignNotFound) {
		t.Fatalf("a missing campaign = %v", err)
	}
}
