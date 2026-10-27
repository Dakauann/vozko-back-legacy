package conversation_usecase

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
)

type arrivalJournal struct{ steps []string }

type arrivalHub struct {
	conversation.EventBroadcaster
	journal *arrivalJournal
}

func (h arrivalHub) BroadcastNewMessage(entryID, _ string, _ *conversation.Message) {
	h.journal.steps = append(h.journal.steps, "broadcast "+entryID)
}

type arrivalStages struct{ journal *arrivalJournal }

func (s arrivalStages) AutoAssignInitialStage(workspaceID, campaignID, _, entryID, _ string) {
	s.journal.steps = append(s.journal.steps, "stage "+workspaceID+"/"+campaignID+"/"+entryID)
}

type arrivalStatus struct {
	conversation.ConversationStatusUpdater
	journal *arrivalJournal
}

func (s arrivalStatus) TransitionOnMessage(entryID, _ string, _ conversation.MessageType, _ conversation.MessageHistoryDirection) error {
	s.journal.steps = append(s.journal.steps, "status "+entryID)
	return nil
}

type arrivalWorkspaces struct {
	conversation.CampaignWorkspaceResolver
	workspaceID string
	campaignID  string
}

func (w arrivalWorkspaces) GetEntryWorkspaceID(string, string) (string, error) {
	return w.workspaceID, nil
}
func (w arrivalWorkspaces) GetEntryCampaignID(string, string) (string, error) {
	return w.campaignID, nil
}

type arrivalReplies struct{ journal *arrivalJournal }

func (r arrivalReplies) ContactReplied(entryID, entryType string, _ time.Time) error {
	r.journal.steps = append(r.journal.steps, "reply "+entryType+"/"+entryID)
	return nil
}

func arrivalsFor(journal *arrivalJournal, workspaces arrivalWorkspaces) conversation.EventBroadcaster {
	b, err := NewMessageArrivals(MessageArrivalsDeps{
		Broadcaster: arrivalHub{journal: journal},
		Workspaces:  workspaces,
		Stages:      arrivalStages{journal: journal},
		Status:      arrivalStatus{journal: journal},
		Replies:     arrivalReplies{journal: journal},
	})
	if err != nil {
		panic(err)
	}
	return b
}

func TestANewMessageMovesTheConversationBeforeItIsBroadcast(t *testing.T) {
	journal := &arrivalJournal{}
	arrivalsFor(journal, arrivalWorkspaces{workspaceID: "ws-1", campaignID: "camp-1"}).
		BroadcastNewMessage("entry-1", "whatsapp", &conversation.Message{})

	want := []string{"stage ws-1/camp-1/entry-1", "status entry-1", "broadcast entry-1"}
	if !reflect.DeepEqual(journal.steps, want) {
		t.Fatalf("steps = %v, want %v", journal.steps, want)
	}
}

func TestAConversationOutsideACampaignGetsNoInitialStage(t *testing.T) {
	journal := &arrivalJournal{}
	arrivalsFor(journal, arrivalWorkspaces{workspaceID: "ws-1"}).
		BroadcastNewMessage("entry-1", "whatsapp", nil)

	want := []string{"broadcast entry-1"}
	if !reflect.DeepEqual(journal.steps, want) {
		t.Fatalf("steps = %v, want %v", journal.steps, want)
	}
}

func TestMessageArrivalsRefuseMissingDependencies(t *testing.T) {
	b, err := NewMessageArrivals(MessageArrivalsDeps{})
	var missing *deps.MissingError
	if b != nil || !errors.As(err, &missing) || len(missing.Missing) != 5 {
		t.Fatalf("b = %v err = %v, want every dependency refused", b, err)
	}
}

func TestAContactMessageOnAnyChannelCancelsItsFollowUpsBeforeAnythingElse(t *testing.T) {
	for _, entryType := range shared.ConversationViewableEntryTypes() {
		t.Run(string(entryType), func(t *testing.T) {
			journal := &arrivalJournal{}
			inbound := &conversation.Message{SentBy: conversation.SentByContact("contact-1"), CreatedAt: time.Now()}
			arrivalsFor(journal, arrivalWorkspaces{workspaceID: "ws-1"}).
				BroadcastNewMessage("entry-1", string(entryType), inbound)

			want := []string{"reply " + string(entryType) + "/entry-1", "status entry-1", "broadcast entry-1"}
			if !reflect.DeepEqual(journal.steps, want) {
				t.Fatalf("steps = %v, want %v", journal.steps, want)
			}
		})
	}
}

func TestABusinessMessageCancelsNoFollowUp(t *testing.T) {
	agent, _ := conversation.SentByAutomation("ai:agent-1")
	for _, message := range []*conversation.Message{
		{SentBy: conversation.SentByPerson("user-1")},
		{SentBy: agent},
		nil,
	} {
		journal := &arrivalJournal{}
		arrivalsFor(journal, arrivalWorkspaces{workspaceID: "ws-1"}).BroadcastNewMessage("entry-1", "instagram", message)
		for _, step := range journal.steps {
			if step == "reply instagram/entry-1" {
				t.Fatalf("an outbound message cancelled follow-ups: %v", journal.steps)
			}
		}
	}
}
