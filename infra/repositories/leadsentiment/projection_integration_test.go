package leadsentiment

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	ca "vozko/domain/audience"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
)

var errRefusedForTest = errors.New("the update was refused")

func refreshed(t *testing.T, s seed, leads ...string) {
	t.Helper()
	if err := RefreshLeads(s.db, s.ws, leads); err != nil {
		t.Fatal(err)
	}
}

func wantSentiment(t *testing.T, s seed, leadID, want string) {
	t.Helper()
	if got, _ := s.sentimentOf(leadID); got != want {
		t.Fatalf("sentiment of %s = %q, want %q", leadID[:8], got, want)
	}
}

func TestALeadTakesTheSentimentOfItsNewestAnalysedConversationAgainstPostgres(t *testing.T) {
	db := projectionDB(t)
	s := newSeed(t, db)
	tg := shared.EntryTypeTelegram

	happy := s.lead()
	entry, _ := s.conversation(tg, happy)
	s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "positive", occurred: hoursAgo(5)})

	stillRed := s.lead()
	entry, _ = s.conversation(tg, stillRed)
	s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "negative", occurred: hoursAgo(5)})
	s.analysis(analysis{entryType: tg, entryID: entry, status: "pending", occurred: hoursAgo(1)})

	turnedRed := s.lead()
	entry, _ = s.conversation(tg, turnedRed)
	s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "positive", occurred: hoursAgo(5)})
	s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "negative", occurred: hoursAgo(1)})

	twoChannels := s.lead()
	older, _ := s.conversation(tg, twoChannels)
	newer, _ := s.conversation(shared.EntryTypeInstagram, twoChannels)
	s.analysis(analysis{entryType: tg, entryID: older, sentiment: "negative", occurred: hoursAgo(9)})
	s.analysis(analysis{entryType: shared.EntryTypeInstagram, entryID: newer, sentiment: "neutral", occurred: hoursAgo(2)})

	silent := s.lead()
	s.conversation(tg, silent)

	noFallback := s.lead()
	entry, _ = s.conversation(tg, noFallback)
	s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "positive", occurred: hoursAgo(9)})
	s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "angry", occurred: hoursAgo(1)})

	otherWorkspace := s.lead()
	entry, _ = s.conversation(tg, otherWorkspace)
	s.analysis(analysis{workspace: uuid.NewString(), entryType: tg, entryID: entry, sentiment: "negative", occurred: hoursAgo(1)})

	deletedVerdict := s.lead()
	entry, _ = s.conversation(tg, deletedVerdict)
	s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "positive", occurred: hoursAgo(9)})
	s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "negative", occurred: hoursAgo(1), deleted: true})

	refreshed(t, s, happy, stillRed, turnedRed, twoChannels, silent, noFallback, otherWorkspace, deletedVerdict)

	for _, c := range []struct {
		name, lead, want string
	}{
		{"one analysed positive conversation", happy, "positive"},
		{"a newer pending revision keeps the last verdict", stillRed, "negative"},
		{"a newer analysed revision replaces the verdict", turnedRed, "negative"},
		{"the newer conversation on another channel wins", twoChannels, "neutral"},
		{"a conversation never analysed gives nothing", silent, ""},
		{"an unknown newest verdict gives nothing, never the older one", noFallback, ""},
		{"an analysis in another workspace is ignored", otherWorkspace, ""},
		{"a deleted analysis is ignored", deletedVerdict, "positive"},
	} {
		t.Run(c.name, func(t *testing.T) { wantSentiment(t, s, c.lead, c.want) })
	}
}

func TestEveryAnalysableChannelSpeaksForItsLeadAgainstPostgres(t *testing.T) {
	db := projectionDB(t)
	s := newSeed(t, db)
	for _, e := range shared.ConversationAnalysableEntryTypes() {
		t.Run(string(e), func(t *testing.T) {
			lead := s.lead()
			entry, _ := s.conversation(e, lead)
			s.analysis(analysis{entryType: e, entryID: entry, sentiment: "negative", occurred: hoursAgo(1)})
			refreshed(t, s, lead)
			wantSentiment(t, s, lead, "negative")
		})
	}
}

