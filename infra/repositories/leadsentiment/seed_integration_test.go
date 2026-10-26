package leadsentiment

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/shared"
	"vozko/infra/repositories/repotest"
)

type channelTables struct {
	entryType     shared.EntryType
	contacts      string
	conversations string
	contactColumn string
}

var contactChannelTables = []channelTables{
	{shared.EntryTypeUnofficialWhatsApp, "unofficial_whatsapp_contacts", "unofficial_whatsapp_conversations", "contact_id"},
	{shared.EntryTypeTelegram, "telegram_contacts", "telegram_conversations", "contact_id"},
	{shared.EntryTypeInstagram, "instagram_contacts", "instagram_conversations", "contact_id"},
	{shared.EntryTypeFacebook, "facebook_contacts", "facebook_conversations", "contact_id"},
	{shared.EntryTypeWebchat, "webchat_visitors", "webchat_conversations", "visitor_id"},
	{shared.EntryTypeTikTok, "tiktok_contacts", "tiktok_conversations", "contact_id"},
	{shared.EntryTypeEmail, "email_contacts", "email_conversations", "contact_id"},
	{shared.EntryTypeRCS, "carrier_contacts", "rcs_conversations", "contact_id"},
	{shared.EntryTypeSMS, "carrier_contacts", "sms_conversations", "contact_id"},
}

func channelOf(t *testing.T, e shared.EntryType) channelTables {
	t.Helper()
	for _, c := range contactChannelTables {
		if c.entryType == e {
			return c
		}
	}
	t.Fatalf("no tables for %s", e)
	return channelTables{}
}

func projectionDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := repotest.IsolatedDB(t, "lead_sentiment")
	repotest.MigrateLeadSentiments(t, db)
	return db
}

func insertRow(t *testing.T, db *gorm.DB, table string, values map[string]interface{}) {
	t.Helper()
	repotest.InsertRow(t, db, table, values)
}

type seed struct {
	t  *testing.T
	db *gorm.DB
	ws string
}

func newSeed(t *testing.T, db *gorm.DB) seed {
	return seed{t: t, db: db, ws: uuid.NewString()}
}

func (s seed) lead() string {
	s.t.Helper()
	id := uuid.NewString()
	insertRow(s.t, s.db, "leads", map[string]interface{}{"id": id, "workspace_id": s.ws, "name": "Lead " + id[:6]})
	return id
}

func (s seed) contact(c channelTables, leadID string) string {
	s.t.Helper()
	id := uuid.NewString()
	insertRow(s.t, s.db, c.contacts, map[string]interface{}{"id": id, "workspace_id": s.ws, "lead_id": leadID})
	return id
}

func (s seed) conversation(e shared.EntryType, leadID string) (entryID, contactID string) {
	s.t.Helper()
	entryID = uuid.NewString()
	if e == shared.EntryTypeWhatsApp {
		insertRow(s.t, s.db, "whatsapp_campaign_entries", map[string]interface{}{"id": entryID, "lead_id": leadID, "campaign_id": uuid.NewString()})
		return entryID, ""
	}
	c := channelOf(s.t, e)
	contactID = s.contact(c, leadID)
	insertRow(s.t, s.db, c.conversations, map[string]interface{}{"id": entryID, "workspace_id": s.ws, c.contactColumn: contactID})
	return entryID, contactID
}

type analysis struct {
	workspace string
	entryType shared.EntryType
	entryID   string
	status    string
	sentiment string
	occurred  time.Time
	deleted   bool
}

func (s seed) analysis(a analysis) string {
	s.t.Helper()
	id := uuid.NewString()
	ws := a.workspace
	if ws == "" {
		ws = s.ws
	}
	status := a.status
	if status == "" {
		status = "analyzed"
	}
	values := map[string]interface{}{
		"id": id, "workspace_id": ws, "subject_kind": "conversation", "revision": uuid.NewString(),
		"source": string(a.entryType), "subject_id": a.entryID, "status": status, "sentiment": a.sentiment,
		"occurred_at": a.occurred, "created_at": a.occurred, "updated_at": a.occurred,
	}
	if a.deleted {
		values["deleted_at"] = time.Now()
	}
	insertRow(s.t, s.db, "audience_analyses", values)
	return id
}

func (s seed) sentimentOf(leadID string) (string, string) {
	s.t.Helper()
	var rows []struct{ Sentiment, AnalysisID string }
	if err := s.db.Raw("SELECT sentiment, analysis_id::text AS analysis_id FROM lead_sentiments WHERE lead_id = ?", leadID).Scan(&rows).Error; err != nil {
		s.t.Fatal(err)
	}
	if len(rows) == 0 {
		return "", ""
	}
	return rows[0].Sentiment, rows[0].AnalysisID
}

func hoursAgo(h int) time.Time {
	return time.Now().UTC().Add(-time.Duration(h) * time.Hour).Truncate(time.Microsecond)
}
