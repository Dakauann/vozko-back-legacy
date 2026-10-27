package conversation_repository

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

type groupSeed struct {
	db        *gorm.DB
	workspace string
	instance  string
}

func seedGroupInbox(t *testing.T) groupSeed {
	t.Helper()
	db := repotest.IsolatedDB(t, "group_scope", append(append(repotest.ConversationVisibilityModels(), repotest.LeadStoreModels()...), &schema.ConversationMessage{})...)
	repotest.DropForeignKeys(t, db)
	exec := execOrFail(t, db)
	workspace := uuid.NewString()
	exec(`INSERT INTO unofficial_whatsapp_instances (id, workspace_id, server_id, provider_instance_id, delivery_token_hash, status)
		SELECT gen_random_uuid(), CASE WHEN n = 0 THEN ?::uuid ELSE gen_random_uuid() END, gen_random_uuid(), 'inst-' || n, 'hash-' || n, 'CONNECTED'
		FROM generate_series(0, 49) AS n`, workspace)
	exec(`INSERT INTO unofficial_whatsapp_conversations (id, workspace_id, instance_id, contact_id, chat_id, is_group, campaign_id, conversation_status, last_message_at)
		SELECT gen_random_uuid(), uwi.workspace_id, uwi.id, gen_random_uuid(),
		       CASE WHEN n % 7 = 0 THEN n || '@g.us' ELSE n || '@s.whatsapp.net' END,
		       n % 7 = 0, '',
		       (ARRAY['new','ongoing','finished'])[1 + (n % 3)],
		       now() - n * interval '1 minute'
		FROM generate_series(1, 5000) AS n
		JOIN unofficial_whatsapp_instances uwi ON uwi.provider_instance_id = 'inst-' || (n % 50)`)
	exec(`INSERT INTO unofficial_whatsapp_contacts (id, workspace_id, instance_id, jid, is_group)
		SELECT contact_id, workspace_id, instance_id, chat_id, is_group FROM unofficial_whatsapp_conversations`)
	exec(`ANALYZE unofficial_whatsapp_instances`)
	exec(`ANALYZE unofficial_whatsapp_conversations`)
	var instance string
	if err := db.Raw(`SELECT id::text FROM unofficial_whatsapp_instances WHERE workspace_id = ?`, workspace).Scan(&instance).Error; err != nil {
		t.Fatal(err)
	}
	return groupSeed{db: db, workspace: workspace, instance: instance}
}

func (s groupSeed) count(t *testing.T, sql string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	if err := s.db.Raw(sql, args...).Scan(&n).Error; err != nil {
		t.Fatalf("count: %v\n%s", err, sql)
	}
	return n
}

func (s groupSeed) expected(t *testing.T, where string) int64 {
	t.Helper()
	return s.count(t, `SELECT COUNT(*) FROM unofficial_whatsapp_conversations WHERE workspace_id = ? AND deleted_at IS NULL AND `+where, s.workspace)
}

func TestTheWorkspaceInboxSeparatesGroupsFromClientsOnAModestSeed(t *testing.T) {
	seed := seedGroupInbox(t)
	cases := []struct {
		name   string
		scope  conversation.GroupScope
		status conversation.ConversationStatus
		where  string
	}{
		{"client tabs", conversation.GroupScopeClients, "", "NOT is_group AND conversation_status <> 'finished'"},
		{"client Novos tab", conversation.GroupScopeClients, conversation.ConversationStatusNew, "NOT is_group AND conversation_status = 'new'"},
		{"client Finalizados tab", conversation.GroupScopeClients, conversation.ConversationStatusFinished, "NOT is_group AND conversation_status = 'finished'"},
		{"groups tab", conversation.GroupScopeGroups, "", "is_group"},
		{"groups tab while hidden", conversation.GroupScopeNone, "", "false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := seed.expected(t, c.where)
			queries, ok := workspaceInboxQueries(conversation.SearchEntriesInput{
				WorkspaceID: seed.workspace, Groups: c.scope, ConversationStatus: c.status,
			}, 20, 0)
			if !ok {
				if want != 0 {
					t.Fatalf("no query, want %d rows", want)
				}
				return
			}
			if got := seed.count(t, queries.countSQL, queries.countArgs...); got != want {
				t.Fatalf("count = %d, want %d", got, want)
			}
			var rows []workspaceInboxRow
			if err := seed.db.Raw(queries.pageSQL, queries.pageArgs...).Scan(&rows).Error; err != nil {
				t.Fatal(err)
			}
			if len(rows) == 0 || rows[0].Total != want {
				t.Fatalf("the page and its total disagree with the count: %d rows, total %+v, want %d", len(rows), rows, want)
			}
		})
	}
}

