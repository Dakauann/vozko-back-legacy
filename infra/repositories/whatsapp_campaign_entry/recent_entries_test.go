package whatsapp_campaign_entry

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	wce "vozko/domain/whatsapp_campaign_entry"
)

func TestRecentEntriesForCampaignsReadEveryCampaignInOneLateralQuery(t *testing.T) {
	repo, mock, sqlDB := newEntryDB(t)
	defer sqlDB.Close()

	newer := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	older := newer.Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT e.* FROM unnest($1::uuid[]) AS c(id) ` +
		`JOIN LATERAL (SELECT * FROM whatsapp_campaign_entries WHERE campaign_id = c.id AND deleted_at IS NULL ORDER BY updated_at DESC, id DESC LIMIT $2) e ON true ` +
		`ORDER BY e.campaign_id, e.updated_at DESC, e.id DESC`)).
		WithArgs(`{"c1","c2","c3"}`, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "campaign_id", "lead_id", "status", "updated_at"}).
			AddRow("e2", "c1", "l2", "SENT", newer).
			AddRow("e1", "c1", "l1", "PENDING", older).
			AddRow("e3", "c3", "l3", "READ", newer))

	got, err := repo.ListRecentlyUpdatedForCampaigns([]string{"c1", " c2 ", "c1", "", "c3"}, 100)
	if err != nil {
		t.Fatalf("ListRecentlyUpdatedForCampaigns: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	if len(got) != 3 {
		t.Fatalf("every requested campaign gets a key, got %v", got)
	}
	if ids := entryIDs(got["c1"]); ids != "e2,e1" {
		t.Errorf("c1 entries = %s, want e2,e1 in the query order", ids)
	}
	if got["c2"] == nil || len(got["c2"]) != 0 {
		t.Errorf("a campaign without entries gets an empty, non-nil list, got %#v", got["c2"])
	}
	if ids := entryIDs(got["c3"]); ids != "e3" {
		t.Errorf("c3 entries = %s, want e3", ids)
	}
	if got["c1"][0].LeadID != "l2" || string(got["c1"][0].Status) != "SENT" {
		t.Errorf("rows map to the domain entry, got %+v", got["c1"][0])
	}
}

func TestRecentEntriesForCampaignsSkipTheDatabaseWithoutCampaigns(t *testing.T) {
	repo, mock, sqlDB := newEntryDB(t)
	defer sqlDB.Close()

	got, err := repo.ListRecentlyUpdatedForCampaigns([]string{"", "  "}, 100)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want an empty map without a query", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecentEntriesForCampaignsBindOnlyTheirTwoArguments(t *testing.T) {
	if got := strings.Count(recentlyUpdatedForCampaignsSQL, "?"); got != 2 {
		t.Fatalf("placeholders = %d, want 2 (campaign ids, limit)", got)
	}
}

func TestRecentEntriesForOneCampaignBreakUpdatedAtTiesById(t *testing.T) {
	repo, mock, sqlDB := newEntryDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "whatsapp_campaign_entries" WHERE campaign_id = $1 AND "whatsapp_campaign_entries"."deleted_at" IS NULL ORDER BY updated_at DESC, id DESC LIMIT $2`)).
		WithArgs("c1", 100).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	got, err := repo.ListRecentlyUpdated("c1", 100)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v, %v; want an empty, non-nil list", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func entryIDs(entries []wce.WhatsAppCampaignEntry) string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return strings.Join(ids, ",")
}
