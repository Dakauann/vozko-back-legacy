package agent

import "testing"

func TestASnapshotPinsBehaviourAndToolRevisions(t *testing.T) {
	draft := &Agent{ID: "a", Name: "Ana", MessagingPrompt: "p1", MessagingModel: "m", IsActive: true, DepartmentID: "d",
		InternalTools: []ToolBinding{{ID: "t1", Name: "http_request", Revision: 3}}}
	s := SnapshotOf(draft)
	if len(s.Tools) != 1 || s.Tools[0] != (ToolPin{ID: "t1", Revision: 3}) || s.MessagingPrompt != "p1" {
		t.Fatalf("snapshot %+v", s)
	}
	if !s.Same(SnapshotOf(draft)) {
		t.Fatal("the same draft gives the same snapshot")
	}
	changed := *draft
	changed.MessagingPrompt = "p2"
	if s.Same(SnapshotOf(&changed)) {
		t.Fatal("a prompt change must show as a draft change")
	}
	live := &Agent{ID: "a", Name: "Ana nova", MessagingPrompt: "p9", IsActive: false, DepartmentID: "d2"}
	published := s.Apply(live, []ToolBinding{{ID: "t1", Revision: 3}})
	if published.MessagingPrompt != "p1" || published.IsActive || published.DepartmentID != "d2" || len(published.InternalTools) != 1 {
		t.Fatalf("published %+v", published)
	}
}
