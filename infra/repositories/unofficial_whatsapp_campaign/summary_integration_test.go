package unofficial_whatsapp_campaign_repository

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/campaign"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
	"vozko/infra/database/schema"
)

func TestTallySendsPlacesEveryEntryByWhenItsOutcomeHappened(t *testing.T) {
	db := groupCampaignDB(t)
	summary := NewSummaryRepository(db)
	ws, sales := uuid.NewString(), uuid.NewString()
	day := func(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC) }
	stamp := func(t time.Time) *time.Time { return &t }

	old := schema.UnofficialWhatsAppCampaign{ID: uuid.NewString(), WorkspaceID: ws, DepartmentID: &sales, Name: "Antiga", Status: string(campaign.StatusCompleted), CreatedAt: day(1).AddDate(0, -2, 0)}
	if err := db.Create(&old).Error; err != nil {
		t.Fatalf("campaign: %v", err)
	}
	for _, e := range []schema.UnofficialWhatsAppCampaignEntry{
		{Status: "READ", SentAt: stamp(day(15)), CreatedAt: day(1)},
		{Status: "DELIVERED", SentAt: stamp(day(2)), CreatedAt: day(1)},
		{Status: "SENT", CreatedAt: day(1)},
		{Status: "NOT_ELIGIBLE_POSSIBLE_SPAM", CreatedAt: day(12)},
		{Status: "FAILED", ErrorCode: campaign.SkipBlocked.FailureCode(), CreatedAt: day(12)},
		{Status: "FAILED", FailedAt: stamp(day(14)), CreatedAt: day(1)},
		{Status: "PENDING", CreatedAt: day(12)},
	} {
		e.ID, e.CampaignID, e.WorkspaceID, e.Number = uuid.NewString(), old.ID, ws, uuid.NewString()[:12]
		if err := db.Create(&e).Error; err != nil {
			t.Fatalf("entry: %v", err)
		}
	}

	from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 20, 23, 59, 59, 0, time.UTC)
	got, err := summary.TallySends(uwc.WorkspaceSummaryFilter{WorkspaceID: ws, Window: campaign.SendWindow{From: &from, To: &to}})
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	want := campaign.SendsTally{
		Campaigns: campaign.Counts{Total: 4, Read: 1, NotEligiblePossibleSpam: 1, Failed: 2},
		Undated:   1,
	}
	if *got != want {
		t.Fatalf("a campaign created two months ago that sent in the period must count there:\ngot  %+v\nwant %+v", *got, want)
	}

	all, err := summary.TallySends(uwc.WorkspaceSummaryFilter{WorkspaceID: ws, DepartmentIDs: []string{sales}})
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if all.Campaigns.Total != 7 || all.Campaigns.Pending != 1 || all.Undated != 0 {
		t.Fatalf("the whole history counts every entry by status: %+v", *all)
	}
}
