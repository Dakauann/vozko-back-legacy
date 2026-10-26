package conversation_usecase

import (
	"testing"

	"github.com/stretchr/testify/require"

	"vozko/domain/conversation"
	"vozko/domain/lead"
	"vozko/domain/shared"
)

func TestSearchInboxEntriesPassesTheOriginToTheRepository(t *testing.T) {
	repo := &capturedSearch{}
	svc := historyWith(HistoryProviderService{messageRepo: repo})

	_, _, err := svc.SearchInboxEntries(conversation.SearchInboxInput{
		UserID:      "user-1",
		WorkspaceID: "ws-1",
		Origin:      conversation.OriginReceptive,
	})

	require.NoError(t, err)
	require.Equal(t, conversation.OriginReceptive, repo.input.Origin)
}

func TestStageCountsCountTheSameOriginTheListShows(t *testing.T) {
	repo := &capturedStageCounts{}
	svc := historyWith(HistoryProviderService{messageRepo: repo})

	_, err := svc.CountInboxEntriesByStages(conversation.SearchInboxInput{
		UserID:      "user-1",
		WorkspaceID: "ws-1",
		Origin:      conversation.OriginCampaign,
	}, []string{"s-1"})

	require.NoError(t, err)
	require.Equal(t, conversation.OriginCampaign, repo.input.Origin)
}

func TestInboxEntriesCarryTheOriginTheRepositoryRead(t *testing.T) {
	svc := historyWith(HistoryProviderService{leadRepo: &builderLeadRepo{byID: map[string]*lead.Lead{}}})

	entries := svc.buildInboxEntries([]conversation.EntryWithLastMessage{
		{EntryID: "tg-1", EntryType: shared.EntryTypeTelegram, Origin: conversation.OriginReceptive},
		{EntryID: "tg-2", EntryType: shared.EntryTypeTelegram},
	}, "ws-1")

	require.Len(t, entries, 2)
	require.Equal(t, conversation.OriginReceptive, entries[0].Origin)
	require.Equal(t, conversation.Origin(""), entries[1].Origin)
}

type capturedStageCounts struct {
	conversation.MessageRepository
	input conversation.SearchEntriesInput
}

func (c *capturedStageCounts) CountEntriesByStages(input conversation.SearchEntriesInput, stageIDs []string) (map[string]int64, error) {
	c.input = input
	return map[string]int64{}, nil
}
