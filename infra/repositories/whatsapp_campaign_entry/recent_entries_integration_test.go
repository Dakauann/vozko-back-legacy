package whatsapp_campaign_entry

import (
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	wce "vozko/domain/whatsapp_campaign_entry"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

type recentSeed struct {
	db   *gorm.DB
	base time.Time
}

func newRecentSeed(t *testing.T) recentSeed {
	t.Helper()
	db := repotest.IsolatedDB(t, "wce_recent")
	db.Config.DisableForeignKeyConstraintWhenMigrating = true
	if err := db.AutoMigrate(&schema.WhatsAppCampaignEntry{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return recentSeed{db: db, base: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
}

func (s recentSeed) entry(t *testing.T, campaignID string, minutesAgo int, deleted bool) {
	t.Helper()
	updatedAt := s.base.Add(-time.Duration(minutesAgo) * time.Minute)
	var deletedAt *time.Time
	if deleted {
		deletedAt = &updatedAt
	}
	if err := s.db.Exec(
		`INSERT INTO whatsapp_campaign_entries (id, campaign_id, lead_id, status, created_at, updated_at, deleted_at) VALUES (?, ?, ?, 'SENT', ?, ?, ?)`,
		uuid.NewString(), campaignID, uuid.NewString(), updatedAt, updatedAt, deletedAt,
	).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func legacyRecentlyUpdated(t *testing.T, db *gorm.DB, campaignID string, limit int) []schema.WhatsAppCampaignEntry {
	t.Helper()
	var rows []schema.WhatsAppCampaignEntry
	if err := db.Where("campaign_id = ?", campaignID).Order("updated_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		t.Fatalf("legacy query: %v", err)
	}
	return rows
}

func TestRecentEntriesForCampaignsMatchThePerCampaignQueryAgainstPostgres(t *testing.T) {
	seed := newRecentSeed(t)
	repo := &repository{db: seed.db}

	busy, empty, onlyDeleted, small := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, minutes := range []int{1, 2, 2, 3, 3, 3, 4, 5, 5, 6} {
		seed.entry(t, busy, minutes, false)
	}
	seed.entry(t, busy, 0, true)
	seed.entry(t, busy, 3, true)
	seed.entry(t, onlyDeleted, 1, true)
	seed.entry(t, onlyDeleted, 2, true)
	seed.entry(t, small, 7, false)
	seed.entry(t, small, 7, false)
	seed.entry(t, small, 1, true)
	campaigns := []string{busy, empty, onlyDeleted, small}

	for _, limit := range []int{1, 3, 4, 6, 100} {
		batch, err := repo.ListRecentlyUpdatedForCampaigns(campaigns, limit)
		if err != nil {
			t.Fatalf("limit %d: batch: %v", limit, err)
		}
		if len(batch) != len(campaigns) {
			t.Fatalf("limit %d: batch keys = %d, want %d", limit, len(batch), len(campaigns))
		}
		for _, campaignID := range campaigns {
			single, err := repo.ListRecentlyUpdated(campaignID, limit)
			if err != nil {
				t.Fatalf("limit %d: single: %v", limit, err)
			}
			got := batch[campaignID]
			if got == nil {
				t.Fatalf("limit %d, campaign %s: nil list, want an empty one", limit, campaignID)
			}
			if a, b := entryIDs(got), entryIDs(single); a != b {
				t.Fatalf("limit %d, campaign %s: batch %s, per campaign %s", limit, campaignID, a, b)
			}
			for i := range got {
				if got[i].CampaignID != campaignID || !got[i].UpdatedAt.Equal(single[i].UpdatedAt) || got[i].LeadID != single[i].LeadID || got[i].Status != single[i].Status {
					t.Fatalf("limit %d, campaign %s, row %d: batch %+v, per campaign %+v", limit, campaignID, i, got[i], single[i])
				}
			}
			assertSameAsLegacyOutsideTies(t, legacyRecentlyUpdated(t, seed.db, campaignID, limit), entryIDs(got), got)
		}
	}

	if got := recentBatch(t, repo, campaigns, 100); len(got[empty]) != 0 || len(got[onlyDeleted]) != 0 || len(got[busy]) != 10 || len(got[small]) != 2 {
		t.Fatalf("soft-deleted rows must stay out: busy %d, empty %d, onlyDeleted %d, small %d", len(got[busy]), len(got[empty]), len(got[onlyDeleted]), len(got[small]))
	}
}

func recentBatch(t *testing.T, repo *repository, campaigns []string, limit int) map[string][]wce.WhatsAppCampaignEntry {
	t.Helper()
	got, err := repo.ListRecentlyUpdatedForCampaigns(campaigns, limit)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	return got
}

func assertSameAsLegacyOutsideTies(t *testing.T, legacy []schema.WhatsAppCampaignEntry, label string, got []wce.WhatsAppCampaignEntry) {
	t.Helper()
	if len(legacy) != len(got) {
		t.Fatalf("%s: %d rows, legacy %d", label, len(got), len(legacy))
	}
	if len(got) == 0 {
		return
	}
	for i := range got {
		if !got[i].UpdatedAt.Equal(legacy[i].UpdatedAt) {
			t.Fatalf("%s: row %d updated_at %v, legacy %v", label, i, got[i].UpdatedAt, legacy[i].UpdatedAt)
		}
	}
	boundary := got[len(got)-1].UpdatedAt
	var gotAbove, legacyAbove []string
	for i := range got {
		if got[i].UpdatedAt.After(boundary) {
			gotAbove = append(gotAbove, got[i].ID)
		}
		if legacy[i].UpdatedAt.After(boundary) {
			legacyAbove = append(legacyAbove, legacy[i].ID)
		}
	}
	sort.Strings(gotAbove)
	sort.Strings(legacyAbove)
	for i := range gotAbove {
		if gotAbove[i] != legacyAbove[i] {
			t.Fatalf("%s: rows above the last tie differ: %v, legacy %v", label, gotAbove, legacyAbove)
		}
	}
}
