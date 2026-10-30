package conversation

import "testing"

func TestAnInboxViewListsOnlyTheConversationsItsTabStandsFor(t *testing.T) {
	cases := []struct {
		name   string
		view   InboxView
		status ConversationStatus
		want   bool
	}{
		{"active tab keeps a new conversation", InboxView{}, ConversationStatusNew, true},
		{"active tab keeps an ongoing conversation", InboxView{}, ConversationStatusOngoing, true},
		{"active tab drops a finished conversation", InboxView{}, ConversationStatusFinished, false},
		{"groups tab keeps a finished group", InboxView{Groups: GroupScopeGroups}, ConversationStatusFinished, true},
		{"finished tab keeps a finished conversation", InboxView{Status: ConversationStatusFinished}, ConversationStatusFinished, true},
		{"finished tab drops a reopened conversation", InboxView{Status: ConversationStatusFinished}, ConversationStatusNew, false},
		{"new tab drops an ongoing conversation", InboxView{Status: ConversationStatusNew}, ConversationStatusOngoing, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.view.Includes(c.status); got != c.want {
				t.Fatalf("Includes(%q) = %v, want %v", c.status, got, c.want)
			}
		})
	}
}

func TestOnlyTheActiveClientTabLeavesFinishedConversationsOut(t *testing.T) {
	if !(InboxView{}).ExcludesFinished() {
		t.Fatal("the active tab must leave finished conversations out")
	}
	if (InboxView{Groups: GroupScopeGroups}).ExcludesFinished() {
		t.Fatal("the groups tab lists finished groups")
	}
	if (InboxView{Status: ConversationStatusFinished}).ExcludesFinished() {
		t.Fatal("a status tab filters by its own status")
	}
}
