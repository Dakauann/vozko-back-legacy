package scheduled_message_repository

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	sm "vozko/domain/scheduled_message"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
	"vozko/infra/testenv"
)

type legacyScheduledMessage struct {
	ID              string `gorm:"primaryKey;type:uuid"`
	WorkspaceID     string `gorm:"type:uuid;not null"`
	EntryID         string `gorm:"type:uuid;not null"`
	EntryType       string `gorm:"size:32;not null"`
	CreatedByUserID string `gorm:"type:uuid;not null"`
	Kind            string `gorm:"size:20;not null;default:'text'"`
	Text            string `gorm:"type:text"`
	ScheduledAt     time.Time
	Status          string `gorm:"size:20;not null;default:'pending'"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt
}

func (legacyScheduledMessage) TableName() string { return "scheduled_messages" }

func postgresRepo(t *testing.T) (sm.Repository, *gorm.DB) {
	t.Helper()
	dsn := testenv.Postgres(t).DSN()
	silent := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(postgres.Open(dsn), silent)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	schemaName := "sched_test_" + uuid.New().String()[:8]
	if err := admin.Exec("CREATE SCHEMA " + schemaName).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schemaName), silent)
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
	if err := db.AutoMigrate(&legacyScheduledMessage{}); err != nil {
		t.Fatalf("legacy table: %v", err)
	}
	if err := db.AutoMigrate(&schema.ScheduledMessage{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepository(db), db
}

const (
	pgWorkspace = "8f6c2a52-1b0f-4f7e-9a34-0d5f8f1e2c3b"
	pgEntry     = "1d2f3a4b-5c6d-4e7f-8a9b-0c1d2e3f4a5b"
)

func followUp(author string, scheduledAt time.Time) *sm.ScheduledMessage {
	return &sm.ScheduledMessage{
		ID:                  uuid.NewString(),
		WorkspaceID:         pgWorkspace,
		EntryID:             pgEntry,
		EntryType:           shared.EntryTypeWhatsApp,
		CreatedByAutomation: author,
		CancelOnReply:       true,
		Kind:                sm.KindText,
		Text:                "Oi, conseguiu enviar o comprovante?",
		ScheduledAt:         scheduledAt,
		Status:              sm.StatusPending,
	}
}

func TestAFollowUpIsStoredOnATableThatUsedToRequireAPerson(t *testing.T) {
	repo, _ := postgresRepo(t)
	m := followUp("ai:agent-1", time.Now().Add(time.Hour))
	if err := repo.CreateCapped(m, sm.MaxPendingFollowUps); err != nil {
		t.Fatalf("CreateCapped: %v", err)
	}
	got, err := repo.FindByID(m.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.CreatedByAutomation != "ai:agent-1" || got.CreatedByUserID != "" || !got.CancelOnReply {
		t.Fatalf("stored = %+v", got)
	}
}

func TestConcurrentFollowUpsNeverPassTheCap(t *testing.T) {
	repo, _ := postgresRepo(t)
	const racers = 8
	var wg sync.WaitGroup
	results := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = repo.CreateCapped(followUp("ai:agent-1", time.Now().Add(time.Hour)), sm.MaxPendingFollowUps)
		}(i)
	}
	wg.Wait()

	created, refused := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, sm.ErrFollowUpLimitReached):
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if created != sm.MaxPendingFollowUps || refused != racers-sm.MaxPendingFollowUps {
		t.Fatalf("created %d refused %d, want %d and %d", created, refused, sm.MaxPendingFollowUps, racers-sm.MaxPendingFollowUps)
	}
}

func TestAFinishedFollowUpFreesItsPlaceUnderTheCap(t *testing.T) {
	repo, _ := postgresRepo(t)
	first := followUp("ai:agent-1", time.Now().Add(time.Hour))
	for _, m := range []*sm.ScheduledMessage{first, followUp("workflow:wf-1", time.Now().Add(time.Hour)), followUp("ai:agent-2", time.Now().Add(time.Hour))} {
		if err := repo.CreateCapped(m, sm.MaxPendingFollowUps); err != nil {
			t.Fatalf("CreateCapped: %v", err)
		}
	}
	if err := repo.CreateCapped(followUp("ai:agent-1", time.Now().Add(time.Hour)), sm.MaxPendingFollowUps); !errors.Is(err, sm.ErrFollowUpLimitReached) {
		t.Fatalf("err = %v, want the cap shared by every automation in the conversation", err)
	}
	if err := repo.Cancel(first.ID, sm.CancelByAutomation); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if err := repo.CreateCapped(followUp("ai:agent-1", time.Now().Add(time.Hour)), sm.MaxPendingFollowUps); err != nil {
		t.Fatalf("after a cancel: %v", err)
	}
	got, _ := repo.FindByID(first.ID)
	if got.Status != sm.StatusCanceled || got.CancelReason == nil || *got.CancelReason != sm.CancelByAutomation {
		t.Fatalf("cancelled = %+v", got)
	}
}

func TestAPersonsMessagesDoNotCountAgainstTheAutomationCap(t *testing.T) {
	repo, _ := postgresRepo(t)
	for i := 0; i < sm.MaxPendingFollowUps; i++ {
		person := followUp("", time.Now().Add(time.Hour))
		person.CreatedByUserID = "6a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"
		person.CancelOnReply = false
		if err := repo.Create(person); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	if err := repo.CreateCapped(followUp("ai:agent-1", time.Now().Add(time.Hour)), sm.MaxPendingFollowUps); err != nil {
		t.Fatalf("CreateCapped: %v", err)
	}
}

func TestAContactReplyCancelsOnlyEarlierFlaggedFollowUps(t *testing.T) {
	repo, db := postgresRepo(t)
	repliedAt := time.Now().UTC()

	flagged := followUp("ai:agent-1", repliedAt.Add(time.Hour))
	unflagged := followUp("ai:agent-1", repliedAt.Add(time.Hour))
	unflagged.CancelOnReply = false
	later := followUp("ai:agent-1", repliedAt.Add(time.Hour))
	for _, m := range []*sm.ScheduledMessage{flagged, unflagged, later} {
		if err := repo.Create(m); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	if err := db.Exec("UPDATE scheduled_messages SET created_at = ? WHERE id IN ?", repliedAt.Add(-time.Minute), []string{flagged.ID, unflagged.ID}).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if err := db.Exec("UPDATE scheduled_messages SET created_at = ? WHERE id = ?", repliedAt.Add(time.Second), later.ID).Error; err != nil {
		t.Fatalf("postdate: %v", err)
	}

	n, err := repo.CancelOnReply(pgEntry, string(shared.EntryTypeWhatsApp), repliedAt)
	if err != nil {
		t.Fatalf("CancelOnReply: %v", err)
	}
	if n != 1 {
		t.Fatalf("cancelled %d, want only the flagged follow-up created before the reply", n)
	}
	got, _ := repo.FindByID(flagged.ID)
	if got.Status != sm.StatusCanceled || got.CancelReason == nil || *got.CancelReason != sm.CancelContactReplied {
		t.Fatalf("flagged = %+v", got)
	}
	for _, id := range []string{unflagged.ID, later.ID} {
		if got, _ := repo.FindByID(id); got.Status != sm.StatusPending {
			t.Fatalf("%s: status %s, want pending", id, got.Status)
		}
	}
}

func TestAClaimedFollowUpCanBeStoodDown(t *testing.T) {
	repo, _ := postgresRepo(t)
	m := followUp("ai:agent-1", time.Now().Add(-time.Minute))
	if err := repo.Create(m); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.CancelClaimed(m.ID, sm.CancelAutomationStoodDown); !errors.Is(err, sm.ErrNotPending) {
		t.Fatalf("err = %v, want a refusal for a row that was never claimed", err)
	}
	if _, err := repo.ClaimForDispatch(m.ID, time.Now()); err != nil {
		t.Fatalf("ClaimForDispatch: %v", err)
	}
	if err := repo.CancelClaimed(m.ID, sm.CancelAutomationStoodDown); err != nil {
		t.Fatalf("CancelClaimed: %v", err)
	}
	got, _ := repo.FindByID(m.ID)
	if got.Status != sm.StatusCanceled || got.CancelReason == nil || *got.CancelReason != sm.CancelAutomationStoodDown {
		t.Fatalf("stood down = %+v", got)
	}
}
