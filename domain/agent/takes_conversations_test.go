package agent

import "testing"

func TestAnAgentTakesConversationsOnlyWhenActiveInTheSameWorkspace(t *testing.T) {
	cases := map[string]struct {
		agent     *Agent
		workspace string
		want      bool
	}{
		"active here":     {&Agent{WorkspaceID: "ws-1", IsActive: true}, "ws-1", true},
		"inactive":        {&Agent{WorkspaceID: "ws-1"}, "ws-1", false},
		"other workspace": {&Agent{WorkspaceID: "ws-2", IsActive: true}, "ws-1", false},
		"no workspace":    {&Agent{WorkspaceID: "ws-1", IsActive: true}, " ", false},
		"missing":         {nil, "ws-1", false},
	}
	for name, c := range cases {
		if got := c.agent.TakesConversationsIn(c.workspace); got != c.want {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}
