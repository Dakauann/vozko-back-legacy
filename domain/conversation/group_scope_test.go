package conversation

import "testing"

func TestTheInboxShowsClientsUnlessTheGroupsTabIsAsked(t *testing.T) {
	cases := []struct {
		name        string
		wantsGroups bool
		hidesGroups bool
		want        GroupScope
	}{
		{"client tabs", false, false, GroupScopeClients},
		{"client tabs while groups are hidden", false, true, GroupScopeClients},
		{"groups tab", true, false, GroupScopeGroups},
		{"groups tab while groups are hidden", true, true, GroupScopeNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := InboxGroupScope(c.wantsGroups, c.hidesGroups); got != c.want {
				t.Fatalf("InboxGroupScope(%v, %v) = %q, want %q", c.wantsGroups, c.hidesGroups, got, c.want)
			}
		})
	}
}

func TestTheZeroGroupScopeListsClientsOnly(t *testing.T) {
	var zero GroupScope
	if zero != GroupScopeClients {
		t.Fatalf("the zero scope must be the client tabs, got %q", zero)
	}
	if !zero.Lists(false) || zero.Lists(true) {
		t.Fatal("the client tabs list client conversations and never groups")
	}
}

func TestEachGroupScopeListsItsOwnConversations(t *testing.T) {
	cases := []struct {
		scope  GroupScope
		client bool
		group  bool
	}{
		{GroupScopeClients, true, false},
		{GroupScopeGroups, false, true},
		{GroupScopeNone, false, false},
		{GroupScope("unknown"), true, false},
	}
	for _, c := range cases {
		if got := c.scope.Lists(false); got != c.client {
			t.Errorf("%q lists a client conversation = %v, want %v", c.scope, got, c.client)
		}
		if got := c.scope.Lists(true); got != c.group {
			t.Errorf("%q lists a group = %v, want %v", c.scope, got, c.group)
		}
	}
}

func TestAnEntryReachesAViewerListedElsewhereOrNotAtAll(t *testing.T) {
	cases := []struct {
		name        string
		isGroup     bool
		scope       GroupScope
		hidesGroups bool
		want        EntryReach
	}{
		{"client on the client tabs", false, GroupScopeClients, false, EntryListed},
		{"group on the groups tab", true, GroupScopeGroups, false, EntryListed},
		{"group on the client tabs", true, GroupScopeClients, false, EntryElsewhere},
		{"client on the groups tab", false, GroupScopeGroups, false, EntryElsewhere},
		{"group for someone who hides groups", true, GroupScopeClients, true, EntryUnseen},
		{"client for someone who hides groups", false, GroupScopeClients, true, EntryListed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ReachOf(c.isGroup, c.scope, c.hidesGroups); got != c.want {
				t.Fatalf("ReachOf = %v, want %v", got, c.want)
			}
		})
	}
}

func TestStatusCountsKeepGroupsOutOfTheClientTabs(t *testing.T) {
	counts := FoldStatusCounts([]StatusTally{
		{Status: "new", Count: 3},
		{Status: "ongoing", Count: 2},
		{Status: "finished", Count: 7},
		{Status: "new", Group: true, Count: 4},
		{Status: "ongoing", Group: true, Count: 1},
		{Status: "finished", Group: true, Count: 9},
	})
	want := map[string]int64{"new": 3, "ongoing": 2, "finished": 7, GroupsStatusCountKey: 14}
	if len(counts) != len(want) {
		t.Fatalf("counts = %v, want %v", counts, want)
	}
	for k, v := range want {
		if counts[k] != v {
			t.Fatalf("counts[%q] = %d, want %d (all: %v)", k, counts[k], v, counts)
		}
	}
}

func TestOnlyTheGroupsTabListsFinishedConversationsWithoutAskingForThem(t *testing.T) {
	if !GroupScopeGroups.ListsFinished() {
		t.Error("the groups tab lists every group, finished or not")
	}
	if GroupScopeClients.ListsFinished() || GroupScopeNone.ListsFinished() {
		t.Error("the client tabs keep finished conversations under Finalizados")
	}
}

func TestStatusCountsWithoutGroupsCarryNoGroupsKey(t *testing.T) {
	counts := FoldStatusCounts([]StatusTally{{Status: "new", Count: 1}})
	if _, ok := counts[GroupsStatusCountKey]; ok {
		t.Fatalf("a channel without groups must not report a groups count: %v", counts)
	}
}
