package conversation_repository

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/lib/pq"

	"vozko/domain/conversation"
	"vozko/domain/crmfilter"
	"vozko/infra/database/sqlexpr"
)

func bindsOrigins(args []interface{}, origins ...string) bool {
	want := pq.Array(append([]string{}, origins...))
	for _, arg := range args {
		if reflect.DeepEqual(arg, want) {
			return true
		}
	}
	return false
}

func TestEveryEntrySourceFiltersByOriginOnItsOwnPrimaryKey(t *testing.T) {
	for _, src := range entrySources {
		t.Run(string(src.EntryType), func(t *testing.T) {
			sql, args := src.inboxSelect(entrySourceScope{Origins: []conversation.Origin{conversation.OriginReceptive}}, "ws-1")
			predicate := sqlexpr.ConversationOriginIn(src.EntryID, "'"+string(src.EntryType)+"'")
			if strings.Count(sql, predicate) != 1 {
				t.Fatalf("the origin predicate must appear exactly once:\n%s", sql)
			}
			if !bindsOrigins(args, "receptive") {
				t.Fatalf("the origin is not bound: %v", args)
			}
			assertPlaceholdersMatchArgs(t, sql, args)
		})
	}
}

func TestNoOriginLeavesEveryEntrySourceUntouched(t *testing.T) {
	for _, src := range entrySources {
		sql, _ := src.inboxSelect(entrySourceScope{}, "ws-1")
		if strings.Contains(sql, sqlexpr.ConversationOriginTable) {
			t.Fatalf("%s reads origins without an origin filter:\n%s", src.EntryType, sql)
		}
	}
}

func TestWorkspaceInboxCountsAndPagesTheSameOriginFilter(t *testing.T) {
	input := conversation.SearchEntriesInput{WorkspaceID: "ws-1", Origin: conversation.OriginCampaign}
	queries, ok := workspaceInboxQueries(input, 20, 0)
	if !ok {
		t.Fatal("no workspace inbox queries")
	}
	for name, q := range map[string]struct {
		sql  string
		args []interface{}
	}{"count": {queries.countSQL, queries.countArgs}, "page": {queries.pageSQL, queries.pageArgs}} {
		if got := strings.Count(q.sql, "FROM "+sqlexpr.ConversationOriginTable); got != len(entrySources) {
			t.Errorf("%s: origin filtered in %d sources, want %d", name, got, len(entrySources))
		}
		assertPlaceholdersMatchArgs(t, q.sql, q.args)
	}
	set, _ := workspaceInboxSet(input)
	stageSQL, stageArgs := set.stageCountQuery("ws-1", []string{"s-1"})
	if !strings.Contains(stageSQL, "FROM "+sqlexpr.ConversationOriginTable) {
		t.Errorf("stage counts ignore the origin filter:\n%s", stageSQL)
	}
	assertPlaceholdersMatchArgs(t, stageSQL, stageArgs)
}

func TestContainerInboxFiltersByOriginOnEveryChannel(t *testing.T) {
	for _, q := range channelQueries {
		t.Run(string(q.EntryType), func(t *testing.T) {
			input := conversation.SearchEntriesInput{CampaignID: "c-1", EntryType: q.EntryType, Origin: conversation.OriginOutreach}
			set := containerInboxSet(input, q, nil)
			predicate := sqlexpr.ConversationOriginIn("entries.entry_id", "'"+string(q.EntryType)+"'::text")
			if strings.Count(set.rows, predicate) != 1 {
				t.Fatalf("the origin predicate must appear exactly once:\n%s", set.rows)
			}
			if !bindsOrigins(set.rowsArgs, "outreach") {
				t.Fatalf("the origin is not bound: %v", set.rowsArgs)
			}
			assertPlaceholdersMatchArgs(t, set.rows, set.rowsArgs)
		})
	}
}

func TestOriginClauseSkipsAnEmptyOrigin(t *testing.T) {
	if sql, args := originClause(nil, "x.id", "'telegram'"); sql != "" || args != nil {
		t.Fatalf("no origin scope must add nothing, got %q %v", sql, args)
	}
}

func TestAContradictoryOriginScopeMatchesNothing(t *testing.T) {
	sql, args := originClause([]conversation.Origin{}, "x.id", "'telegram'")
	if sql != sqlexpr.ConversationOriginIn("x.id", "'telegram'") || !bindsOrigins(args) {
		t.Fatalf("an empty scope must still filter, by an empty set: %q %v", sql, args)
	}
}

func TestTheBoardPushesItsOriginPredicatesIntoEverySource(t *testing.T) {
	cte, args, err := filteredEntriesCTE(conversation.SearchByFilterInput{
		WorkspaceID: "ws-1",
		Filter: crmfilter.Filter{Groups: []crmfilter.Group{{Conjunction: crmfilter.And, Predicates: []crmfilter.Predicate{
			{Field: crmfilter.FieldOrigin, Operator: crmfilter.OpNotIn, Values: []string{"campaign", "outreach"}},
			{Field: crmfilter.FieldLabel, Operator: crmfilter.OpIn, Values: []string{"6f1b5a3e-9a59-4d5e-8a43-2d3f0c4b1e77"}},
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range entrySources {
		if !strings.Contains(cte, sqlexpr.ConversationOriginIn(src.EntryID, "'"+string(src.EntryType)+"'")) {
			t.Errorf("%s does not filter its own origins", src.EntryType)
		}
	}
	if strings.Contains(cte, sqlexpr.ConversationOriginIn("ae.entry_id", "ae.entry_type")) {
		t.Errorf("the hoisted origin is applied again over the union:\n%s", cte)
	}
	if !bindsOrigins(args, "receptive", "unknown") || !strings.Contains(cte, "entry_labels") {
		t.Errorf("the scope or the remaining filter got lost: %v", args)
	}
	assertPlaceholdersMatchArgs(t, cte, args)
}

func TestTheBoardRefusesAnUnknownOriginBeforeQuerying(t *testing.T) {
	_, _, err := filteredEntriesCTE(conversation.SearchByFilterInput{
		WorkspaceID: "ws-1",
		Filter: crmfilter.Filter{Groups: []crmfilter.Group{{Predicates: []crmfilter.Predicate{
			{Field: crmfilter.FieldOrigin, Operator: crmfilter.OpIn, Values: []string{"inbound"}},
		}}}},
	})
	if !errors.Is(err, conversation.ErrOriginInvalid) {
		t.Fatalf("err = %v, want an invalid origin", err)
	}
}
