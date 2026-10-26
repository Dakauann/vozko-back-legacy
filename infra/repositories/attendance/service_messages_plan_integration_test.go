package attendance_repository

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/infra/database"
	"vozko/infra/repositories/repotest"
)

func TestTheServiceMessageCountProbesThePartialIndexPerConversationAgainstPostgres(t *testing.T) {
	db := repotest.IsolatedDB(t, "att_svcplan", attendanceModels()...)
	day := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	seed := serviceMessageSeed{db: db, t: t, day: day}
	paying := seedAttendanceWorkspace(t, db, day)
	crowd := uuid.NewString()
	for workspaceID, conversations := range map[string]int{paying: 40, crowd: 400} {
		campaignID := seed.campaign(workspaceID, nil)
		seed.exec(`INSERT INTO whatsapp_campaign_entries (id, campaign_id, lead_id, conversation_status, last_message_at, created_at, updated_at)
			SELECT gen_random_uuid(), ?, gen_random_uuid(), 'ongoing', ?::timestamptz - (g % 40) * INTERVAL '1 day', ?::timestamptz - INTERVAL '60 days', ?::timestamptz - INTERVAL '60 days'
			FROM generate_series(1, ?) g`, campaignID, day, day, day, conversations)
		seed.exec(`INSERT INTO conversation_messages (id, entry_id, entry_type, message_type, direction, text, sender_kind, delivery_status, sent_via,
				meta_pricing_billable, meta_pricing_category, meta_conversation_origin, created_at, updated_at)
			SELECT gen_random_uuid(), e.id, 'whatsapp',
				CASE WHEN g % 3 = 0 THEN 'text' ELSE 'operator' END,
				CASE WHEN g % 3 = 0 THEN 'INBOUND' ELSE 'OUTBOUND' END,
				'oi', 'human', 'delivered', '',
				CASE WHEN g % 5 = 0 THEN TRUE ELSE NULL END,
				CASE WHEN g % 5 = 0 THEN 'service' ELSE '' END,
				'', e.last_message_at - g * INTERVAL '1 minute', e.last_message_at
			FROM whatsapp_campaign_entries e CROSS JOIN generate_series(1, 12) g
			WHERE e.campaign_id = ?`, campaignID)
	}
	ddl, ok := database.ConcurrentIndexSQL(database.ChargedServiceMessageIndex)
	if !ok {
		t.Fatalf("%s is not declared", database.ChargedServiceMessageIndex)
	}
	seed.exec(strings.Replace(ddl, "CONCURRENTLY ", "", 1))
	seed.exec(`ANALYZE conversation_messages, whatsapp_campaign_entries, whatsapp_campaigns, inbox_assignments`)

	var messages int64
	if err := db.Raw(`SELECT COUNT(*) FROM conversation_messages`).Scan(&messages).Error; err != nil {
		t.Fatal(err)
	}
	sql, args := serviceMessageCountQuery(paying, serviceMessageQueryFor("2026-10-04", "2026-10-10")).build()
	explain := func(settings ...string) string {
		t.Helper()
		var plan []string
		err := db.Transaction(func(tx *gorm.DB) error {
			for _, setting := range settings {
				if err := tx.Exec("SET LOCAL " + setting).Error; err != nil {
					return err
				}
			}
			return tx.Raw("EXPLAIN (ANALYZE, BUFFERS, COSTS OFF) "+sql, args...).Scan(&plan).Error
		})
		if err != nil {
			t.Fatalf("explain: %v", err)
		}
		return strings.Join(plan, "\n")
	}

	chosen := explain()
	t.Logf("%d messages seeded, plan the planner chose:\n%s", messages, chosen)
	if !strings.Contains(chosen, database.ChargedServiceMessageIndex) || strings.Contains(chosen, "Seq Scan on conversation_messages") {
		t.Fatalf("the count does not read through %s:\n%s", database.ChargedServiceMessageIndex, chosen)
	}

	perConversation := explain("join_collapse_limit = 1", "enable_hashjoin = off", "enable_mergejoin = off", "enable_material = off", "enable_memoize = off")
	t.Logf("plan driven from the workspace conversations:\n%s", perConversation)
	if !strings.Contains(perConversation, "Scan using "+database.ChargedServiceMessageIndex) ||
		!strings.Contains(perConversation, "(entry_id = wce.id)") {
		t.Fatalf("a conversation first plan does not probe %s by conversation:\n%s", database.ChargedServiceMessageIndex, perConversation)
	}
}