func TestARelinkedConversationStopsSpeakingForTheOldLeadAndSpeaksForTheNewOneAgainstPostgres(t *testing.T) {
	db := projectionDB(t)
	s := newSeed(t, db)
	tg := shared.EntryTypeTelegram
	before, after := s.lead(), s.lead()
	kept, _ := s.conversation(tg, before)
	moved, contact := s.conversation(tg, before)
	s.analysis(analysis{entryType: tg, entryID: kept, sentiment: "positive", occurred: hoursAgo(9)})
	s.analysis(analysis{entryType: tg, entryID: moved, sentiment: "negative", occurred: hoursAgo(1)})
	refreshed(t, s, before, after)
	wantSentiment(t, s, before, "negative")

	err := Relink(db, &schema.TelegramContact{}, contact, func(tx *gorm.DB) error {
		return tx.Model(&schema.TelegramContact{}).Where("id = ?", contact).Update("lead_id", after).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSentiment(t, s, before, "positive")
	wantSentiment(t, s, after, "negative")

	err = Relink(db, &schema.TelegramContact{}, contact, func(tx *gorm.DB) error {
		return tx.Model(&schema.TelegramContact{}).Where("id = ?", contact).Update("lead_id", nil).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSentiment(t, s, after, "")
}

func TestAFailedRelinkLeavesTheProjectionAsItWasAgainstPostgres(t *testing.T) {
	db := projectionDB(t)
	s := newSeed(t, db)
	tg := shared.EntryTypeTelegram
	lead := s.lead()
	entry, contact := s.conversation(tg, lead)
	s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "neutral", occurred: hoursAgo(1)})
	refreshed(t, s, lead)
	refused := errRefusedForTest
	err := Relink(db, &schema.TelegramContact{}, contact, func(tx *gorm.DB) error {
		if err := tx.Model(&schema.TelegramContact{}).Where("id = ?", contact).Update("lead_id", nil).Error; err != nil {
			return err
		}
		return refused
	})
	if err != refused {
		t.Fatalf("Relink() = %v, want the update's own error", err)
	}
	wantSentiment(t, s, lead, "neutral")
}

func TestSavingAnalysesRefreshesTheLeadsOfTheirConversationsAgainstPostgres(t *testing.T) {
	db := projectionDB(t)
	s := newSeed(t, db)
	tg := shared.EntryTypeTelegram
	lead := s.lead()
	entry, _ := s.conversation(tg, lead)
	first := s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "positive", occurred: hoursAgo(9)})
	if err := RefreshForAnalyses(db, []*ca.Analysis{{ID: first, WorkspaceID: s.ws, Source: ca.SourceTelegram, SubjectKind: ca.SubjectKindConversation, SubjectID: entry}}); err != nil {
		t.Fatal(err)
	}
	wantSentiment(t, s, lead, "positive")

	second := s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "negative", occurred: hoursAgo(1)})
	if err := RefreshForAnalyses(db, []*ca.Analysis{{ID: second, WorkspaceID: s.ws, Source: ca.SourceTelegram, SubjectKind: ca.SubjectKindConversation, SubjectID: entry}}); err != nil {
		t.Fatal(err)
	}
	wantSentiment(t, s, lead, "negative")

	if err := db.Exec("DELETE FROM audience_analyses WHERE id = ?", second).Error; err != nil {
		t.Fatal(err)
	}
	if err := RefreshReferencing(db, s.ws, []string{second}); err != nil {
		t.Fatal(err)
	}
	wantSentiment(t, s, lead, "positive")
}

func TestACommentAnalysisNeverTouchesTheProjectionAgainstPostgres(t *testing.T) {
	db := projectionDB(t)
	s := newSeed(t, db)
	lead := s.lead()
	if err := RefreshForAnalyses(db, []*ca.Analysis{{ID: uuid.NewString(), WorkspaceID: s.ws, Source: ca.SourceInstagram, SubjectKind: ca.SubjectKindComment, SubjectID: "17890000"}}); err != nil {
		t.Fatal(err)
	}
	wantSentiment(t, s, lead, "")
}