func TestTheAccountInboxSeparatesGroupsFromClientsOnAModestSeed(t *testing.T) {
	seed := seedGroupInbox(t)
	ch, _ := channelQueryFor(shared.EntryTypeUnofficialWhatsApp)
	var ids []string
	if err := seed.db.Raw(`SELECT id::text FROM unofficial_whatsapp_conversations WHERE instance_id = ?`, seed.instance).Scan(&ids).Error; err != nil {
		t.Fatal(err)
	}
	exec := execOrFail(t, seed.db)
	exec(`INSERT INTO conversation_messages (id, entry_id, entry_type, created_at)
		SELECT gen_random_uuid(), id, 'unofficial_whatsapp', now() FROM unofficial_whatsapp_conversations WHERE instance_id = ?`, seed.instance)
	for _, c := range []struct {
		scope conversation.GroupScope
		where string
	}{
		{conversation.GroupScopeClients, "NOT is_group AND conversation_status <> 'finished'"},
		{conversation.GroupScopeGroups, "is_group"},
		{conversation.GroupScopeNone, "false"},
	} {
		set := containerInboxSet(conversation.SearchEntriesInput{EntryType: shared.EntryTypeUnofficialWhatsApp, Groups: c.scope}, ch, ids)
		got := seed.count(t, "SELECT COUNT(*) FROM ("+set.rows+") AS m", set.rowsArgs...)
		want := seed.count(t, `SELECT COUNT(*) FROM unofficial_whatsapp_conversations WHERE instance_id = ? AND `+c.where, seed.instance)
		if got != want {
			t.Errorf("%q: account inbox count = %d, want %d", c.scope, got, want)
		}
	}
}

func TestTheStatusTabCountsLeaveGroupsToTheirOwnBadgeOnAModestSeed(t *testing.T) {
	seed := seedGroupInbox(t)
	table := ChannelConversationTable{DB: seed.db, NewModel: func() any { return &unofficialRow{} }, ContainerColumn: "instance_id", GroupColumn: UnofficialGroupColumn}
	counts, err := table.CountByStatus(t.Context(), seed.workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"new", "ongoing", "finished"} {
		if want := seed.expected(t, "NOT is_group AND conversation_status = '"+status+"'"); counts[status] != want {
			t.Errorf("%s = %d, want %d", status, counts[status], want)
		}
	}
	if want := seed.expected(t, "is_group"); counts[conversation.GroupsStatusCountKey] != want {
		t.Errorf("groups = %d, want %d", counts[conversation.GroupsStatusCountKey], want)
	}
}

type unofficialRow struct{}

func (unofficialRow) TableName() string { return "unofficial_whatsapp_conversations" }

func TestTheGroupPredicateIsACheapFilterOnTheRowAlreadyReadOnAModestSeed(t *testing.T) {
	seed := seedGroupInbox(t)
	for _, scope := range []conversation.GroupScope{conversation.GroupScopeClients, conversation.GroupScopeGroups} {
		queries, _ := workspaceInboxQueries(conversation.SearchEntriesInput{
			WorkspaceID: seed.workspace, EntryType: shared.EntryTypeUnofficialWhatsApp, Groups: scope,
		}, 20, 0)
		plan := explain(t, seed.db, queries.pageSQL, queries.pageArgs...)
		t.Logf("%q inbox page plan:\n%s", scope, plan)
		if strings.Contains(plan, "Seq Scan on unofficial_whatsapp_conversations") {
			t.Errorf("%q: the inbox must not scan every unofficial conversation:\n%s", scope, plan)
		}
		if strings.Count(plan, " on unofficial_whatsapp_conversations ") != 1 || strings.Contains(plan, "SubPlan") {
			t.Errorf("%q: the group flag must be read from the row already scanned, not a second probe:\n%s", scope, plan)
		}
	}
}
