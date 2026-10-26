package whatsapp_outreach

import (
	"context"
	"errors"
	"testing"

	ia "vozko/domain/inbox_assignment"
	"vozko/domain/shared"
	"vozko/domain/whatsapp/template"
	wce "vozko/domain/whatsapp_campaign_entry"
	wo "vozko/domain/whatsapp_outreach"
)

type fakeStarts struct {
	sender         *fakeSender
	holds          []ia.ConversationStart
	sendsAtHold    []int
	confirmed      []ia.StartHold
	sendsAtConfirm []int
	released       []ia.StartHold
	holdErr        error
}

func (f *fakeStarts) HoldForStarter(in ia.ConversationStart) (ia.StartHold, error) {
	f.holds = append(f.holds, in)
	f.sendsAtHold = append(f.sendsAtHold, f.sender.calls)
	if f.holdErr != nil {
		return ia.StartHold{}, f.holdErr
	}
	return ia.StartHold{Start: in, Ownership: ia.StartOwnership{Owner: in.StarterID, Takes: true, Pauses: true}}, nil
}

func (f *fakeStarts) ConfirmStart(hold ia.StartHold) {
	f.confirmed = append(f.confirmed, hold)
	f.sendsAtConfirm = append(f.sendsAtConfirm, f.sender.calls)
}

func (f *fakeStarts) ReleaseStart(hold ia.StartHold) error {
	f.released = append(f.released, hold)
	return nil
}

func TestStartingAConversationGivesItToTheStarterBeforeTheTemplateLeaves(t *testing.T) {
	uc := newUC(t)

	started, err := uc.uc.Execute(context.Background(), input())
	if err != nil {
		t.Fatal(err)
	}

	want := ia.ConversationStart{WorkspaceID: "ws-1", EntryID: started.EntryID, EntryType: string(shared.EntryTypeWhatsApp), AccountID: "bp-1", StarterID: "user-1"}
	if len(uc.starts.holds) != 1 || uc.starts.holds[0] != want {
		t.Fatalf("holds = %+v, want %+v", uc.starts.holds, want)
	}
	if uc.starts.sendsAtHold[0] != 0 {
		t.Fatal("the starter must own the conversation before the contact can reply to the template")
	}
	if len(uc.starts.confirmed) != 1 || uc.starts.sendsAtConfirm[0] != 1 || len(uc.starts.released) != 0 {
		t.Fatalf("confirmed = %d released = %d, want the start confirmed after the send", len(uc.starts.confirmed), len(uc.starts.released))
	}
	if len(uc.claims.calls) != 0 {
		t.Fatalf("claims = %+v, a start goes through the starter rule only", uc.claims.calls)
	}
}

func TestAStartThatCannotBeGivenToTheStarterSendsNothing(t *testing.T) {
	claims := newFakeClaims()
	uc := newUC(t, func(d *Deps) {
		d.SendClaims = claims
		d.SpamPolicy = &fakeSpamPolicy{days: 7}
	})
	uc.starts.holdErr = ia.StarterNotAssigned(errors.New("db down"))

	_, err := uc.uc.Execute(context.Background(), input())

	if !errors.Is(err, ia.ErrStarterNotAssigned) {
		t.Fatalf("err = %v, want ErrStarterNotAssigned", err)
	}
	if wo.ErrorCode(err) != wo.CodeStarterNotAssigned {
		t.Fatalf("code = %q, want %q", wo.ErrorCode(err), wo.CodeStarterNotAssigned)
	}
	if uc.sender.calls != 0 {
		t.Fatal("nothing may be charged or sent for a conversation the automation would take")
	}
	if len(claims.held) != 0 || len(claims.released) != 1 {
		t.Fatalf("held = %v released = %v, the contact must be free for the next try", claims.held, claims.released)
	}
	if len(uc.entries.statuses) != 1 || uc.entries.statuses[0] != string(wce.SendStatusFailed) {
		t.Fatalf("statuses = %v, want the entry marked failed", uc.entries.statuses)
	}
}

