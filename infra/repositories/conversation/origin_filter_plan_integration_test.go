package conversation_repository

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/conversation"
	"vozko/domain/crmfilter"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
	"vozko/infra/database/sqlexpr"
	"vozko/infra/repositories/repotest"
)

func TestTheOriginFilterProbesOriginsByPrimaryKeyOnAModestSeed(t *testing.T) {
	db := repotest.IsolatedDB(t, "origin_plan", append(append(repotest.ConversationVisibilityModels(), repotest.LeadStoreModels()...), &schema.ConversationOrigin{})...)
	repotest.DropForeignKeys(t, db)
	exec := execOrFail(t, db)
	workspace := uuid.NewString()
	exec(`INSERT INTO telegram_accounts (id, workspace_id, bot_user_id)
		SELECT gen_random_uuid(), CASE WHEN n = 0 THEN ?::uuid ELSE gen_random_uuid() END, n FROM generate_series(0, 49) AS n`, workspace)
	exec(`INSERT INTO telegram_conversations (id, workspace_id, account_id, contact_id, tg_chat_id, conversation_status, last_message_at)
		SELECT gen_random_uuid(), tga.workspace_id, tga.id, gen_random_uuid(), n, 'ongoing', now() - n * interval '1 minute'
		FROM generate_series(1, 5000) AS n JOIN telegram_accounts tga ON tga.bot_user_id = n % 50`)
	exec(`INSERT INTO conversation_origins (entry_id, entry_type, origin, first_message_id, first_message_at)
		SELECT id, 'telegram', (ARRAY['receptive','campaign','outreach'])[1 + (tg_chat_id % 3)], gen_random_uuid()::text, last_message_at
		FROM telegram_conversations`)
	exec(`ANALYZE telegram_accounts`)
	exec(`ANALYZE telegram_conversations`)
	exec(`ANALYZE conversation_origins`)

	queries, ok := workspaceInboxQueries(conversation.SearchEntriesInput{
		WorkspaceID: workspace, EntryType: shared.EntryTypeTelegram, Origin: conversation.OriginReceptive,
	}, 20, 0)
	if !ok {
		t.Fatal("no workspace inbox queries")
	}
	plan := explain(t, db, queries.pageSQL, queries.pageArgs...)
	t.Logf("origin filtered inbox page plan:\n%s", plan)
	if strings.Contains(plan, "Seq Scan on conversation_origins") || !strings.Contains(plan, "conversation_origins_pkey") {
		t.Fatalf("the origin filter must probe conversation_origins by its primary key:\n%s", plan)
	}

	board, err := filteredPageQueries(conversation.SearchByFilterInput{
		WorkspaceID: workspace,
		Filter: crmfilter.Filter{Groups: []crmfilter.Group{{Conjunction: crmfilter.And, Predicates: []crmfilter.Predicate{
			{Field: crmfilter.FieldOrigin, Operator: crmfilter.OpIn, Values: []string{"receptive"}},
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	boardPlan := explain(t, db, board.pageSQL, board.pageArgs...)
	t.Logf("origin filtered board page plan:\n%s", boardPlan)
	if strings.Contains(boardPlan, "Seq Scan on conversation_origins") || !strings.Contains(boardPlan, "conversation_origins_pkey") {
		t.Fatalf("the board origin filter must probe conversation_origins by its primary key:\n%s", boardPlan)
	}

	set, _ := workspaceInboxSet(conversation.SearchEntriesInput{WorkspaceID: workspace, EntryType: shared.EntryTypeTelegram})
	hydrate := set.with + ", page AS (SELECT entry_id, entry_type, lm_created_at FROM entries_with_msg ORDER BY lm_created_at DESC LIMIT 20)" +
		" SELECT page.entry_id, COALESCE(co.origin, '') FROM page " + sqlexpr.ConversationOriginJoin("co", "page.entry_id", "page.entry_type")
	readPlan := explain(t, db, hydrate, set.withArgs...)
	t.Logf("origin read plan:\n%s", readPlan)
	if strings.Contains(readPlan, "Seq Scan on conversation_origins") {
		t.Fatalf("reading the origin of a listed page must not scan every origin:\n%s", readPlan)
	}
}

func explain(t *testing.T, db *gorm.DB, sql string, args ...interface{}) string {
	t.Helper()
	var lines []string
	if err := db.Raw("EXPLAIN "+sql, args...).Scan(&lines).Error; err != nil {
		t.Fatalf("explain: %v", err)
	}
	return strings.Join(lines, "\n")
}
