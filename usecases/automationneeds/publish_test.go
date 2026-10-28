package automationneeds

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agent"
	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	ia "vozko/domain/inbox_assignment"
	"vozko/domain/workflow"
)

type fakeSlots []domain.Slot

func (f fakeSlots) InWorkspace(context.Context, string) ([]domain.Slot, error) { return f, nil }

type fakeHeld struct{ held []HeldConversation }

func (f *fakeHeld) HeldOn(_ context.Context, _ string, _ conversation.Automation, slotID string, _ int) ([]HeldConversation, error) {
	if slotID != "ig" {
		return nil, nil
	}
	return f.held, nil
}

type recordedHandOffs struct{ handed []ia.RouletteHandOff }

func (r *recordedHandOffs) HandOffToRoulette(in ia.RouletteHandOff) (string, error) {
	r.handed = append(r.handed, in)
	return "", nil
}

type forgotten struct{ ids []string }

func (f *forgotten) Forget(id string) { f.ids = append(f.ids, id) }

type publishWorld struct {
	guard     *PublishGuard
	publisher *recordedPublisher
	handOffs  *recordedHandOffs
	caches    *forgotten
}

func newPublishWorld(t *testing.T, slots fakeSlots, held []HeldConversation) publishWorld {
	t.Helper()
	w := publishWorld{publisher: &recordedPublisher{}, handOffs: &recordedHandOffs{}, caches: &forgotten{}}
	alerts, _ := NewAlerts(w.publisher)
	service, err := NewService(ServiceDeps{
		Source: PublishedSource{
			Agents: fakeAgents{"livre": {ID: "livre", WorkspaceID: "ws", Name: "Livre", MessagingPrompt: "Oi"}},
			Workflows: fakeWorkflows{
				"com-ia": {ID: "com-ia", WorkspaceID: "ws", Name: "Com IA", Graph: workflow.Graph{Nodes: []workflow.Node{{ID: "n", Type: workflow.NodeTypeActionAIAgent, Config: map[string]interface{}{"agent_id": "livre"}}}}},
				"outro":  {ID: "outro", WorkspaceID: "ws", Name: "Outro"},
			},
		},
		Values:       fakeValues{},
		Declarations: declared,
		Sheets:       &fakeSheetFiles{},
	})
	if err != nil {
		t.Fatal(err)
	}
	w.guard, err = NewPublishGuard(PublishGuardDeps{Service: service, Slots: slots, Alerts: alerts, Held: &fakeHeld{held: held}, HandOff: w.handOffs, Containers: w.caches})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func slotsRunning() fakeSlots {
	return fakeSlots{
		{Kind: domain.SlotInstagram, ID: "ig", Name: "@loja", Running: []conversation.Automation{{Kind: conversation.AutomationAgent, ID: "livre"}}},
		{Kind: domain.SlotWhatsAppNumber, ID: "num", Name: "+55", Running: []conversation.Automation{{Kind: conversation.AutomationWorkflow, ID: "com-ia"}}},
		{Kind: domain.SlotWebchat, ID: "site", Name: "Site", Running: []conversation.Automation{{Kind: conversation.AutomationWorkflow, ID: "outro"}}},
	}
}

func TestPublishingAnAgentThatNowNeedsAVariableAsksToConfirmTheSwitchOff(t *testing.T) {
	w := newPublishWorld(t, slotsRunning(), nil)
	draft := &agent.Agent{ID: "livre", WorkspaceID: "ws", Name: "Livre", MessagingPrompt: "Turma {{turma}}"}
	_, err := w.guard.PlanAgent(context.Background(), draft)
	refusal, ok := domain.RefusalOf(err)
	if !ok || !errors.Is(err, domain.ErrPublishSwitchesOff) {
		t.Fatalf("err = %v", err)
	}
	if len(refusal.Impact) != 2 || refusal.Impact[0].Slot.ID != "ig" || refusal.Impact[1].Slot.ID != "num" || refusal.Impact[1].Automation.Name != "Com IA" {
		t.Fatalf("impact = %+v, want the agent's own channel and the workflow that runs it", refusal.Impact)
	}
	plan, err := w.guard.PlanAgent(WithSwitchOffConfirmed(context.Background()), draft)
	if err != nil || len(plan.Offs) != 2 {
		t.Fatalf("confirmed plan = %+v, %v", plan, err)
	}
}

func TestPublishingWithoutNewNeedsSwitchesNothingOff(t *testing.T) {
	w := newPublishWorld(t, slotsRunning(), nil)
	plan, err := w.guard.PlanAgent(context.Background(), &agent.Agent{ID: "livre", WorkspaceID: "ws", Name: "Livre", MessagingPrompt: "Oi {{lead_name}}"})
	if err != nil || len(plan.Offs) != 0 {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
}

func TestAVoiceWorkflowCannotNeedVariables(t *testing.T) {
	w := newPublishWorld(t, nil, nil)
	draft := &workflow.Workflow{ID: "voz", WorkspaceID: "ws", Name: "URA", Type: workflow.WorkflowTypeVoice, Graph: workflow.Graph{Nodes: []workflow.Node{{ID: "n", Type: workflow.NodeTypeActionPlayAudio, Config: map[string]interface{}{"text": "{{campvars.nome}}"}}}}}
	if _, err := w.guard.PlanWorkflow(context.Background(), draft); !errors.Is(err, domain.ErrVoiceVariables) {
		t.Fatalf("err = %v", err)
	}
}

func TestAfterTheSwitchOffTheOwnersAreToldAndHeldConversationsGoToTheTeam(t *testing.T) {
	w := newPublishWorld(t, slotsRunning(), []HeldConversation{{EntryID: "e1", EntryType: "instagram"}, {EntryID: "e2", EntryType: "instagram"}})
	draft := &agent.Agent{ID: "livre", WorkspaceID: "ws", Name: "Livre", MessagingPrompt: "Turma {{turma}}"}
	plan, err := w.guard.PlanAgent(WithSwitchOffConfirmed(context.Background()), draft)
	if err != nil {
		t.Fatal(err)
	}
	w.guard.Done(context.Background(), plan, []string{"container-1"})
	if len(w.caches.ids) != 1 || w.caches.ids[0] != "container-1" {
		t.Fatalf("forgotten = %v", w.caches.ids)
	}
	if len(w.publisher.drafts) != 2 || w.publisher.drafts[0].EpisodeKey != "automation_switch_off:instagram:ig" {
		t.Fatalf("drafts = %+v", w.publisher.drafts)
	}
	if len(w.handOffs.handed) != 2 || w.handOffs.handed[0].ByActorID != "ai:livre" {
		t.Fatalf("hand-offs = %+v", w.handOffs.handed)
	}
	if len(w.publisher.resolved) != 1 || w.publisher.resolved[0] != "automation_variables_unmet:agent:livre" {
		t.Fatalf("resolved = %v", w.publisher.resolved)
	}
}
