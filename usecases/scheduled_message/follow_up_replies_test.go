package scheduled_message_usecase

import (
	"context"
	"testing"
	"time"

	sm "vozko/domain/scheduled_message"
	"vozko/domain/shared"
)

func TestAContactReplyCancelsTheFlaggedFollowUps(t *testing.T) {
	repo := newFakeRepo()
	replies, err := NewReplyCancellation(repo)
	if err != nil {
		t.Fatalf("NewReplyCancellation: %v", err)
	}
	repo.put(&sm.ScheduledMessage{
		ID: "f-1", EntryID: "entry-1", EntryType: shared.EntryTypeInstagram, CreatedByAutomation: "ai:agent-1",
		CancelOnReply: true, Status: sm.StatusPending, CreatedAt: fixedNow.Add(-time.Hour),
	})

	if err := replies.ContactReplied("entry-1", string(shared.EntryTypeInstagram), fixedNow); err != nil {
		t.Fatalf("ContactReplied: %v", err)
	}
	got := repo.get("f-1")
	if got.Status != sm.StatusCanceled || got.CancelReason == nil || *got.CancelReason != sm.CancelContactReplied {
		t.Fatalf("stored = %+v", got)
	}
}

func TestNewReplyCancellationNeedsTheRepository(t *testing.T) {
	if _, err := NewReplyCancellation(nil); err == nil {
		t.Fatal("a nil repository was accepted")
	}
}

func TestAnAutomationMessageIsStoredUnderTheCapAndAPersonsIsNot(t *testing.T) {
	f := newScheduleFixture(t)

	person := scheduleInput()
	if _, err := f.uc.Execute(context.Background(), person); err != nil {
		t.Fatalf("person: %v", err)
	}
	if f.repo.capped != 0 {
		t.Fatal("a person's message went through the automation cap")
	}

	automation := scheduleInput()
	automation.CreatedByUserID = ""
	automation.CreatedByAutomation = "workflow:wf-1"
	automation.CancelOnReply = true
	result, err := f.uc.Execute(context.Background(), automation)
	if err != nil {
		t.Fatalf("automation: %v", err)
	}
	if f.repo.capped != 1 || !result.Message.CancelOnReply || result.Message.CreatedByAutomation != "workflow:wf-1" {
		t.Fatalf("capped %d, message %+v", f.repo.capped, result.Message)
	}
}

func TestTheListNamesTheAgentOrWorkflowThatScheduled(t *testing.T) {
	repo := newFakeRepo()
	list, err := NewListUseCase(repo, &fakeWindows{open: true}, &fixedClock{now: fixedNow}, fakeNames{"ai:agent-1": "Cobrança", "workflow:wf-1": "Pós-venda"})
	if err != nil {
		t.Fatalf("NewListUseCase: %v", err)
	}
	messages := []*sm.ScheduledMessage{
		{ID: "a", CreatedByAutomation: "ai:agent-1"},
		{ID: "b", CreatedByAutomation: "workflow:wf-1"},
		{ID: "c", CreatedByUserID: "user-1"},
	}
	names := list.AuthorNames(messages)
	if names["ai:agent-1"] != "Cobrança" || names["workflow:wf-1"] != "Pós-venda" {
		t.Fatalf("names = %v", names)
	}
}
