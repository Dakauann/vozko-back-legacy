package database

import (
	"encoding/json"
	"testing"
)

func TestTheToolInstanceRepairIsDeclared(t *testing.T) {
	for _, name := range repairNames() {
		if name == "ag_tool_instances" {
			return
		}
	}
	t.Fatal("the tool instance repair must run at boot")
}

func TestLegacyBindingsBecomeInstancesWithTodaysNames(t *testing.T) {
	raw := `{"bindings":[
		{"name":"http_request","config":{"url":"https://a.test","method":"GET"}},
		{"name":"run_workflow","config":{"workflow_id":"669a1bad-1111-4222-8333-444455556666"}},
		{"name":"run_workflow","config":{"workflow_id":"669a1bad-1111-4222-8333-444455556666"}},
		{"name":"add_sheet_row","config":{"sheet_id":"sheet-1"}},
		{"name":"conversation_analysis"},
		{"name":"manage_lead_memory","visibility":["messaging"]}
	]}`
	got := legacyInstancesOf("agent-1", raw)
	if len(got) != 4 {
		t.Fatalf("got %+v", got)
	}
	want := []struct{ name, key string }{
		{"http_request", ""},
		{"run_workflow_669a1bad-1111-4222-8333-444455556666", "target:669a1bad-1111-4222-8333-444455556666"},
		{"add_sheet_row_sheet-1", "target:sheet-1"},
		{"manage_lead_memory", "single"},
	}
	for i, w := range want {
		if got[i].Name != w.name || got[i].TargetKey != w.key || got[i].Position != i {
			t.Fatalf("instance %d = %+v, want %+v", i, got[i], w)
		}
	}
	var visibility []string
	if err := json.Unmarshal([]byte(got[3].Visibility), &visibility); err != nil || len(visibility) != 1 {
		t.Fatalf("visibility %s", got[3].Visibility)
	}
}

func TestInstanceIDsAreStableSoTheRepairCanRunTwice(t *testing.T) {
	raw := `{"bindings":[{"name":"http_request","config":{"url":"https://a.test"}}]}`
	first, second := legacyInstancesOf("agent-1", raw), legacyInstancesOf("agent-1", raw)
	if first[0].ID != second[0].ID || first[0].ID == legacyInstancesOf("agent-2", raw)[0].ID {
		t.Fatalf("ids %s %s", first[0].ID, second[0].ID)
	}
}
