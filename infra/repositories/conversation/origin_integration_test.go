package conversation_repository

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/infra/database"
	"vozko/infra/database/schema"
	"vozko/infra/database/sqlexpr"
	"vozko/infra/repositories/repotest"
)

var originClock = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func originDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := repotest.IsolatedDB(t, "conv_origin", &schema.ConversationMessage{}, &schema.ConversationOrigin{})
	sql, ok := database.PerformanceIndexSQL("ux_cm_entry_external_msgid")
	if !ok {
		t.Fatal("ux_cm_entry_external_msgid is not declared")
	}
	if err := db.Exec(sql).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func originMessage(entryID string, sentBy conversation.SentBy, at time.Time) *conversation.Message {
	return &conversation.Message{
		ID: uuid.NewString(), EntryID: entryID, EntryType: shared.EntryTypeWhatsApp, Channel: conversation.MessageChannelWhatsApp,
		MessageType: conversation.MessageTypeUserMessage, From: "x", Text: "oi", SentBy: sentBy, CreatedAt: at, UpdatedAt: at,
	}
}

type storedOrigin struct {
	Origin         string
	FirstMessageID string
}

func originOf(t *testing.T, db *gorm.DB, entryID string) storedOrigin {
	t.Helper()
	var rows []storedOrigin
	if err := db.Raw(`SELECT origin, first_message_id FROM conversation_origins WHERE entry_id = ? AND entry_type = 'whatsapp'`, entryID).
		Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("conversation %s has %d origin rows, want exactly one", entryID, len(rows))
	}
	return rows[0]
}

func createAll(t *testing.T, repo conversation.MessageRepository, messages ...*conversation.Message) {
	t.Helper()
	for _, m := range messages {
		if err := repo.Create(m); err != nil {
			t.Fatalf("Create(%s): %v", m.SentBy.Kind(), err)
		}
	}
}

func TestTheFirstSenderDecidesWhereAConversationCameFromAgainstPostgres(t *testing.T) {
	cases := map[string]struct {
		senders []conversation.SentBy
		want    conversation.Origin
	}{
		"the contact wrote first":            {[]conversation.SentBy{conversation.SentByContact("5511"), conversation.SentByPerson(uuid.NewString())}, conversation.OriginReceptive},
		"a campaign opened it":               {[]conversation.SentBy{conversation.SentByCampaign("camp-1"), conversation.SentByContact("5511")}, conversation.OriginCampaign},
		"the team started it from the inbox": {[]conversation.SentBy{conversation.SentByPerson(uuid.NewString()), conversation.SentByContact("5511")}, conversation.OriginOutreach},
		"an automation started it":           {[]conversation.SentBy{conversation.SentByWorkflow("wf-1"), conversation.SentByContact("5511")}, conversation.OriginOutreach},
		"the business app echo came first":   {[]conversation.SentBy{conversation.SentExternally(), conversation.SentByContact("5511")}, conversation.OriginOutreach},
		"a system notice never opens":        {[]conversation.SentBy{conversation.SentBySystem(), conversation.SentByContact("5511")}, conversation.OriginReceptive},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			db := originDB(t)
			repo := NewRepository(db)
			entryID := uuid.NewString()
			var first string
			for i, sender := range tc.senders {
				m := originMessage(entryID, sender, originClock.Add(time.Duration(i)*time.Minute))
				createAll(t, repo, m)
				if first == "" && sender.Kind().OpensConversation() {
					first = m.ID
				}
			}
			got := originOf(t, db, entryID)
			if got.Origin != string(tc.want) || got.FirstMessageID != first {
				t.Fatalf("origin = %+v, want %s from %s", got, tc.want, first)
			}
		})
	}
}

func TestAnImportSeedNeverDecidesTheOriginAgainstPostgres(t *testing.T) {
	db := originDB(t)
	repo := NewRepository(db)
	entryID := uuid.NewString()
	seed := originMessage(entryID, conversation.SentBySystem(), originClock)
	seed.MessageType = conversation.MessageTypeSystem
	seed.Metadata, _ = json.Marshal(map[string]string{conversation.SeedMetadataKey: conversation.SeedSourceLeadImport})
	reply := originMessage(entryID, conversation.SentByContact("5511"), originClock.Add(time.Hour))
	createAll(t, repo, seed, reply)

	if got := originOf(t, db, entryID); got.Origin != string(conversation.OriginReceptive) || got.FirstMessageID != reply.ID {
		t.Fatalf("origin = %+v, want receptive from the contact's message", got)
	}
}