func TestRefreshingTwiceChangesNothingAgainstPostgres(t *testing.T) {
	db := projectionDB(t)
	s := newSeed(t, db)
	tg := shared.EntryTypeTelegram
	lead := s.lead()
	entry, _ := s.conversation(tg, lead)
	s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "positive", occurred: hoursAgo(1)})
	refreshed(t, s, lead)
	var first, second string
	db.Raw("SELECT refreshed_at::text FROM lead_sentiments WHERE lead_id = ?", lead).Scan(&first)
	refreshed(t, s, lead)
	db.Raw("SELECT refreshed_at::text FROM lead_sentiments WHERE lead_id = ?", lead).Scan(&second)
	if first == "" || first != second {
		t.Fatalf("refreshed_at moved from %q to %q although nothing changed", first, second)
	}
}

func TestALeadOfAnotherWorkspaceIsNeverRefreshedAgainstPostgres(t *testing.T) {
	db := projectionDB(t)
	s := newSeed(t, db)
	tg := shared.EntryTypeTelegram
	lead := s.lead()
	entry, _ := s.conversation(tg, lead)
	s.analysis(analysis{entryType: tg, entryID: entry, sentiment: "positive", occurred: hoursAgo(1)})
	if err := RefreshLeads(db, uuid.NewString(), []string{lead}); err != nil {
		t.Fatal(err)
	}
	wantSentiment(t, s, lead, "")
}

func TestRemovingTheConversationALeadWasReadFromFallsBackToItsOtherConversationAgainstPostgres(t *testing.T) {
	db := projectionDB(t)
	s := newSeed(t, db)
	wa := shared.EntryTypeWhatsApp
	lead, bystander := s.lead(), s.lead()
	older, _ := s.conversation(shared.EntryTypeTelegram, lead)
	removed, _ := s.conversation(wa, lead)
	kept, _ := s.conversation(wa, bystander)
	s.analysis(analysis{entryType: shared.EntryTypeTelegram, entryID: older, sentiment: "positive", occurred: hoursAgo(9)})
	s.analysis(analysis{entryType: wa, entryID: removed, sentiment: "negative", occurred: hoursAgo(1)})
	s.analysis(analysis{entryType: wa, entryID: kept, sentiment: "neutral", occurred: hoursAgo(1)})
	refreshed(t, s, lead, bystander)
	_, bystanderRead := s.sentimentOf(bystander)

	if err := db.Exec("UPDATE whatsapp_campaign_entries SET deleted_at = now() WHERE id = ?", removed).Error; err != nil {
		t.Fatal(err)
	}
	if err := RefreshLeadsReadingEntries(db, wa, "SELECT id FROM whatsapp_campaign_entries WHERE id = ?", removed); err != nil {
		t.Fatal(err)
	}
	wantSentiment(t, s, lead, "positive")
	if _, read := s.sentimentOf(bystander); read != bystanderRead {
		t.Fatal("a lead reading another conversation is left alone")
	}
}

func TestTheBackfillWalksEveryLeadInChunksAndASecondRunChangesNothingAgainstPostgres(t *testing.T) {
	db := projectionDB(t)
	s := newSeed(t, db)
	tg := shared.EntryTypeTelegram
	want := map[string]string{}
	for i, sentiment := range []string{"positive", "neutral", "negative", "", "angry"} {
		lead := s.lead()
		entry, _ := s.conversation(tg, lead)
		if sentiment != "" {
			s.analysis(analysis{entryType: tg, entryID: entry, sentiment: sentiment, occurred: hoursAgo(i + 1)})
		}
		if sentiment == "angry" {
			sentiment = ""
		}
		want[lead] = sentiment
	}
	run := func() int {
		visited, after := 0, ""
		for {
			next, n, err := RefreshChunk(context.Background(), db, s.ws, after, 2)
			if err != nil {
				t.Fatal(err)
			}
			visited += n
			if next == "" {
				return visited
			}
			after = next
		}
	}
	if visited := run(); visited != len(want) {
		t.Fatalf("visited %d leads, want %d", visited, len(want))
	}
	var before []string
	db.Raw("SELECT lead_id::text || sentiment || refreshed_at::text FROM lead_sentiments ORDER BY lead_id").Scan(&before)
	run()
	var after []string
	db.Raw("SELECT lead_id::text || sentiment || refreshed_at::text FROM lead_sentiments ORDER BY lead_id").Scan(&after)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatalf("a second run moved rows:\n%v\n%v", before, after)
	}
	for lead, sentiment := range want {
		wantSentiment(t, s, lead, sentiment)
	}
}
