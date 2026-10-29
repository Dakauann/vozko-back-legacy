package siteforms

import (
	"context"
	"errors"
	"testing"

	whatsappform_usecase "vozko/usecases/whatsappform"
)

type automationDesk struct {
	byForm    map[string][]whatsappform_usecase.FormAutomation
	scaffolds []string
	err       error
}

func (d *automationDesk) List(_ context.Context, workspaceID, formID string) ([]whatsappform_usecase.FormAutomation, error) {
	if d.err != nil {
		return nil, d.err
	}
	return d.byForm[workspaceID+"|"+formID], nil
}

func (d *automationDesk) Scaffold(_ context.Context, workspaceID, userID, formID, formName string) (string, error) {
	if d.err != nil {
		return "", d.err
	}
	d.scaffolds = append(d.scaffolds, workspaceID+"|"+userID+"|"+formID+"|"+formName)
	d.byForm[workspaceID+"|"+formID] = append(d.byForm[workspaceID+"|"+formID], whatsappform_usecase.FormAutomation{WorkflowID: "wf-new", Name: "Respostas", Use: "trigger"})
	return "wf-new", nil
}

func TestADraftFormGetsItsKeyAndListsOnlyTheWorkflowsItTriggers(t *testing.T) {
	w := newWorld(t)
	view, err := w.svc.Automations(context.Background(), "w1", "p1", "contato")
	if err != nil {
		t.Fatal(err)
	}
	if view.FormKey == "" || w.forms.keys["p1"]["contato"] != view.FormKey || len(view.Automations) != 0 {
		t.Fatalf("got %+v", view)
	}
	w.automations.byForm["w1|"+view.FormKey] = []whatsappform_usecase.FormAutomation{{WorkflowID: "wf-1", Use: "trigger", Active: true}, {WorkflowID: "wf-2", Use: "node"}}
	again, err := w.svc.Automations(context.Background(), "w1", "p1", "contato")
	if err != nil || again.FormKey != view.FormKey || len(again.Automations) != 1 || again.Automations[0].WorkflowID != "wf-1" {
		t.Fatalf("got %+v %v", again, err)
	}
	pages, err := w.svc.PageForms(context.Background(), "w1")
	if err != nil || len(pages) != 1 || pages[0].FormKey != view.FormKey || pages[0].Live {
		t.Fatalf("a draft form is listable before the first publish: %+v %v", pages, err)
	}
	draft, err := w.svc.PageForm(context.Background(), "w1", view.FormKey)
	if err != nil || draft.FormID != "contato" || draft.ProjectID != "p1" || draft.Live {
		t.Fatalf("got %+v %v", draft, err)
	}
	if _, err := w.svc.PageForm(context.Background(), "w2", view.FormKey); err == nil {
		t.Fatal("a form of another workspace is unknown")
	}
}

func TestScaffoldingAPageFormAutomationBindsItToTheFormKey(t *testing.T) {
	w := newWorld(t)
	id, err := w.svc.ScaffoldAutomation(context.Background(), "w1", "user-1", "p1", "contato")
	if err != nil || id != "wf-new" {
		t.Fatalf("got %s %v", id, err)
	}
	key := w.forms.keys["p1"]["contato"]
	if len(w.automations.scaffolds) != 1 || w.automations.scaffolds[0] != "w1|user-1|"+key+"|contato" {
		t.Fatalf("scaffolds %v", w.automations.scaffolds)
	}
	bound, err := w.svc.WorkflowBound(context.Background(), "w1", "p1", "contato", "wf-new")
	if err != nil || !bound {
		t.Fatalf("got %v %v", bound, err)
	}
	if bound, err := w.svc.WorkflowBound(context.Background(), "w1", "p1", "contato", "wf-other"); err != nil || bound {
		t.Fatalf("got %v %v", bound, err)
	}
}

func TestPageFormAutomationsRefuseAnotherWorkspaceAndBadIDsAndFailClosed(t *testing.T) {
	w := newWorld(t)
	if _, err := w.svc.Automations(context.Background(), "w2", "p1", "contato"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := w.svc.ScaffoldAutomation(context.Background(), "w1", "user-1", "p1", "Bad Id"); !errors.Is(err, ErrFormIDInvalid) {
		t.Fatalf("got %v", err)
	}
	w.automations.err = errors.New("db down")
	if bound, err := w.svc.WorkflowBound(context.Background(), "w1", "p1", "contato", "wf-1"); err == nil || bound {
		t.Fatalf("got %v %v", bound, err)
	}
}
