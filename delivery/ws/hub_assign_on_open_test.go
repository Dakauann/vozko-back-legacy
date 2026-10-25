package ws

import (
	"testing"
)

type recordingOpenClaims struct {
	quietHubAssigner
	calls []string
}

func (r *recordingOpenClaims) AssignOnOpen(entryID, entryType, userID string, isSystemAdmin bool) (bool, error) {
	admin := "member"
	if isSystemAdmin {
		admin = "system admin"
	}
	r.calls = append(r.calls, entryID+"|"+entryType+"|"+userID+"|"+admin)
	return true, nil
}

func makeConn(userID, workspaceID string, isAdmin bool) *WSConnection {
	return &WSConnection{
		ID:          "conn-" + userID,
		UserID:      userID,
		WorkspaceID: workspaceID,
		IsAdmin:     isAdmin,
		Send:        make(chan []byte, 16),
	}
}

func TestOpeningAConversationLetsTheAssignerDecideForTheViewer(t *testing.T) {
	claims := &recordingOpenClaims{}
	hub := testHub(ConversationHubDeps{Authorizer: multiOpenAuthorizer{}})
	hub.cmds().Assigner = claims

	hub.tryAssignOnOpen(makeConn("ana", "ws-1", true), "entry-1", "whatsapp")

	if len(claims.calls) != 1 || claims.calls[0] != "entry-1|whatsapp|ana|system admin" {
		t.Fatalf("claims = %v, want one open claim for the viewer", claims.calls)
	}
}
