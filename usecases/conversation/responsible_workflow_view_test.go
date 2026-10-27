package conversation_usecase

import (
	"errors"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

type viewVisibility struct {
	visible bool
	err     error
}

func (v viewVisibility) CanView(shared.Person, string, string, string) (bool, error) {
	return v.visible, v.err
}

func (v viewVisibility) Scope(shared.Person, string) (conversation.VisibilityScope, error) {
	return conversation.VisibilityScope{}, nil
}

type viewWorkflows struct {
	byID  map[string]*workflow.Workflow
	err   error
	asked []string
}

func (w *viewWorkflows) FindByID(id string) (*workflow.Workflow, error) {
	w.asked = append(w.asked, id)
	if w.err != nil {
		return nil, w.err
	}
	return w.byID[id], nil
}

func responsibleView(profile gateProfiles, visibility viewVisibility, workflows *viewWorkflows) *ResponsibleWorkflowView {
	return NewResponsibleWorkflowView(ResponsibleWorkflowViewDeps{
		Gate:       responsibilityGate(ownedBy(""), profile),
		Visibility: visibility,
		Workflows:  workflows,
	})
}

var viewer = shared.Person{UserID: "u-1"}

func TestTheOpenConversationNamesItsResponsibleWorkflow(t *testing.T) {
	one := 1
	workflows := &viewWorkflows{byID: map[string]*workflow.Workflow{
		"wf-1": {ID: "wf-1", WorkspaceID: "ws-1", Name: "Atendimento", Status: workflow.WorkflowStatusActive, PublishedVersion: &one},
	}}
	view := responsibleView(gateProfiles{profile: conversation.AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true}}, viewVisibility{visible: true}, workflows)

	got, err := view.Execute(viewer, "ws-1", "e-1", string(shared.EntryTypeInstagram))
	if err != nil {
		t.Fatal(err)
	}
	want := &ResponsibleWorkflow{ID: "wf-1", Name: "Atendimento", State: workflow.RunningActive}
	if got == nil || *got != *want {
		t.Fatalf("responsible = %+v, want %+v", got, want)
	}
}

func TestAPausedResponsibleWorkflowSaysItDoesNotRun(t *testing.T) {
	one := 1
	workflows := &viewWorkflows{byID: map[string]*workflow.Workflow{
		"wf-1": {ID: "wf-1", WorkspaceID: "ws-1", Name: "Atendimento", Status: workflow.WorkflowStatusPaused, PublishedVersion: &one},
	}}
	view := responsibleView(gateProfiles{profile: conversation.AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true}}, viewVisibility{visible: true}, workflows)
	got, err := view.Execute(viewer, "ws-1", "e-1", string(shared.EntryTypeWhatsApp))
	if err != nil || got == nil || got.State != workflow.RunningPaused {
		t.Fatalf("responsible = %+v, %v", got, err)
	}
}

func TestNoResponsibleWorkflowIsNothingToShow(t *testing.T) {
	cases := map[string]struct {
		profile   conversation.AutomationProfile
		workflows map[string]*workflow.Workflow
	}{
		"an agent answers":          {profile: conversation.AutomationProfile{AgentID: "a-1", AgentResponsesEnabled: true}},
		"workflows switched off":    {profile: conversation.AutomationProfile{WorkflowID: "wf-1"}},
		"the workflow was deleted":  {profile: conversation.AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true}},
		"a workflow from elsewhere": {profile: conversation.AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true}, workflows: map[string]*workflow.Workflow{"wf-1": {ID: "wf-1", WorkspaceID: "ws-9", Name: "Alheio"}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			view := responsibleView(gateProfiles{profile: tc.profile}, viewVisibility{visible: true}, &viewWorkflows{byID: tc.workflows})
			got, err := view.Execute(viewer, "ws-1", "e-1", string(shared.EntryTypeWhatsApp))
			if err != nil || got != nil {
				t.Fatalf("responsible = %+v, %v; want none", got, err)
			}
		})
	}
}

func TestAHiddenConversationRevealsNoWorkflow(t *testing.T) {
	workflows := &viewWorkflows{}
	view := responsibleView(gateProfiles{profile: conversation.AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true}}, viewVisibility{}, workflows)
	if _, err := view.Execute(viewer, "ws-1", "e-1", string(shared.EntryTypeWhatsApp)); !errors.Is(err, conversation.ErrNotVisible) {
		t.Fatalf("err = %v, want not visible", err)
	}
	if len(workflows.asked) != 0 {
		t.Fatalf("read %v before checking visibility", workflows.asked)
	}
}

func TestAnUnreadableResponsibilityIsAnErrorNotAGuess(t *testing.T) {
	down := errors.New("db down")
	cases := map[string]*ResponsibleWorkflowView{
		"the automation": responsibleView(gateProfiles{err: down}, viewVisibility{visible: true}, &viewWorkflows{}),
		"the workflow":   responsibleView(gateProfiles{profile: conversation.AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true}}, viewVisibility{visible: true}, &viewWorkflows{err: down}),
	}
	for name, view := range cases {
		if got, err := view.Execute(viewer, "ws-1", "e-1", string(shared.EntryTypeWhatsApp)); !errors.Is(err, down) || got != nil {
			t.Errorf("%s: responsible = %+v, err = %v; want the read error", name, got, err)
		}
	}
}

func TestTheResponsibleWorkflowViewNeedsEveryDependency(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a view without its workflow reader must not be built")
		}
	}()
	NewResponsibleWorkflowView(ResponsibleWorkflowViewDeps{Gate: responsibilityGate(ownedBy(""), gateProfiles{}), Visibility: viewVisibility{}})
}