func TestAFailedSendGivesTheConversationBack(t *testing.T) {
	uc := newUC(t)
	uc.sender.result = &template.BilledSendResult{AttemptID: "att-1", Outcome: template.OutcomeRejected}
	uc.sender.err = errors.New("meta refused")

	if _, err := uc.uc.Execute(context.Background(), input()); err == nil {
		t.Fatal("expected the send error")
	}
	if len(uc.starts.released) != 1 || len(uc.starts.confirmed) != 0 {
		t.Fatalf("released = %d confirmed = %d, a send that never went out must not keep the conversation", len(uc.starts.released), len(uc.starts.confirmed))
	}
}

func TestAnUnknownOutcomeKeepsTheConversationWithTheStarter(t *testing.T) {
	uc := newUC(t)
	uc.sender.result = &template.BilledSendResult{AttemptID: "att-1", Outcome: template.OutcomeUnknown}
	uc.sender.err = errors.New("upstream down")

	if _, err := uc.uc.Execute(context.Background(), input()); !errors.Is(err, wo.ErrSendOutcomeUnknown) {
		t.Fatalf("err = %v, want ErrSendOutcomeUnknown", err)
	}
	if len(uc.starts.confirmed) != 1 || len(uc.starts.released) != 0 {
		t.Fatalf("confirmed = %d released = %d, the template may have reached the contact", len(uc.starts.confirmed), len(uc.starts.released))
	}
}

func TestAReplayKeepsTheConversationWithTheStarter(t *testing.T) {
	uc := newUC(t)
	uc.sender.result.Replayed = true

	if _, err := uc.uc.Execute(context.Background(), input()); err != nil {
		t.Fatal(err)
	}
	if len(uc.starts.confirmed) != 1 || len(uc.starts.released) != 0 {
		t.Fatalf("confirmed = %d released = %d, a retry of a delivered start must still leave it with the starter", len(uc.starts.confirmed), len(uc.starts.released))
	}
}

func TestAnOpenWindowHoldsNothing(t *testing.T) {
	uc := newUC(t, func(d *Deps) { d.Windows = &fakeWindows{open: true} })

	if _, err := uc.uc.Execute(context.Background(), input()); !errors.Is(err, wo.ErrWindowAlreadyOpen) {
		t.Fatalf("err = %v, want ErrWindowAlreadyOpen", err)
	}
	if len(uc.starts.holds) != 0 {
		t.Fatalf("holds = %+v, no template goes out while the window is open", uc.starts.holds)
	}
}

func TestTheStartRuleDecidesForASendWithoutAUser(t *testing.T) {
	uc := newUC(t)
	in := input()
	in.UserID = ""

	if _, err := uc.uc.Execute(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if len(uc.starts.holds) != 1 || uc.starts.holds[0].StarterID != "" {
		t.Fatalf("holds = %+v, the rule itself decides that nobody takes it", uc.starts.holds)
	}
}

func TestATemplateIntoAConversationClaimsItForTheSender(t *testing.T) {
	f := newConversationUC(t)

	if _, err := f.uc.Send(context.Background(), conversationInput()); err != nil {
		t.Fatal(err)
	}

	want := claimCall{"entry-1", string(shared.EntryTypeWhatsApp), "bp-1", "ws-1", "user-1", ia.TriggerOutreachSent}
	if len(f.claims.calls) != 1 || f.claims.calls[0] != want {
		t.Fatalf("claims = %+v, want %+v", f.claims.calls, want)
	}
}

func TestATemplateIntoAConversationWithoutAUserDoesNotClaim(t *testing.T) {
	f := newConversationUC(t)
	in := conversationInput()
	in.UserID = ""

	if _, err := f.uc.Send(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if len(f.claims.calls) != 0 {
		t.Fatalf("claims = %+v, there is no sender to give the conversation to", f.claims.calls)
	}
}

func TestStartingAConversationRefusesToBootWithoutTheStarterRule(t *testing.T) {
	_, err := NewStartConversationUseCase(Deps{
		Phones: &fakePhones{}, Templates: &fakeTemplates{}, TemplateGrant: &fakeGrant{},
		Leads: &fakeLeads{}, Entries: &fakeEntries{}, EnsureReceptive: &fakeOrganic{},
		CampaignSends: &fakeCampaignSends{}, SpamPolicy: &fakeSpamPolicy{}, SendClaims: newFakeClaims(),
		History: &fakeHistory{}, Assignments: &fakeClaimer{}, Sender: &fakeSender{},
	})
	if err == nil {
		t.Fatal("a start without the starter rule would leave the conversation to the automation")
	}
}