func TestHistoryStoredBeforeTheOriginExistedDecidesItAgainstPostgres(t *testing.T) {
	db := originDB(t)
	entryID := uuid.NewString()
	legacyID := uuid.NewString()
	if err := db.Exec(`INSERT INTO conversation_messages (id, entry_id, entry_type, message_type, text, sender_kind, created_at, updated_at)
		VALUES (?, ?, 'whatsapp', 'user_message', 'antes', 'contact', ?, ?)`, legacyID, entryID, originClock, originClock).Error; err != nil {
		t.Fatal(err)
	}
	createAll(t, NewRepository(db), originMessage(entryID, conversation.SentByPerson(uuid.NewString()), originClock.Add(24*time.Hour)))

	if got := originOf(t, db, entryID); got.Origin != string(conversation.OriginReceptive) || got.FirstMessageID != legacyID {
		t.Fatalf("origin = %+v, want receptive from the stored history", got)
	}
}

func TestAnUnattributedFirstMessageFailsClosedAgainstPostgres(t *testing.T) {
	db := originDB(t)
	entryID := uuid.NewString()
	if err := db.Exec(`INSERT INTO conversation_messages (id, entry_id, entry_type, message_type, text, created_at, updated_at)
		VALUES (?, ?, 'whatsapp', 'user_message', 'antigo', ?, ?)`, uuid.NewString(), entryID, originClock, originClock).Error; err != nil {
		t.Fatal(err)
	}
	createAll(t, NewRepository(db), originMessage(entryID, conversation.SentByContact("5511"), originClock.Add(time.Minute)))

	if got := originOf(t, db, entryID); got.Origin != string(conversation.OriginUnknown) {
		t.Fatalf("origin = %+v: an unknown sender must never be skipped into receptive", got)
	}
}

func TestAnOlderMessageArrivingLaterTakesOverTheOriginAgainstPostgres(t *testing.T) {
	db := originDB(t)
	repo := NewRepository(db)
	entryID := uuid.NewString()
	live := originMessage(entryID, conversation.SentByPerson(uuid.NewString()), originClock)
	imported := originMessage(entryID, conversation.SentByContact("5511"), originClock.Add(-48*time.Hour))
	later := originMessage(entryID, conversation.SentByCampaign("camp-1"), originClock.Add(time.Hour))
	createAll(t, repo, live, imported, later)

	if got := originOf(t, db, entryID); got.Origin != string(conversation.OriginReceptive) || got.FirstMessageID != imported.ID {
		t.Fatalf("origin = %+v, want the imported contact message that came first", got)
	}
}

func TestACampaignSendClaimingItsEchoMovesTheOriginAgainstPostgres(t *testing.T) {
	db := originDB(t)
	repo := NewRepository(db)
	entryID := uuid.NewString()
	external := "3EB0ECHO"
	echo := originMessage(entryID, conversation.SentExternally(), originClock)
	echo.ExternalMessageID = &external
	send := originMessage(entryID, conversation.SentByCampaign("camp-1"), originClock)
	send.ExternalMessageID = &external
	createAll(t, repo, echo)
	if got := originOf(t, db, entryID); got.Origin != string(conversation.OriginOutreach) {
		t.Fatalf("before the claim origin = %+v, want outreach", got)
	}
	createAll(t, repo, send)

	if got := originOf(t, db, entryID); got.Origin != string(conversation.OriginCampaign) || got.FirstMessageID != echo.ID {
		t.Fatalf("after the claim origin = %+v, want campaign on the claimed row", got)
	}
}

func TestTheOriginTableIsTheOneTheFiltersRead(t *testing.T) {
	if got := (schema.ConversationOrigin{}).TableName(); got != sqlexpr.ConversationOriginTable {
		t.Fatalf("schema table %q, filters read %q", got, sqlexpr.ConversationOriginTable)
	}
}
