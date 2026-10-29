package tutorial

import "testing"

func TestTheSitesChecklistFollowsTheBuildOrder(t *testing.T) {
	list := SitesChecklist(SiteFacts{HasPage: true, FormToFunnel: true, Published: true})
	want := []Item{
		{ID: "create_page", Done: true},
		{ID: "edit_by_hand", Done: false},
		{ID: "form_to_funnel", Done: true},
		{ID: "site_chat", Done: false},
		{ID: "booking", Done: false},
		{ID: "custom_domain", Done: false},
		{ID: "publish", Done: true},
	}
	if list.Area != AreaSites || len(list.Items) != len(want) {
		t.Fatalf("list = %+v", list)
	}
	for i := range want {
		if list.Items[i] != want[i] {
			t.Errorf("item %d = %+v, want %+v", i, list.Items[i], want[i])
		}
	}
	if list.DoneCount() != 3 {
		t.Fatalf("done = %d, want 3", list.DoneCount())
	}
}

func TestAnEmptyWorkspaceHasNothingDone(t *testing.T) {
	list := SitesChecklist(SiteFacts{})
	if list.DoneCount() != 0 || len(list.Items) != 7 {
		t.Fatalf("list = %+v", list)
	}
}
