package ws

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"vozko/domain/conversation"
)

func funnelSummaryFrame(t *testing.T, stageIDs ...string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(RequestFunnelSummaryPayload{StageIDs: stageIDs})
	require.NoError(t, err)
	return raw
}

func TestFunnelSummaryCountsEveryStageInOneCallInTheRequestedOrder(t *testing.T) {
	hub, conn, _, inbox := newInboxHub(t)
	conn.DepartmentID = "dept-1"
	conn.IsAdmin = false
	inbox.stageCounts = map[string]int64{"stage-a": 3, "stage-b": 1}

	hub.handleRequestFunnelSummary(conn, funnelSummaryFrame(t, "stage-b", "stage-a", "__unstaged__"))

	payload := awaitPayloadOf(t, conn, WSEventFunnelSummary)
	require.JSONEq(t, `[{"stage_id":"stage-b","total_items":1},{"stage_id":"stage-a","total_items":3},{"stage_id":"__unstaged__","total_items":0}]`, string(payload["columns"]))

	inbox.mu.Lock()
	defer inbox.mu.Unlock()
	require.Len(t, inbox.stageCountCalls, 1)
	call := inbox.stageCountCalls[0]
	require.Equal(t, conn.UserID, call.userID)
	require.Equal(t, []string{"stage-b", "stage-a", "__unstaged__"}, call.stageIDs)
	require.Equal(t, conversation.SearchInboxInput{
		UserID:               conn.UserID,
		SelectedDepartmentID: "dept-1",
		CampaignID:           conn.CampaignID,
		CampaignType:         conn.CampaignType,
		WhatsAppCampaignType: conn.WhatsAppCampaignType,
		ContainerKind:        conversation.ContainerKind(conn.ContainerKind),
		AssignedUserID:       conn.UserID,
		WorkspaceID:          conn.WorkspaceID,
	}, call.input)
	require.Empty(t, inbox.inputs, "the summary no longer pages each stage")
}

func TestFunnelSummaryOutsideTheGlobalViewKeepsTheContainerWorkspace(t *testing.T) {
	hub, conn, _, inbox := newInboxHub(t)
	conn.ViewMode = "campaign"
	conn.CampaignID, conn.CampaignType = "campaign-1", "whatsapp"

	hub.handleRequestFunnelSummary(conn, funnelSummaryFrame(t, "stage-a"))

	awaitPayloadOf(t, conn, WSEventFunnelSummary)
	inbox.mu.Lock()
	defer inbox.mu.Unlock()
	require.Len(t, inbox.stageCountCalls, 1)
	require.Empty(t, inbox.stageCountCalls[0].input.WorkspaceID)
	require.Equal(t, "campaign-1", inbox.stageCountCalls[0].input.CampaignID)
}

func TestFunnelSummaryFailureIsAnErrorNotZeros(t *testing.T) {
	hub, conn, _, inbox := newInboxHub(t)
	inbox.stageCountErr = errors.New("db down")

	hub.handleRequestFunnelSummary(conn, funnelSummaryFrame(t, "stage-a", "stage-b"))

	payload := awaitPayloadOf(t, conn, WSEventError)
	require.JSONEq(t, `"fetch_error"`, string(payload["code"]))
	select {
	case raw := <-conn.Send:
		var msg struct {
			Type WSEventType `json:"type"`
		}
		require.NoError(t, json.Unmarshal(raw, &msg))
		require.NotEqual(t, WSEventFunnelSummary, msg.Type, "a failed count must never be shown as zeros")
	default:
	}
}

func TestFunnelSummaryStillRefusesAnEmptyRequest(t *testing.T) {
	hub, conn, _, inbox := newInboxHub(t)

	hub.handleRequestFunnelSummary(conn, funnelSummaryFrame(t))

	payload := awaitPayloadOf(t, conn, WSEventError)
	require.JSONEq(t, `"missing_fields"`, string(payload["code"]))
	require.Empty(t, inbox.stageCountCalls)
}
