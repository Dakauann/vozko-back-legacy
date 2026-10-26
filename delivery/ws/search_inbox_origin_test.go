package ws

import (
	"testing"

	"github.com/stretchr/testify/require"

	"vozko/domain/conversation"
)

func TestSearchInboxFiltersByOriginAndEchoesIt(t *testing.T) {
	hub, conn, _, inbox := newInboxHub(t)
	hub.handleSearchInbox(conn, personalFrame(t, map[string]any{"origin": "receptive"}))

	payload := awaitPayloadOf(t, conn, WSEventSearchResults)
	require.Equal(t, conversation.OriginReceptive, inbox.last().Origin)
	require.JSONEq(t, `{"origin":"receptive"}`, string(payload["filters"]))
}

func TestSearchInboxWithoutOriginListsEveryOrigin(t *testing.T) {
	hub, conn, _, inbox := newInboxHub(t)
	hub.handleSearchInbox(conn, personalFrame(t, nil))
	awaitPayloadOf(t, conn, WSEventSearchResults)
	require.Equal(t, conversation.Origin(""), inbox.last().Origin)
}

func TestSearchInboxRefusesAnUnknownOriginBeforeSearching(t *testing.T) {
	hub, conn, _, inbox := newInboxHub(t)
	hub.handleSearchInbox(conn, personalFrame(t, map[string]any{"origin": "inbound"}))

	payload := decodeErrorPayload(t, awaitSendPayload(t, conn))
	require.Equal(t, "invalid_origin", payload.Code)
	require.Empty(t, inbox.inputs, "a refused search must not run")
}
