package agent

import "testing"

func TestAListedAgentCarriesItsDescriptionForPickers(t *testing.T) {
	described := "Atende alunos matriculados."
	if item := mapAgentRowToListItem(&agentListRow{ID: "ag-1", Description: &described}); item.Description != described {
		t.Fatalf("item = %+v", item)
	}
	if item := mapAgentRowToListItem(&agentListRow{ID: "ag-2"}); item.Description != "" {
		t.Fatalf("an agent without a description lists an empty one: %+v", item)
	}
}
