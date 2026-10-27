package conversation_repository

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

func TestReadingSinceAPositionWalksForwardWithoutSkippingTiesAgainstPostgres(t *testing.T) {
	repo := messageStateRepo(t)
	entryID := uuid.NewString()
	start := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	first := storedMessage(t, repo, entryID, start, "a")
	tieA := storedMessage(t, repo, entryID, start.Add(time.Minute), "b")
	tieB := storedMessage(t, repo, entryID, start.Add(time.Minute), "c")
	storedMessage(t, repo, entryID, start.Add(2*time.Minute), "d")
	storedMessage(t, repo, uuid.NewString(), start.Add(3*time.Minute), "other conversation")
	if tieB.ID < tieA.ID {
		tieA, tieB = tieB, tieA
	}

	all, err := repo.ListByEntryPaginated(conversation.ListMessagesInput{EntryID: entryID, EntryType: shared.EntryTypeWhatsApp, Limit: 10, Since: &conversation.MessageKeyset{}})
	if err != nil || len(all) != 4 || all[0].ID != first.ID {
		t.Fatalf("from the start = %d messages, %v: oldest first", len(all), err)
	}

	page, err := repo.ListByEntryPaginated(conversation.ListMessagesInput{EntryID: entryID, EntryType: shared.EntryTypeWhatsApp, Limit: 1, Since: &conversation.MessageKeyset{At: first.CreatedAt, ID: first.ID}})
	if err != nil || len(page) != 1 || page[0].ID != tieA.ID {
		t.Fatalf("page = %v, %v", page, err)
	}
	next, err := repo.ListByEntryPaginated(conversation.ListMessagesInput{EntryID: entryID, EntryType: shared.EntryTypeWhatsApp, Limit: 10, Since: &conversation.MessageKeyset{At: page[0].CreatedAt, ID: page[0].ID}})
	if err != nil || textsOf(next) != tieB.Text+"d" {
		t.Fatalf("after the first tie = %q, %v: a message sharing the time of the cursor must not be skipped", textsOf(next), err)
	}
}
