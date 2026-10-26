package conversation_repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func seedOriginMessage(t *testing.T, db *gorm.DB, entryID, entryType, sender string, at time.Time) string {
	t.Helper()
	id := uuid.NewString()
	if err := db.Exec(`INSERT INTO conversation_messages (id, entry_id, entry_type, message_type, text, sender_kind, created_at, updated_at)
		VALUES (?, ?, ?, 'user_message', 'oi', ?, ?, ?)`, id, entryID, entryType, sender, at, at).Error; err != nil {
		t.Fatalf("seed message: %v", err)
	}
	return id
}

func originRows(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	var rows []struct {
		EntryID string
		Origin  string
	}
	if err := db.Raw(`SELECT entry_id::text AS entry_id, origin FROM conversation_origins`).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, row := range rows {
		out[row.EntryID] = row.Origin
	}
	return out
}

func TestTheOriginBackfillFillsEveryConversationInResumableChunksAgainstPostgres(t *testing.T) {
	db := repotest.IsolatedDB(t, "origin_backfill", &schema.ConversationMessage{}, &schema.ConversationOrigin{})
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	want := map[string]string{}
	senders := []string{"contact", "campaign", "human", "external", "unknown"}
	origins := []string{"receptive", "campaign", "outreach", "outreach", "unknown"}
	entryTypes := []string{"whatsapp", "instagram", "email", "unofficial_whatsapp", "sms"}
	for i := 0; i < 15; i++ {
		entryID := uuid.NewString()
		seedOriginMessage(t, db, entryID, entryTypes[i%5], "system", at)
		seedOriginMessage(t, db, entryID, entryTypes[i%5], senders[i%5], at.Add(time.Minute))
		seedOriginMessage(t, db, entryID, entryTypes[i%5], "contact", at.Add(time.Hour))
		want[entryID] = origins[i%5]
	}
	onlySystem := "ffffffff-ffff-4fff-bfff-ffffffffffff"
	seedOriginMessage(t, db, onlySystem, "whatsapp", "system", at)

	backfill := newOriginBackfill(db, 4, 2)
	if err := backfill.Run(context.Background()); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if got := len(originRows(t, db)); got != 8 {
		t.Fatalf("one run of 2 chunks of 4 filled %d conversations, want 8", got)
	}
	for runs := 0; runs < 5; runs++ {
		if err := backfill.Run(context.Background()); err != nil {
			t.Fatalf("run %d: %v", runs, err)
		}
	}
	got := originRows(t, db)
	if len(got) != len(want) {
		t.Fatalf("filled %d conversations, want %d (a system only conversation has no origin)", len(got), len(want))
	}
	for entryID, origin := range want {
		if got[entryID] != origin {
			t.Fatalf("conversation %s origin = %q, want %q", entryID, got[entryID], origin)
		}
	}

	if err := db.Exec(`UPDATE conversation_origins SET origin = 'campaign'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := newOriginBackfill(db, 50, 10).Run(context.Background()); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	for entryID, origin := range originRows(t, db) {
		if origin != "campaign" {
			t.Fatalf("a rerun rewrote the settled conversation %s to %q; settled rows are kept by the write path", entryID, origin)
		}
	}
}

func TestTheOriginBackfillNeedsADatabase(t *testing.T) {
	if _, err := NewOriginBackfill(nil); err == nil {
		t.Fatal("a backfill without a database must refuse to start")
	}
}
