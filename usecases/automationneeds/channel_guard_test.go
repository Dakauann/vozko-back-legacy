package automationneeds

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agent"
	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

func boletoFlow(id string) *workflow.Workflow {
	return &workflow.Workflow{ID: id, WorkspaceID: "ws", Name: "Boleto", Graph: workflow.Graph{
		Nodes: []workflow.Node{
			{ID: "t", Type: workflow.NodeTypeTriggerMessageReceived, Config: map[string]interface{}{}},
			{ID: "tpl", Type: workflow.NodeTypeActionSendTemplate, Config: map[string]interface{}{"template_id": "boleto", "display_name": "Enviar boleto"}},
		},
		Edges: []workflow.Edge{{Source: "t", Target: "tpl"}},
	}}
}

func channelService(t *testing.T) *Service {
	t.Helper()
	s := newService(t, fakeValues{values: map[string]string{}})
	src := s.source.(PublishedSource)
	src.Workflows.(fakeWorkflows)["boleto"] = boletoFlow("boleto")
	return s
}

func runsBoleto() conversation.ChannelAutomation {
	return conversation.ChannelAutomation{WorkflowID: id("boleto"), EnableWorkflow: true}
}

func TestLinkingAWorkflowThatSendsATemplateToInstagramIsRefused(t *testing.T) {
	err := channelService(t).AdmitReceptive("ws", domain.SlotInstagram, runsBoleto())
	refusal, ok := domain.RefusalOf(err)
	if !errors.Is(err, domain.ErrChannelUnsupported) || !ok || len(refusal.Channels) != 1 || refusal.Channels[0].Step != "Enviar boleto" {
		t.Fatalf("err = %v", err)
	}
}

func TestLinkingTheSameWorkflowToTheOfficialNumberIsFine(t *testing.T) {
	if err := channelService(t).AdmitReceptive("ws", domain.SlotWhatsAppNumber, runsBoleto()); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestHandingAnInstagramConversationToAWorkflowThatSendsATemplateIsRefused(t *testing.T) {
	s := channelService(t)
	target := ref(conversation.AutomationWorkflow, "boleto")
	if err := s.ConversationMissing(context.Background(), "ws", "e1", shared.EntryTypeInstagram, target); !errors.Is(err, domain.ErrChannelUnsupported) {
		t.Fatalf("err = %v", err)
	}
	if err := s.ConversationMissingWith(context.Background(), "ws", "e1", shared.EntryTypeInstagram, target, map[string]string{}); !errors.Is(err, domain.ErrChannelUnsupported) {
		t.Fatalf("err = %v", err)
	}
	if err := s.ConversationMissing(context.Background(), "ws", "e1", shared.EntryTypeWhatsApp, target); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func channelPublishWorld(t *testing.T, slots fakeSlots) publishWorld {
	t.Helper()
	w := newPublishWorld(t, slots, nil)
	src := w.guard.d.Service.source.(PublishedSource)
	src.Workflows.(fakeWorkflows)["boleto"] = boletoFlow("boleto")
	src.Agents.(fakeAgents)["cobra"] = &agent.Agent{ID: "cobra", WorkspaceID: "ws", Name: "Cobra", MessagingPrompt: "Oi"}
	return w
}

func cobraOnInstagram() fakeSlots {
	return fakeSlots{
		{Kind: domain.SlotInstagram, ID: "ig", Name: "@loja", Running: []conversation.Automation{{Kind: conversation.AutomationAgent, ID: "cobra"}}},
		{Kind: domain.SlotWhatsAppNumber, ID: "num", Name: "+55", Running: []conversation.Automation{{Kind: conversation.AutomationWorkflow, ID: "boleto"}}},
	}
}

func TestPublishingAnAgentOnInstagramThatNowCallsATemplateWorkflowIsRefused(t *testing.T) {
	w := channelPublishWorld(t, cobraOnInstagram())
	draft := &agent.Agent{ID: "cobra", WorkspaceID: "ws", Name: "Cobra", MessagingPrompt: "Oi", InternalTools: []agent.ToolBinding{{Name: workflow.AgentRunWorkflowTool, Config: map[string]interface{}{"workflow_id": "boleto"}}}}
	_, err := w.guard.PlanAgent(WithSwitchOffConfirmed(context.Background()), draft)
	refusal, ok := domain.RefusalOf(err)
	if !errors.Is(err, domain.ErrChannelUnsupported) || !ok || refusal.Automation.ID != "cobra" {
		t.Fatalf("err = %v", err)
	}
}

func TestTheChannelsOfAWorkflowAreTheOnesItsRunnersReachItOn(t *testing.T) {
	w := channelPublishWorld(t, cobraOnInstagram())
	got, err := w.guard.ChannelsOf(context.Background(), boletoFlow("boleto"))
	if err != nil || len(got) != 1 || got[0] != shared.EntryTypeWhatsApp {
		t.Fatalf("channels = %v, %v", got, err)
	}
}

type failingSlots struct{}

func (failingSlots) InWorkspace(context.Context, string) ([]domain.Slot, error) {
	return nil, errors.New("db down")
}

func TestUnreadableSlotsFailTheChannelRead(t *testing.T) {
	w := channelPublishWorld(t, nil)
	w.guard.d.Slots = failingSlots{}
	if _, err := w.guard.ChannelsOf(context.Background(), boletoFlow("boleto")); !errors.Is(err, domain.ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
}
