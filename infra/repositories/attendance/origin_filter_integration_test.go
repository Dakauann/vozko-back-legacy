package attendance_repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/attendance"
	"vozko/domain/conversation"
	conversation_repository "vozko/infra/repositories/conversation"
	"vozko/infra/repositories/repotest"
)

type originSeed struct {
	channel string
	senders []string
}

func seedOriginWorkspace(t *testing.T, db *gorm.DB, now time.Time) string {
	t.Helper()
	workspaceID := seedAttendanceWorkspace(t, db, now)
	campaignID, accountID := uuid.NewString(), uuid.NewString()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}
	exec(`INSERT INTO whatsapp_campaigns (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, 'Black Friday', ?, ?)`,
		campaignID, workspaceID, now.AddDate(0, -1, 0), now.AddDate(0, -1, 0))
	exec(`INSERT INTO instagram_accounts (id, workspace_id, ig_user_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		accountID, workspaceID, uuid.NewString(), now.AddDate(0, -1, 0), now.AddDate(0, -1, 0))
	seeds := []originSeed{
		{"whatsapp", []string{"campaign", "contact"}},
		{"whatsapp", []string{"campaign"}},
		{"whatsapp", []string{"human", "contact"}},
		{"instagram", []string{"system", "contact", "human"}},
		{"instagram", []string{"unknown", "contact"}},
	}
	for i, seed := range seeds {
		entryID := uuid.NewString()
		created := now.AddDate(0, 0, -(i + 1))
		switch seed.channel {
		case "whatsapp":
			exec(`INSERT INTO whatsapp_campaign_entries (id, campaign_id, lead_id, conversation_status, last_message_at, created_at, updated_at)
				VALUES (?, ?, ?, 'ongoing', ?, ?, ?)`, entryID, campaignID, uuid.NewString(), created.Add(time.Hour), created, created)
		case "instagram":
			exec(`INSERT INTO instagram_conversations (id, workspace_id, ig_account_id, contact_id, conversation_status, last_message_at, created_at, updated_at)
				VALUES (?, ?, ?, ?, 'ongoing', ?, ?, ?)`, entryID, workspaceID, accountID, uuid.NewString(), created.Add(time.Hour), created, created)
		}
		for m, sender := range seed.senders {
			exec(`INSERT INTO conversation_messages (id, entry_id, entry_type, message_type, text, sender_kind, created_at, updated_at)
				VALUES (?, ?, ?, 'text', 'oi', ?, ?, ?)`, uuid.NewString(), entryID, seed.channel, sender,
				created.Add(time.Duration(m+1)*time.Minute), created)
		}
	}
	backfill, err := conversation_repository.NewOriginBackfill(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := backfill.Run(context.Background()); err != nil {
		t.Fatalf("record origins: %v", err)
	}
	return workspaceID
}

func TestTheOriginFilterSplitsEverySectionByWhereTheConversationStartedAgainstPostgres(t *testing.T) {
	db := repotest.IsolatedDB(t, "att_origin", attendanceModels()...)
	now := time.Now().UTC()
	workspaceID := seedOriginWorkspace(t, db, now)
	repo := New(db)
	wide := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	engaged := map[conversation.Origin]int64{}
	created := map[conversation.Origin]int64{}
	for _, origin := range append([]conversation.Origin{""}, conversation.Origins()...) {
		filter := attendance.OverviewFilter{DateFrom: &wide, DateTo: &now, IncludeAI: true, Origin: origin}
		sections := readSections(context.Background(), t, repo, workspaceID, filter, now)
		engaged[origin] = sections.Summary.KPIs.Engaged
		for _, bucket := range sections.Trend.Buckets {
			created[origin] += bucket.Created
		}
		if got := sections.Stages.StagedEngaged + sections.Stages.UnstagedEngaged; got != sections.Summary.KPIs.Engaged {
			t.Fatalf("origin %q: stages see %d engaged, summary %d", origin, got, sections.Summary.KPIs.Engaged)
		}
	}

	want := map[conversation.Origin]int64{
		conversation.OriginReceptive: 7,
		conversation.OriginCampaign:  2,
		conversation.OriginOutreach:  1,
		conversation.OriginUnknown:   1,
	}
	var sum int64
	for origin, count := range want {
		if engaged[origin] != count {
			t.Fatalf("engaged %s = %d, want %d (all engaged: %v)", origin, engaged[origin], count, engaged)
		}
		sum += count
	}
	if engaged[""] != sum {
		t.Fatalf("unfiltered engaged = %d, want the %d the origins add up to", engaged[""], sum)
	}
	if created[conversation.OriginCampaign] == 0 || created[conversation.OriginCampaign] > created[""] {
		t.Fatalf("trend created by campaign origin = %d of %d", created[conversation.OriginCampaign], created[""])
	}
}
