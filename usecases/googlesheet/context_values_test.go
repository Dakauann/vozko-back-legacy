package googlesheet

import (
	"context"
	"testing"

	"vozko/domain/googlesheet"
	"vozko/domain/lead"
	"vozko/domain/shared"
)

type noLeads struct{ lead.Repository }

func (noLeads) FindByID(string, string) (*lead.Lead, error) { return nil, lead.ErrLeadNotFound }

type noEntryLeads struct{}

func (noEntryLeads) LeadOfEntry(context.Context, string, shared.EntryRef) (string, error) {
	return "", nil
}

func contextValuesForTest() *ContextValues {
	name := func(context.Context, string, string) (string, error) { return "Fluxo de cobrança", nil }
	return NewContextValues(ContextValuesDeps{
		Leads: noLeads{}, EntryLeads: noEntryLeads{}, AgentNames: name, WorkflowNames: name,
		FrontendBaseURL: "https://app.example.com/",
	})
}

func TestAConversationRowLinksToItsConversation(t *testing.T) {
	values, err := contextValuesForTest().Resolve(context.Background(), "ws-1", googlesheet.Origin{
		Kind: googlesheet.OriginWorkflow, WorkflowID: "wf-1", EntryID: "entry-1", EntryType: string(shared.EntryTypeWhatsApp),
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if values[googlesheet.SourceConversationLink] == "" {
		t.Fatalf("conversation link = %q", values[googlesheet.SourceConversationLink])
	}
}

func TestARunOnALeadWritesNoConversationLink(t *testing.T) {
	values, err := contextValuesForTest().Resolve(context.Background(), "ws-1", googlesheet.Origin{
		Kind: googlesheet.OriginWorkflow, WorkflowID: "wf-1", EntryID: "lead-1", EntryType: "lead",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if link, present := values[googlesheet.SourceConversationLink]; present {
		t.Fatalf("a lead run must not get a conversation link, got %#v", link)
	}
}
