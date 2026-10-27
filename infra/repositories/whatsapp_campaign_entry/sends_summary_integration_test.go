package whatsapp_campaign_entry

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vozko/domain/campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
	"vozko/infra/database/schema"
	"vozko/infra/testenv"
)

func sendsSummaryDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testenv.Postgres(t).DSN()
	silent := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(postgres.Open(dsn), silent)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	schemaName := "wce_sends_test_" + uuid.New().String()[:8]
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
	for _, ddl := range []string{
		`CREATE TABLE whatsapp_templates (id uuid PRIMARY KEY, category varchar(20))`,
		`CREATE TABLE whatsapp_campaigns (
			id uuid PRIMARY KEY, workspace_id uuid NOT NULL, department_id uuid, template_id uuid,
			type varchar(20), created_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz)`,
		`CREATE TABLE whatsapp_campaign_entries (
			id uuid PRIMARY KEY, campaign_id uuid NOT NULL, status varchar(40) NOT NULL,
			error_code int NOT NULL DEFAULT 0, created_at timestamptz NOT NULL, deleted_at timestamptz, metadata jsonb NOT NULL DEFAULT '{}',
			sent_at timestamptz, delivered_at timestamptz, read_at timestamptz, failed_at timestamptz)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	if err := db.AutoMigrate(&schema.WhatsAppTemplateSend{}, &schema.ConversationMessage{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

type sendsCampaign struct {
	id, workspace, kind string
	department          *string
	template            string
}

type sendsEntry struct {
	id, campaign, status string
	errorCode            int
	created              time.Time
	sent, failed, gone   *time.Time
	metadata             string
}

type oneOffSend struct {
	workspace, status, category, wamid string
	campaign, entry                    *string
	created                            time.Time
	gone                               bool
	receipt                            string
}

func october(day int) time.Time { return time.Date(2026, 10, day, 12, 0, 0, 0, time.UTC) }

func TestTallySendsAgainstPostgres(t *testing.T) {
	db := sendsSummaryDB(t)
	repo := &repository{db: db}

	workspace, foreign := uuid.NewString(), uuid.NewString()
	sales, support := uuid.NewString(), uuid.NewString()
	marketing, utility := uuid.NewString(), uuid.NewString()
	launch, receptive, supportLaunch, foreignLaunch := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()

	for id, category := range map[string]string{marketing: "MARKETING", utility: "utility"} {
		if err := db.Exec(`INSERT INTO whatsapp_templates (id, category) VALUES (?, ?)`, id, category).Error; err != nil {
			t.Fatalf("seed template: %v", err)
		}
	}
	for _, c := range []sendsCampaign{
		{id: launch, workspace: workspace, kind: "standard", department: &sales, template: marketing},
		{id: receptive, workspace: workspace, kind: "organic", department: &sales, template: marketing},
		{id: supportLaunch, workspace: workspace, kind: "standard", department: &support, template: utility},
		{id: foreignLaunch, workspace: foreign, kind: "standard", template: marketing},
	} {
		if err := db.Exec(`INSERT INTO whatsapp_campaigns (id, workspace_id, department_id, template_id, type, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			c.id, c.workspace, c.department, c.template, c.kind, october(1).AddDate(0, -2, 0)).Error; err != nil {
			t.Fatalf("seed campaign: %v", err)
		}
	}

	joined := october(12)
	supportEntry := uuid.NewString()
	for _, e := range []sendsEntry{
		{id: uuid.NewString(), campaign: launch, status: "READ", created: october(1), sent: at(october(15))},
		{id: uuid.NewString(), campaign: launch, status: "READ", created: october(1), sent: at(october(16)), metadata: `{"template_info": {"category": "UTILITY"}}`},
		{id: uuid.NewString(), campaign: launch, status: "DELIVERED", created: october(1), sent: at(october(1))},
		{id: uuid.NewString(), campaign: launch, status: "SENT", created: october(1)},
		{id: uuid.NewString(), campaign: launch, status: "NOT_ELIGIBLE_POSSIBLE_SPAM", created: joined},
		{id: uuid.NewString(), campaign: launch, status: "FAILED", errorCode: campaign.SkipOptedOut.FailureCode(), created: joined},
		{id: uuid.NewString(), campaign: launch, status: "FAILED", errorCode: 131026, created: october(1)},
		{id: uuid.NewString(), campaign: launch, status: "PENDING", created: joined},
		{id: uuid.NewString(), campaign: launch, status: "READ", created: october(1), sent: at(october(15)), gone: at(october(16))},
		{id: uuid.NewString(), campaign: receptive, status: "SENT", created: october(1), sent: at(october(15))},
		{id: supportEntry, campaign: supportLaunch, status: "DELIVERED", created: october(1), sent: at(october(16))},
		{id: uuid.NewString(), campaign: foreignLaunch, status: "READ", created: october(1), sent: at(october(15))},
	} {
		if e.metadata == "" {
			e.metadata = "{}"
		}
		if err := db.Exec(`INSERT INTO whatsapp_campaign_entries (id, campaign_id, status, error_code, created_at, sent_at, failed_at, deleted_at, metadata) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb)`,
			e.id, e.campaign, e.status, e.errorCode, e.created, e.sent, e.failed, e.gone, e.metadata).Error; err != nil {
			t.Fatalf("seed entry: %v", err)
		}
	}

	for _, s := range []oneOffSend{
		{workspace: workspace, status: "sent", category: "UTILITY", wamid: "wamid.a", campaign: &receptive, created: october(14), receipt: "read"},
		{workspace: workspace, status: "refunded", category: "MARKETING", wamid: "wamid.b", campaign: &launch, created: october(14), receipt: "delivered"},
		{workspace: workspace, status: "pending", category: "MARKETING", campaign: &launch, created: october(14)},
		{workspace: workspace, status: "sent", category: "UTILITY", wamid: "wamid.d", campaign: &receptive, created: october(1)},
		{workspace: workspace, status: "sent", category: "AUTHENTICATION", wamid: "wamid.e", entry: &supportEntry, created: october(15), receipt: "delivered"},
		{workspace: workspace, status: "unknown", category: "marketing", created: october(15)},
		{workspace: foreign, status: "sent", category: "MARKETING", wamid: "wamid.g", created: october(15)},
		{workspace: workspace, status: "sent", category: "MARKETING", wamid: "wamid.h", campaign: &receptive, created: october(15), gone: true},
	} {
		row := schema.WhatsAppTemplateSend{
			ID: uuid.NewString(), WorkspaceID: s.workspace, UserID: uuid.NewString(), IdempotencyKey: uuid.NewString(), BusinessPhoneID: uuid.NewString(),
			TemplateID: uuid.NewString(), Category: s.category, Status: s.status, ProviderMessageID: s.wamid,
			CampaignID: s.campaign, EntryID: s.entry, CreatedAt: s.created,
		}
		if s.gone {
			row.DeletedAt = gorm.DeletedAt{Time: s.created, Valid: true}
		}
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed send: %v", err)
		}
		if s.receipt != "" {
			wamid := s.wamid
			message := schema.ConversationMessage{EntryID: uuid.NewString(), EntryType: "whatsapp", WhatsAppMessageID: &wamid, DeliveryStatus: s.receipt}
			if err := db.Create(&message).Error; err != nil {
				t.Fatalf("seed message: %v", err)
			}
		}
	}

	from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 20, 23, 59, 59, 0, time.UTC)
	window := campaign.SendWindow{From: &from, To: &to}

	t.Run("a period counts every send by when it happened and reports the undated ones", func(t *testing.T) {
		got, err := repo.TallySends(wce.WorkspaceSummaryFilter{WorkspaceID: workspace, Window: window})
		if err != nil {
			t.Fatalf("tally: %v", err)
		}
		want := campaign.SendsTally{
			Campaigns:  campaign.Counts{Total: 5, Read: 2, Delivered: 1, NotEligiblePossibleSpam: 1, Failed: 1},
			OneOff:     campaign.Counts{Total: 4, Read: 1, Delivered: 1, Sent: 1, Failed: 1},
			ByCategory: campaign.CategoryDispatches{Marketing: 2, Utility: 3, Authentication: 1},
			Undated:    2,
		}
		if *got != want {
			t.Fatalf("got  %+v\nwant %+v", *got, want)
		}
	})

	t.Run("a department sees its campaigns and the one-off sends made from them", func(t *testing.T) {
		got, err := repo.TallySends(wce.WorkspaceSummaryFilter{WorkspaceID: workspace, DepartmentIDs: []string{sales}, Window: window})
		if err != nil {
			t.Fatalf("tally: %v", err)
		}
		want := campaign.SendsTally{
			Campaigns:  campaign.Counts{Total: 4, Read: 2, NotEligiblePossibleSpam: 1, Failed: 1},
			OneOff:     campaign.Counts{Total: 2, Read: 1, Failed: 1},
			ByCategory: campaign.CategoryDispatches{Marketing: 1, Utility: 2},
			Undated:    2,
		}
		if *got != want {
			t.Fatalf("got  %+v\nwant %+v", *got, want)
		}
	})

	t.Run("the whole history counts every row by its status and has nothing undated", func(t *testing.T) {
		got, err := repo.TallySends(wce.WorkspaceSummaryFilter{WorkspaceID: workspace})
		if err != nil {
			t.Fatalf("tally: %v", err)
		}
		want := campaign.SendsTally{
			Campaigns:  campaign.Counts{Total: 9, Pending: 1, Sent: 1, Delivered: 2, Read: 2, Failed: 2, NotEligiblePossibleSpam: 1},
			OneOff:     campaign.Counts{Total: 5, Sent: 2, Delivered: 1, Read: 1, Failed: 1},
			ByCategory: campaign.CategoryDispatches{Marketing: 4, Utility: 4, Authentication: 1},
		}
		if *got != want {
			t.Fatalf("got  %+v\nwant %+v", *got, want)
		}
	})

	t.Run("no workspace reads nothing", func(t *testing.T) {
		got, err := repo.TallySends(wce.WorkspaceSummaryFilter{})
		if err != nil || *got != (campaign.SendsTally{}) {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
}
