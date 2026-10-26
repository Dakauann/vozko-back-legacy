package audience_repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	ca "vozko/domain/audience"
	"vozko/domain/shared"
	"vozko/infra/repositories/repotest"
)

type sentimentSeed struct {
	t       *testing.T
	db      *gorm.DB
	ws      string
	lead    string
	entry   string
	account string
}

func seedTelegramLead(t *testing.T, db *gorm.DB) sentimentSeed {
	t.Helper()
	s := sentimentSeed{t: t, db: db, ws: uuid.NewString(), lead: uuid.NewString(), entry: uuid.NewString(), account: uuid.NewString()}
	contact := uuid.NewString()
	repotest.InsertRow(t, db, "leads", map[string]interface{}{"id": s.lead, "workspace_id": s.ws, "name": "Maria"})
	repotest.InsertRow(t, db, "telegram_contacts", map[string]interface{}{"id": contact, "workspace_id": s.ws, "lead_id": s.lead})
	repotest.InsertRow(t, db, "telegram_conversations", map[string]interface{}{"id": s.entry, "workspace_id": s.ws, "contact_id": contact})
	return s
}

func (s sentimentSeed) analysed(sentiment shared.Sentiment, at time.Time) *ca.Analysis {
	s.t.Helper()
	a := &ca.Analysis{
		ID: uuid.NewString(), WorkspaceID: s.ws, SubjectKind: ca.SubjectKindConversation, Revision: uuid.NewString(),
		Source: ca.SourceTelegram, AccountID: s.account, ContainerID: s.entry, SubjectID: s.entry, AuthorExternalID: "x",
		Status: ca.StatusPending, OccurredAt: at, CreatedAt: at, UpdatedAt: at,
	}
	if _, err := NewRepository(s.db).Insert(context.Background(), a); err != nil {
		s.t.Fatal(err)
	}
	a.Status, a.Sentiment = ca.StatusAnalyzed, sentiment
	return a
}

func (s sentimentSeed) sentiment() string {
	s.t.Helper()
	var got []string
	if err := s.db.Raw("SELECT sentiment FROM lead_sentiments WHERE lead_id = ?", s.lead).Scan(&got).Error; err != nil {
		s.t.Fatal(err)
	}
	if len(got) == 0 {
		return ""
	}
	return got[0]
}

func TestSavingAnAnalysedConversationMovesItsLeadSentimentInTheSameWriteAgainstPostgres(t *testing.T) {
	db := integrationDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	s := seedTelegramLead(t, db)

	first := s.analysed(shared.SentimentNegative, time.Now().Add(-3*time.Hour))
	if err := repo.Save(ctx, first); err != nil {
		t.Fatal(err)
	}
	if got := s.sentiment(); got != "negative" {
		t.Fatalf("after Save the lead reads %q, want negative", got)
	}

	second := s.analysed(shared.SentimentPositive, time.Now().Add(-time.Hour))
	if err := repo.SaveMany(ctx, []*ca.Analysis{second}); err != nil {
		t.Fatal(err)
	}
	if got := s.sentiment(); got != "positive" {
		t.Fatalf("after SaveMany the lead reads %q, want positive", got)
	}

	now := time.Now()
	second.DeletedAt = &now
	if err := repo.Save(ctx, second); err != nil {
		t.Fatal(err)
	}
	if got := s.sentiment(); got != "negative" {
		t.Fatalf("after deleting the newest verdict the lead reads %q, want the remaining negative", got)
	}
}

func TestPurgingOldAnalysesRefreshesTheLeadsTheyColouredAgainstPostgres(t *testing.T) {
	db := integrationDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	s := seedTelegramLead(t, db)
	a := s.analysed(shared.SentimentNeutral, time.Now().Add(-time.Hour))
	if err := repo.Save(ctx, a); err != nil {
		t.Fatal(err)
	}
	if n, err := repo.PurgeBefore(ctx, time.Now().Add(time.Hour), 100); err != nil || n != 1 {
		t.Fatalf("PurgeBefore() = %d, %v", n, err)
	}
	if got := s.sentiment(); got != "" {
		t.Fatalf("a purged verdict still colours the lead: %q", got)
	}
}

func TestPurgingAnAccountRefreshesTheLeadsItColouredAgainstPostgres(t *testing.T) {
	db := integrationDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	s := seedTelegramLead(t, db)
	a := s.analysed(shared.SentimentPositive, time.Now().Add(-time.Hour))
	if err := repo.Save(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := NewAccountPurger(db).PurgeAccount(ctx, ca.SourceTelegram, s.account); err != nil {
		t.Fatal(err)
	}
	if got := s.sentiment(); got != "" {
		t.Fatalf("a purged account still colours the lead: %q", got)
	}
}
