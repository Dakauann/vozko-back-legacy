package conversation_repository

import (
	"strings"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

const (
	clientsOnly = "uwc.is_group IS NOT TRUE"
	groupsOnly  = "uwc.is_group IS TRUE"
)

func TestOnlyUnofficialWhatsAppHasGroups(t *testing.T) {
	for _, q := range channelQueries {
		want := ""
		if q.EntryType == shared.EntryTypeUnofficialWhatsApp {
			want = "uwc.is_group"
		}
		if q.GroupColumn != want {
			t.Errorf("%s group column = %q, want %q", q.EntryType, q.GroupColumn, want)
		}
	}
}

func TestTheGroupColumnLivesOnTheSourceTableItself(t *testing.T) {
	for _, src := range entrySources {
		column := src.groupColumn()
		if column == "" {
			continue
		}
		alias := strings.SplitN(column, ".", 2)[0]
		if !strings.HasSuffix(src.From, " "+alias) {
			t.Errorf("%s reads %s but its table is %q", src.EntryType, column, src.From)
		}
	}
}

func TestGroupScopeConditionNeverHidesARowWithAnUnknownFlag(t *testing.T) {
	cases := []struct {
		scope  conversation.GroupScope
		column string
		want   string
	}{
		{conversation.GroupScopeClients, "uwc.is_group", clientsOnly},
		{conversation.GroupScopeClients, "", ""},
		{conversation.GroupScopeGroups, "uwc.is_group", groupsOnly},
		{conversation.GroupScopeGroups, "", "1 = 0"},
		{conversation.GroupScopeNone, "uwc.is_group", "1 = 0"},
		{conversation.GroupScopeNone, "", "1 = 0"},
	}
	for _, c := range cases {
		if got := groupScopeCondition(c.scope, c.column); got != c.want {
			t.Errorf("groupScopeCondition(%q, %q) = %q, want %q", c.scope, c.column, got, c.want)
		}
	}
}

func TestTheClientTabsDropGroupsInsideTheirOwnSource(t *testing.T) {
	for _, src := range entrySources {
		sql, args := src.inboxSelect(entrySourceScope{}, "ws-1")
		want := 0
		if src.groupColumn() != "" {
			want = 1
		}
		if got := strings.Count(sql, clientsOnly); got != want {
			t.Errorf("%s: the client predicate appears %d times, want %d:\n%s", src.EntryType, got, want, sql)
		}
		if strings.Contains(sql, groupsOnly) {
			t.Errorf("%s: the client tabs ask for groups:\n%s", src.EntryType, sql)
		}
		assertPlaceholdersMatchArgs(t, sql, args)
	}
}

func TestTheGroupsTabReadsOnlyChannelsThatHaveGroups(t *testing.T) {
	scope := entrySourceScope{Groups: conversation.GroupScopeGroups}
	selected := scope.selected()
	if len(selected) != 1 || selected[0].EntryType != shared.EntryTypeUnofficialWhatsApp {
		t.Fatalf("the groups tab must read unofficial WhatsApp only, got %d sources", len(selected))
	}
	sql, args := selected[0].inboxSelect(scope, "ws-1")
	if strings.Count(sql, groupsOnly) != 1 || strings.Contains(sql, clientsOnly) {
		t.Fatalf("the groups tab predicate is wrong:\n%s", sql)
	}
	if strings.Contains(sql, "IS DISTINCT FROM 'finished'") {
		t.Fatalf("the groups tab lists every group, finished or not:\n%s", sql)
	}
	assertPlaceholdersMatchArgs(t, sql, args)
}

func TestTheGroupsTabStillHonoursAnExplicitStatus(t *testing.T) {
	scope := entrySourceScope{Groups: conversation.GroupScopeGroups, ConversationStatus: "new", ExcludeFinished: true}
	sql, _ := scope.selected()[0].inboxSelect(scope, "ws-1")
	if !strings.Contains(sql, "uwc.conversation_status = ?") {
		t.Fatalf("an explicit status still filters the groups tab:\n%s", sql)
	}
}

func TestTheClientTabsKeepHidingFinishedConversations(t *testing.T) {
	scope := entrySourceScope{ExcludeFinished: true}
	for _, src := range scope.selected() {
		sql, _ := src.inboxSelect(scope, "ws-1")
		if src.ConversationStatus != "" && !strings.Contains(sql, "IS DISTINCT FROM 'finished'") {
			t.Errorf("%s client tabs lost the finished rule:\n%s", src.EntryType, sql)
		}
	}
}

func TestAHiddenGroupsTabReadsNothingAtAll(t *testing.T) {
	if got := (entrySourceScope{Groups: conversation.GroupScopeNone}).selected(); len(got) != 0 {
		t.Fatalf("a hidden groups tab must not query any source, got %d", len(got))
	}
	if _, ok := workspaceInboxQueries(conversation.SearchEntriesInput{WorkspaceID: "ws-1", Groups: conversation.GroupScopeNone}, 20, 0); ok {
		t.Fatal("a hidden groups tab must not build a query")
	}
}

func TestWorkspaceInboxCountsPagesAndStageCountsTheSameGroupScope(t *testing.T) {
	for _, scope := range []conversation.GroupScope{conversation.GroupScopeClients, conversation.GroupScopeGroups} {
		want := clientsOnly
		if scope == conversation.GroupScopeGroups {
			want = groupsOnly
		}
		input := conversation.SearchEntriesInput{WorkspaceID: "ws-1", Groups: scope}
		queries, ok := workspaceInboxQueries(input, 20, 0)
		if !ok {
			t.Fatalf("%q: no workspace inbox queries", scope)
		}
		set, _ := workspaceInboxSet(input)
		stageSQL, stageArgs := set.stageCountQuery("ws-1", []string{"s-1"})
		for name, q := range map[string]struct {
			sql  string
			args []interface{}
		}{"count": {queries.countSQL, queries.countArgs}, "page": {queries.pageSQL, queries.pageArgs}, "stages": {stageSQL, stageArgs}} {
			if strings.Count(q.sql, want) != 1 {
				t.Errorf("%q %s: want the group predicate once:\n%s", scope, name, q.sql)
			}
			assertPlaceholdersMatchArgs(t, q.sql, q.args)
		}
	}
}

func TestContainerInboxAppliesTheGroupScopeOnEveryChannel(t *testing.T) {
	for _, q := range channelQueries {
		t.Run(string(q.EntryType), func(t *testing.T) {
			for _, c := range []struct {
				scope conversation.GroupScope
				want  string
			}{
				{conversation.GroupScopeClients, groupScopeCondition(conversation.GroupScopeClients, q.GroupColumn)},
				{conversation.GroupScopeGroups, groupScopeCondition(conversation.GroupScopeGroups, q.GroupColumn)},
				{conversation.GroupScopeNone, "1 = 0"},
			} {
				conds, _ := containerInboxConditions(conversation.SearchEntriesInput{CampaignID: "c-1", EntryType: q.EntryType, Groups: c.scope}, q, nil, true)
				sql := strings.Join(conds, " AND ")
				if c.want == "" {
					if strings.Contains(sql, "is_group") {
						t.Errorf("%q: a channel without groups must not read a group flag:\n%s", c.scope, sql)
					}
				} else if !strings.Contains(sql, c.want) {
					t.Errorf("%q: want %q in:\n%s", c.scope, c.want, sql)
				}
				finishedRule := strings.Contains(sql, "IS DISTINCT FROM 'finished'")
				if q.StatusColumn != "" && finishedRule == (c.scope == conversation.GroupScopeGroups) {
					t.Errorf("%q: the finished rule is wrong:\n%s", c.scope, sql)
				}
				set := containerInboxSet(conversation.SearchEntriesInput{CampaignID: "c-1", EntryType: q.EntryType, Groups: c.scope}, q, nil)
				assertPlaceholdersMatchArgs(t, set.rows, set.rowsArgs)
			}
		})
	}
}

func TestTheBoardAndTableNeverListGroups(t *testing.T) {
	cte, args, err := filteredEntriesCTE(conversation.SearchByFilterInput{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(cte, clientsOnly) != 1 || strings.Contains(cte, groupsOnly) {
		t.Fatalf("the board must keep groups out:\n%s", cte)
	}
	assertPlaceholdersMatchArgs(t, cte, args)
}
