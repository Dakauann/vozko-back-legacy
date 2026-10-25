package agentturn

import (
	"context"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/agentrun"
	"vozko/domain/ai"
)

func TestAssembleNamesTheConversationTurnSoItsToolCallsReachTheAttendanceSession(t *testing.T) {
	ag := &agent.Agent{ID: "agent-1", WorkspaceID: "ws-1", RunningVersion: 5}
	cases := []struct {
		name string
		req  Request
		want ai.AgentTurn
	}{
		{
			"entry on the request",
			Request{Agent: ag, EntryID: "entry-1", EntryType: "instagram", CampaignID: "camp-1"},
			ai.AgentTurn{EntryID: "entry-1", EntryType: "instagram", AgentID: "agent-1", CampaignID: "camp-1", Source: "conversation", AgentVersion: 5},
		},
		{
			"entry only in the tool seed",
			Request{Agent: ag, ToolSeed: map[string]interface{}{"__entry_id": "entry-2", "__entry_type": "whatsapp"}},
			ai.AgentTurn{EntryID: "entry-2", EntryType: "whatsapp", AgentID: "agent-1", Source: "conversation", AgentVersion: 5},
		},
		{
			"a simulation runs the draft",
			Request{Agent: ag, Source: agentrun.SourceSimulator},
			ai.AgentTurn{AgentID: "agent-1", Source: "simulator"},
		},
		{
			"a workflow node",
			Request{Agent: ag, Source: agentrun.SourceWorkflow, EntryID: "entry-3", EntryType: "whatsapp"},
			ai.AgentTurn{EntryID: "entry-3", EntryType: "whatsapp", AgentID: "agent-1", Source: "workflow", AgentVersion: 5},
		},
		{
			"no agent",
			Request{EntryID: "entry-1", EntryType: "telegram"},
			ai.AgentTurn{EntryID: "entry-1", EntryType: "telegram"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := New(nil, nil, nil).Assemble(context.Background(), c.req).Input.AgentTurn
			if got != c.want {
				t.Fatalf("agent turn = %+v, want %+v", got, c.want)
			}
		})
	}
}
