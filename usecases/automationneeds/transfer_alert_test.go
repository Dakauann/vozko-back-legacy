package automationneeds

import (
	"context"
	"strings"
	"testing"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
)

func TestAnAgentThatCouldNotTransferWarnsInTheTargetsOwnEpisode(t *testing.T) {
	publisher := &recordedPublisher{}
	alerts, err := NewAlerts(publisher)
	if err != nil {
		t.Fatal(err)
	}
	target := conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "wf-matricula"}
	refusal := &domain.Refusal{Cause: domain.ErrConversationMissing, Automation: domain.Link{Automation: target, Name: "Matrícula"}, Needs: domain.Needs{{Name: "turma"}}}

	if err := alerts.TransferUnmet(context.Background(), "ws-1", refusal, "Agente Prospecção"); err != nil {
		t.Fatal(err)
	}

	if len(publisher.drafts) != 1 {
		t.Fatalf("drafts = %d", len(publisher.drafts))
	}
	draft := publisher.drafts[0]
	if draft.EpisodeKey != UnmetEpisodeKey(target) || !draft.Email {
		t.Fatalf("draft = %+v", draft)
	}
	for _, want := range []string{"Agente Prospecção", "Matrícula", "turma"} {
		if !strings.Contains(draft.Title+" "+draft.Body, want) {
			t.Fatalf("missing %q in %q / %q", want, draft.Title, draft.Body)
		}
	}
	if strings.Contains(draft.Body, "não respondeu") {
		t.Fatalf("a refused transfer is not a silent automation: %q", draft.Body)
	}
}
