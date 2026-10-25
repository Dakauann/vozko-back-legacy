package ws

import (
	"fmt"
	"sync"
	"testing"

	"vozko/domain/conversation"
)

type workspaceRuleAuthorizer struct {
	hubDepartmentTestAuthorizer
	entryWorkspace string
	permitted      map[string]bool

	mu      sync.Mutex
	checked map[string]int
}

func (a *workspaceRuleAuthorizer) ResolveEntryAccess(workspaceID, _, _ string) func(string, bool) bool {
	return func(userID string, isAdmin bool) bool {
		a.mu.Lock()
		a.checked[userID]++
		a.mu.Unlock()
		if isAdmin {
			return a.permitted[userID]
		}
		return workspaceID != "" && workspaceID == a.entryWorkspace && a.permitted[userID]
	}
}

type stubLabelProvider struct{ conversation.LabelProvider }

func (stubLabelProvider) GetEntryLabels(string, string, string) ([]conversation.InboxEntryLabel, error) {
	return []conversation.InboxEntryLabel{}, nil
}

type audienceMember struct {
	user      string
	workspace string
	admin     bool
	permitted bool
}

func audienceCombinations() []audienceMember {
	var members []audienceMember
	for _, workspace := range []string{"ws-1", "ws-2", ""} {
		for _, admin := range []bool{true, false} {
			for _, permitted := range []bool{true, false} {
				members = append(members, audienceMember{
					user:      fmt.Sprintf("user-%s-admin=%v-permitted=%v", workspace, admin, permitted),
					workspace: workspace, admin: admin, permitted: permitted,
				})
			}
		}
	}
	return members
}

func audienceHub(members []audienceMember) (*ConversationHub, *workspaceRuleAuthorizer) {
	auth := &workspaceRuleAuthorizer{entryWorkspace: "ws-1", permitted: map[string]bool{}, checked: map[string]int{}}
	hub := testHub(ConversationHubDeps{Authorizer: auth})
	hub.stageProvider = stubStageProvider{}
	hub.labelProvider = stubLabelProvider{}
	for i, m := range members {
		auth.permitted[m.user] = m.permitted
		conn := &WSConnection{
			ID: fmt.Sprintf("conn-%d", i), UserID: m.user, WorkspaceID: m.workspace, IsAdmin: m.admin,
			Send: make(chan []byte, 4), Done: make(chan struct{}),
		}
		hub.connections[conn.ID] = conn
		hub.userConnections[m.user] = map[string]bool{conn.ID: true}
	}
	return hub, auth
}

func TestStageAndLabelUpdatesReachTheSameConnectionsWithoutCheckingOtherWorkspaces(t *testing.T) {
	broadcasts := map[string]func(h *ConversationHub, workspaceID string){
		"stage": func(h *ConversationHub, workspaceID string) {
			h.broadcastStageUpdateLocal(workspaceID, "entry-1", "whatsapp")
		},
		"label": func(h *ConversationHub, workspaceID string) {
			h.broadcastLabelUpdateLocal(workspaceID, "entry-1", "whatsapp")
		},
	}
	for kind, broadcast := range broadcasts {
		for _, broadcastWorkspace := range []string{"ws-1", ""} {
			t.Run(kind+" in "+fmt.Sprintf("%q", broadcastWorkspace), func(t *testing.T) {
				members := audienceCombinations()
				hub, auth := audienceHub(members)

				broadcast(hub, broadcastWorkspace)

				for i, m := range members {
					conn := hub.connections[fmt.Sprintf("conn-%d", i)]
					wantDelivered := m.permitted && (m.admin || m.workspace == "ws-1")
					if delivered := len(conn.Send) > 0; delivered != wantDelivered {
						t.Fatalf("%s: delivered = %v, want %v", m.user, delivered, wantDelivered)
					}
					otherWorkspaceMember := !m.admin && broadcastWorkspace != "" && m.workspace != "" && m.workspace != broadcastWorkspace
					if otherWorkspaceMember && auth.checked[m.user] != 0 {
						t.Fatalf("%s: a non-admin of another workspace was permission checked", m.user)
					}
					if !otherWorkspaceMember && auth.checked[m.user] != 1 {
						t.Fatalf("%s: checked %d times, want once", m.user, auth.checked[m.user])
					}
				}
			})
		}
	}
}
