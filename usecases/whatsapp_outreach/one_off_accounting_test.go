package whatsapp_outreach

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/whatsapp/template"
	wc "vozko/domain/whatsapp_campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
)

func launchEntry() *wce.WhatsAppCampaignEntry {
	return &wce.WhatsAppCampaignEntry{ID: "entry-launch", CampaignID: "launch-1", LeadID: "lead-1", Status: wce.SendStatusRead, MessageID: "wamid.campaign"}
}

func TestStartConversation_OnALeadAStandardCampaignReachedKeepsTheCampaignsNumbers(t *testing.T) {
	entries := &fakeEntries{existing: launchEntry()}
	uc := newUC(t, func(d *Deps) { d.Entries = entries })

	result, err := uc.uc.Execute(context.Background(), input())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if result.EntryID != "entry-launch" || len(uc.history.records) != 1 || uc.history.records[0].EntryID != "entry-launch" {
		t.Fatalf("the message must still land in the conversation the lead already has: %+v", result)
	}
	if len(entries.statuses) != 0 || entries.metadata != nil {
		t.Fatalf("a one-off send wrote %v and %v onto the campaign's entry; the campaign never sent it", entries.statuses, entries.metadata)
	}
	if uc.sender.lastIn.EntryID != "entry-launch" {
		t.Fatal("the attempt must still name its conversation, it is the one-off dispatch fact")
	}
}

func TestStartConversation_AFailedOneOffNeverFailsAStandardCampaignsEntry(t *testing.T) {
	entries := &fakeEntries{existing: launchEntry()}
	uc := newUC(t, func(d *Deps) { d.Entries = entries })
	uc.sender.result = &template.BilledSendResult{AttemptID: "att-1", Outcome: template.OutcomeRejected}
	uc.sender.err = errors.New("template paused")

	if _, err := uc.uc.Execute(context.Background(), input()); err == nil {
		t.Fatal("a rejected send must surface")
	}
	if len(entries.statuses) != 0 {
		t.Fatalf("statuses = %v, a campaign's entry must not count a one-off failure", entries.statuses)
	}
}

func TestStartConversation_AFailedHoldNeverFailsAStandardCampaignsEntry(t *testing.T) {
	entries := &fakeEntries{existing: launchEntry()}
	uc := newUC(t, func(d *Deps) { d.Entries = entries })
	uc.starts.holdErr = errors.New("assignment down")

	if _, err := uc.uc.Execute(context.Background(), input()); err == nil {
		t.Fatal("a failed hold must refuse")
	}
	if len(entries.statuses) != 0 {
		t.Fatalf("statuses = %v, a campaign's entry must not count a refused one-off", entries.statuses)
	}
}

func TestConversationTemplate_InAStandardCampaignsConversationKeepsTheCampaignsNumbers(t *testing.T) {
	f := newConversationUC(t, func(d *Deps) {
		d.Campaigns = &fakeCampaigns{campaign: &wc.Campaign{ID: "camp-1", WorkspaceID: "ws-1", BusinessPhoneID: "bp-1", Type: wc.CampaignTypeStandard}}
	})

	if _, err := f.uc.Send(context.Background(), conversationInput()); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(f.history.records) != 1 {
		t.Fatal("the template must still appear in the conversation")
	}
	if len(f.entries.statuses) != 0 || f.entries.metadata != nil {
		t.Fatalf("a one-off send wrote %v and %v onto the campaign's entry", f.entries.statuses, f.entries.metadata)
	}
}

func TestConversationTemplate_AFailedOneOffNeverFailsAStandardCampaignsEntry(t *testing.T) {
	f := newConversationUC(t, func(d *Deps) {
		d.Campaigns = &fakeCampaigns{campaign: &wc.Campaign{ID: "camp-1", WorkspaceID: "ws-1", BusinessPhoneID: "bp-1", Type: wc.CampaignTypeStandard}}
	})
	f.sender.result = &template.BilledSendResult{AttemptID: "att-1", Outcome: template.OutcomeRejected}
	f.sender.err = errors.New("template paused by Meta")

	if _, err := f.uc.Send(context.Background(), conversationInput()); err == nil {
		t.Fatal("a rejected send must surface")
	}
	if len(f.entries.statuses) != 0 {
		t.Fatalf("statuses = %v", f.entries.statuses)
	}
}

func TestConversationTemplate_InTheReceptiveContainerRecordsTheSendOnItsEntry(t *testing.T) {
	f := newConversationUC(t)

	if _, err := f.uc.Send(context.Background(), conversationInput()); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(f.entries.statuses) != 1 || f.entries.statuses[0] != string(wce.SendStatusSent) {
		t.Fatalf("statuses = %v, the receptive conversation keeps its last send", f.entries.statuses)
	}
}
