package workflow_usecase

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	uw "vozko/domain/unofficial_whatsapp"
	"vozko/domain/workflow"
)

type pagedPrompts struct {
	prompts []workflow.SentPrompt
	asked   []string
	types   []workflow.NodeType
	err     error
}

func (p *pagedPrompts) SentPrompts(_ context.Context, entryType shared.EntryType, nodeTypes []workflow.NodeType, after string, limit int) ([]workflow.SentPrompt, error) {
	p.asked = append(p.asked, after)
	p.types = nodeTypes
	if p.err != nil {
		return nil, p.err
	}
	var page []workflow.SentPrompt
	for _, prompt := range p.prompts {
		if prompt.EntryType == entryType && prompt.StepID > after && len(page) < limit {
			page = append(page, prompt)
		}
	}
	return page, nil
}

type repairedTranscript struct {
	stored   map[string]bool
	attached map[string]conversation.InteractiveContent
	refs     []conversation.SentMessageRef
	err      error
}

func (r *repairedTranscript) AttachMissingInteractive(_ context.Context, ref conversation.SentMessageRef, content conversation.InteractiveContent) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	r.refs = append(r.refs, ref)
	if r.stored[ref.ProviderMessageID] {
		return false, nil
	}
	if r.attached == nil {
		r.attached = map[string]conversation.InteractiveContent{}
	}
	r.attached[ref.ProviderMessageID] = content
	return true, nil
}

func sentMenu(step, provider string) workflow.SentPrompt {
	return workflow.SentPrompt{
		StepID: step, WorkflowID: "wf-1", EntryID: "conv-1", EntryType: shared.EntryTypeUnofficialWhatsApp, ProviderMessageID: provider,
		Config: map[string]interface{}{
			"interactive_type": "list", "body": "MENU INICIAL", "header_text": "JURIDICO SINDCOP", "footer": "Atendimento",
			"list_button": "Ver opções",
			"sections":    `[{"title":"Assuntos","rows":[{"id":"filiado","title":"Já sou filiado","description":""},{"id":"processo","title":"Processo","description":"Ação judicial"}]}]`,
		},
	}
}

func newRepair(t *testing.T, prompts workflow.SentPromptSource, messages conversation.InteractiveBackfill) *PromptTranscriptRepair {
	t.Helper()
	repair, err := NewPromptTranscriptRepair(PromptTranscriptRepairDeps{
		Prompts: prompts, Messages: messages,
		Shapers: map[shared.EntryType]conversation.InteractiveShaper{shared.EntryTypeUnofficialWhatsApp: uw.ShownInteractive},
	})
	if err != nil {
		t.Fatal(err)
	}
	return repair
}

func TestPromptRepairStoresTheMenuTheLineShowedOnTheWorkflowsMessage(t *testing.T) {
	prompts := &pagedPrompts{prompts: []workflow.SentPrompt{sentMenu("step-1", "3EB0MENU")}}
	transcript := &repairedTranscript{}

	tally, err := newRepair(t, prompts, transcript).Run(context.Background(), shared.EntryTypeUnofficialWhatsApp, 10)
	if err != nil {
		t.Fatal(err)
	}

	want := uw.ShownInteractive(conversation.SendInteractiveRequest{
		Body: "MENU INICIAL", Header: "JURIDICO SINDCOP", Footer: "Atendimento", Style: "list", ListButton: "Ver opções",
		Options: []conversation.InteractiveOption{
			{ID: "filiado", Title: "Já sou filiado", Section: "Assuntos"},
			{ID: "processo", Title: "Processo", Description: "Ação judicial", Section: "Assuntos"},
		},
	})
	if got := transcript.attached["3EB0MENU"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("attached %+v, want %+v", got, want)
	}
	wantRef := conversation.SentMessageRef{EntryType: shared.EntryTypeUnofficialWhatsApp, EntryID: "conv-1", ProviderMessageID: "3EB0MENU", SentBy: conversation.SentByWorkflow("wf-1")}
	if transcript.refs[0] != wantRef {
		t.Fatalf("ref %+v, want the workflow's own message %+v", transcript.refs[0], wantRef)
	}
	if tally != (PromptRepairTally{Scanned: 1, Repaired: 1}) {
		t.Fatalf("tally %+v", tally)
	}
	if !reflect.DeepEqual(prompts.types, workflow.InteractivePromptNodeTypes()) {
		t.Fatalf("asked for node types %v", prompts.types)
	}
}

func TestPromptRepairWalksEveryPageAndLeavesStoredMenusAlone(t *testing.T) {
	var all []workflow.SentPrompt
	for i := 1; i <= 5; i++ {
		all = append(all, sentMenu(fmt.Sprintf("step-%d", i), fmt.Sprintf("P%d", i)))
	}
	broken := sentMenu("step-6", "P6")
	broken.Config = map[string]interface{}{"interactive_type": "list", "body": "MENU", "sections": `[{"rows":`}
	empty := sentMenu("step-7", "P7")
	empty.Config["body"] = "  "
	all = append(all, broken, empty)
	prompts := &pagedPrompts{prompts: all}
	transcript := &repairedTranscript{stored: map[string]bool{"P2": true}}

	tally, err := newRepair(t, prompts, transcript).Run(context.Background(), shared.EntryTypeUnofficialWhatsApp, 2)
	if err != nil {
		t.Fatal(err)
	}

	if tally != (PromptRepairTally{Scanned: 7, Repaired: 4, Untouched: 1, Unreadable: 2}) {
		t.Fatalf("tally %+v", tally)
	}
	wantCursors := []string{workflow.SentPromptCursorStart, "step-2", "step-4", "step-6"}
	if !reflect.DeepEqual(prompts.asked, wantCursors) {
		t.Fatalf("cursors %v, want %v", prompts.asked, wantCursors)
	}
}

func TestPromptRepairStopsAtTheFirstFailure(t *testing.T) {
	failure := errors.New("database gone")
	prompts := &pagedPrompts{prompts: []workflow.SentPrompt{sentMenu("step-1", "P1")}}

	_, err := newRepair(t, prompts, &repairedTranscript{err: failure}).Run(context.Background(), shared.EntryTypeUnofficialWhatsApp, 10)
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want the store failure", err)
	}
	_, err = newRepair(t, &pagedPrompts{err: failure}, &repairedTranscript{}).Run(context.Background(), shared.EntryTypeUnofficialWhatsApp, 10)
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want the source failure", err)
	}
}

func TestPromptRepairRefusesAChannelItCannotShapeAndABadPage(t *testing.T) {
	repair := newRepair(t, &pagedPrompts{}, &repairedTranscript{})

	if _, err := repair.Run(context.Background(), shared.EntryTypeInstagram, 10); !errors.Is(err, ErrPromptRepairChannelUnsupported) {
		t.Fatalf("err = %v, want unsupported channel", err)
	}
	if _, err := repair.Run(context.Background(), shared.EntryTypeUnofficialWhatsApp, 0); !errors.Is(err, ErrPromptRepairPageInvalid) {
		t.Fatalf("err = %v, want invalid page", err)
	}
}

func TestPromptRepairNeedsEveryDependency(t *testing.T) {
	_, err := NewPromptTranscriptRepair(PromptTranscriptRepairDeps{})
	if !errors.Is(err, deps.ErrMissing) {
		t.Fatalf("err = %v, want missing dependencies", err)
	}
}
