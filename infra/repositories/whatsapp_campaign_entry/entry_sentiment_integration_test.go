package whatsapp_campaign_entry

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/infra/repositories/repotest"
)

type entryWorld struct {
	db       *gorm.DB
	ws       string
	lead     string
	campaign string
	entry    string
}

func entrySentimentWorld(t *testing.T) entryWorld {
	t.Helper()
	db := repotest.IsolatedDB(t, "wce_sentiment")
	repotest.MigrateLeadSentiments(t, db)
	w := entryWorld{db: db, ws: uuid.NewString(), lead: uuid.NewString(), campaign: uuid.NewString(), entry: uuid.NewString()}
	contact, conversation := uuid.NewString(), uuid.NewString()
	repotest.InsertRow(t, db, "leads", map[string]interface{}{"id": w.lead, "workspace_id": w.ws, "name": "Maria"})
	repotest.InsertRow(t, db, "telegram_contacts", map[string]interface{}{"id": contact, "workspace_id": w.ws, "lead_id": w.lead})
	repotest.InsertRow(t, db, "telegram_conversations", map[string]interface{}{"id": conversation, "workspace_id": w.ws, "contact_id": contact})
	repotest.InsertRow(t, db, "whatsapp_campaign_entries", map[string]interface{}{"id": w.entry, "lead_id": w.lead, "campaign_id": w.campaign})
	older, newer := time.Now().Add(-9*time.Hour), time.Now().Add(-time.Hour)
	analysed := func(source, subject, sentiment string, at time.Time) string {
		id := uuid.NewString()
		repotest.InsertRow(t, db, "audience_analyses", map[string]interface{}{
			"id": id, "workspace_id": w.ws, "subject_kind": "conversation", "revision": uuid.NewString(), "source": source,
			"subject_id": subject, "status": "analyzed", "sentiment": sentiment, "occurred_at": at, "created_at": at,
		})
		return id
	}
	analysed("telegram", conversation, "positive", older)
	read := analysed("whatsapp", w.entry, "negative", newer)
	repotest.InsertRow(t, db, "lead_sentiments", map[string]interface{}{"lead_id": w.lead, "workspace_id": w.ws, "sentiment": "negative", "analysis_id": read, "refreshed_at": newer})
	return w
}

func (w entryWorld) sentiment(t *testing.T) string {
	t.Helper()
	var got []string
	if err := w.db.Raw("SELECT sentiment FROM lead_sentiments WHERE lead_id = ?", w.lead).Scan(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		return ""
	}
	return got[0]
}

func TestDeletingTheCampaignEntryALeadWasReadFromFallsBackToItsOtherConversationAgainstPostgres(t *testing.T) {
	for name, remove := range map[string]func(w entryWorld) error{
		"one entry":       func(w entryWorld) error { return NewRepository(w.db).Delete(w.entry) },
		"the campaign":    func(w entryWorld) error { return NewRepository(w.db).DeleteByCampaignID(w.campaign) },
		"a replaced list": func(w entryWorld) error { return NewRepository(w.db).ReplaceCampaignEntries(w.campaign, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			w := entrySentimentWorld(t)
			if err := remove(w); err != nil {
				t.Fatal(err)
			}
			if got := w.sentiment(t); got != "positive" {
				t.Fatalf("after removing the entry the lead reads %q, want its telegram verdict", got)
			}
		})
	}
}
