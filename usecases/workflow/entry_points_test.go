package workflow_usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"vozko/domain/workflow"
)

type recordingEntryPointReader struct {
	triggerCalls [][]string
	sourceCalls  []workflow.EntryPointSourceQuery
	activeCalls  []string
	triggers     map[string][]workflow.Node
	sources      *workflow.EntryPointSources
	active       []workflow.ActiveTrigger
	err          error
	activeErr    error
}

func (r *recordingEntryPointReader) ActiveTriggers(workspaceID string, types []workflow.TriggerType) ([]workflow.ActiveTrigger, error) {
	r.activeCalls = append(r.activeCalls, workspaceID)
	if !reflect.DeepEqual(types, workflow.OverlappingTriggerTypes()) {
		return nil, errors.New("asked for triggers that cannot overlap")
	}
	return r.active, r.activeErr
}

func (r *recordingEntryPointReader) PublishedTriggers(ids []string) (map[string][]workflow.Node, error) {
	r.triggerCalls = append(r.triggerCalls, ids)
	return r.triggers, r.err
}

func (r *recordingEntryPointReader) EntryPointSources(q workflow.EntryPointSourceQuery) (*workflow.EntryPointSources, error) {
	r.sourceCalls = append(r.sourceCalls, q)
	if r.err != nil {
		return nil, r.err
	}
	return r.sources, nil
}

func entryPointsRepo(t *testing.T) *MockWorkflowRepository {
	t.Helper()
	repo := NewMockWorkflowRepository()
	webhookDraft := workflow.Graph{Nodes: []workflow.Node{{ID: "t", Type: workflow.NodeTypeTriggerWebhook}, {ID: "end", Type: workflow.NodeTypeEnd}}, Edges: []workflow.Edge{{Source: "t", Target: "end"}}}
	for _, w := range []*workflow.Workflow{
		{ID: "wf-1", WorkspaceID: "ws1", Name: "Publicado", Status: workflow.WorkflowStatusActive, Type: workflow.WorkflowTypeMessages, Graph: manualGraph("end"), Version: 1},
		{ID: "wf-2", WorkspaceID: "ws1", Name: "Rascunho", Status: workflow.WorkflowStatusDraft, Type: workflow.WorkflowTypeMessages, Graph: webhookDraft, Version: 1},
	} {
		if err := repo.Create(w); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

func TestTheListCarriesEntryPointsLoadedOnceForTheWholePage(t *testing.T) {
	repo := entryPointsRepo(t)
	reader := &recordingEntryPointReader{
		triggers: map[string][]workflow.Node{"wf-1": {{ID: "agent", Type: workflow.NodeTypeTriggerAgentCall}}},
		sources:  &workflow.EntryPointSources{Agents: map[string][]workflow.EntryAgent{"wf-1": {{ID: "a-1", Name: "Bia", Active: true}}}},
	}
	list := NewListWorkflowsWithEntryPoints(NewListWorkflowsUseCase(repo), NewEntryPoints(reader))

	page, err := list.Execute(workflow.ListWorkflowsInput{WorkspaceID: "ws1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.triggerCalls) != 1 || !reflect.DeepEqual(reader.triggerCalls[0], []string{"wf-1"}) || len(reader.sourceCalls) != 1 {
		t.Fatalf("trigger reads = %v, source reads = %d, want one of each for the page and published triggers only for published workflows", reader.triggerCalls, len(reader.sourceCalls))
	}
	byID := map[string]*workflow.Workflow{}
	for _, w := range page.Items {
		byID[w.ID] = w
	}
	if eps := byID["wf-1"].EntryPoints; len(eps) != 1 || eps[0].Kind != workflow.EntryAgentCall || eps[0].Readiness != workflow.EntryReady {
		t.Fatalf("published entry points = %+v, want the published agent trigger, ready", eps)
	}
	if eps := byID["wf-2"].EntryPoints; len(eps) != 1 || eps[0].Kind != workflow.EntryWebhook || eps[0].Reason != workflow.EntryBlockedNotPublished {
		t.Fatalf("draft entry points = %+v, want the draft webhook blocked as not published", eps)
	}
}

func TestAnEntryPointReadFailureFailsTheListInsteadOfGuessing(t *testing.T) {
	repo := entryPointsRepo(t)
	broken := errors.New("db down")
	list := NewListWorkflowsWithEntryPoints(NewListWorkflowsUseCase(repo), NewEntryPoints(&recordingEntryPointReader{err: broken}))
	if _, err := list.Execute(workflow.ListWorkflowsInput{WorkspaceID: "ws1"}); !errors.Is(err, broken) {
		t.Fatalf("err = %v, want the read failure", err)
	}
}

func TestAnUnwiredEntryPointReaderIsRefused(t *testing.T) {
	repo := entryPointsRepo(t)
	list := NewListWorkflowsWithEntryPoints(NewListWorkflowsUseCase(repo), NewEntryPoints(nil))
	if _, err := list.Execute(workflow.ListWorkflowsInput{WorkspaceID: "ws1"}); !errors.Is(err, ErrEntryPointsUnavailable) {
		t.Fatalf("err = %v, want unavailable", err)
	}
}

func TestTheDetailCarriesItsEntryPoints(t *testing.T) {
	repo := entryPointsRepo(t)
	reader := &recordingEntryPointReader{sources: &workflow.EntryPointSources{Webhooks: map[string]workflow.WebhookEntry{"wf-2": {Active: true, Method: "POST"}}}}
	uc := NewScopedWorkflowsUseCase(ScopedWorkflowsDeps{
		Get: NewGetWorkflowUseCase(repo), Update: NewUpdateWorkflowUseCase(repo, staticCatalog{}), Versions: NewWorkflowVersionsUseCase(repo), EntryPoints: NewEntryPoints(reader),
	})
	detail, err := uc.Detail(ownerScope("u-1"), "wf-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.EntryPoints) != 1 || detail.EntryPoints[0].Details.Webhook == nil || len(reader.triggerCalls) != 0 {
		t.Fatalf("detail entry points = %+v, published reads = %v", detail.EntryPoints, reader.triggerCalls)
	}
	if reader.sourceCalls[0].WorkspaceID != "ws1" {
		t.Fatalf("query = %+v, want the scope's workspace", reader.sourceCalls[0])
	}
	if _, err := uc.Update(context.Background(), ownerScope("u-1"), "wf-2", &workflow.Workflow{Name: "x", Graph: detail.Graph}, detail.Version); err != nil {
		t.Fatalf("the detail must stay saveable: %v", err)
	}
}

func birthdayPage(t *testing.T) (*MockWorkflowRepository, *recordingEntryPointReader) {
	t.Helper()
	repo := entryPointsRepo(t)
	reader := &recordingEntryPointReader{
		triggers: map[string][]workflow.Node{"wf-1": {{ID: "bday", Type: workflow.NodeTypeTriggerBirthday}}},
		sources:  &workflow.EntryPointSources{},
		active: []workflow.ActiveTrigger{
			{WorkflowID: "wf-1", WorkflowName: "Publicado", Node: workflow.Node{ID: "bday", Type: workflow.NodeTypeTriggerBirthday}},
			{WorkflowID: "wf-9", WorkflowName: "Cupom de aniversário", Node: workflow.Node{ID: "b", Type: workflow.NodeTypeTriggerBirthday}},
		},
	}
	return repo, reader
}

func TestTheListCarriesARunSummaryWithOverlapsFromOneWorkspaceRead(t *testing.T) {
	repo, reader := birthdayPage(t)
	list := NewListWorkflowsWithEntryPoints(NewListWorkflowsUseCase(repo), NewEntryPoints(reader))

	page, err := list.Execute(workflow.ListWorkflowsInput{WorkspaceID: "ws1"})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*workflow.Workflow{}
	for _, w := range page.Items {
		byID[w.ID] = w
	}
	running := byID["wf-1"].RunSummary
	if running == nil || running.State != workflow.RunningActive || len(running.Conditions) != 1 || running.Conditions[0].Kind != workflow.EntryBirthday {
		t.Fatalf("published summary = %+v", running)
	}
	if len(running.Overlaps) != 1 || !reflect.DeepEqual(running.Overlaps[0].Workflows, []workflow.WorkflowRef{{ID: "wf-9", Name: "Cupom de aniversário"}}) {
		t.Fatalf("overlaps = %+v", running.Overlaps)
	}
	draft := byID["wf-2"].RunSummary
	if draft == nil || draft.State != workflow.RunningDraft || len(draft.Overlaps) != 0 || len(draft.Conditions) != 1 || draft.Conditions[0].Kind != workflow.EntryWebhook {
		t.Fatalf("draft summary = %+v", draft)
	}
	if !reflect.DeepEqual(reader.activeCalls, []string{"ws1"}) {
		t.Fatalf("active trigger reads = %v, want one for the whole page", reader.activeCalls)
	}
}

func TestNoOverlapReadWhenNothingOnThePageCanOverlap(t *testing.T) {
	repo := entryPointsRepo(t)
	reader := &recordingEntryPointReader{
		triggers: map[string][]workflow.Node{"wf-1": {{ID: "agent", Type: workflow.NodeTypeTriggerAgentCall}}},
		sources:  &workflow.EntryPointSources{},
	}
	list := NewListWorkflowsWithEntryPoints(NewListWorkflowsUseCase(repo), NewEntryPoints(reader))
	if _, err := list.Execute(workflow.ListWorkflowsInput{WorkspaceID: "ws1"}); err != nil {
		t.Fatal(err)
	}
	if len(reader.activeCalls) != 0 {
		t.Fatalf("active trigger reads = %v, want none", reader.activeCalls)
	}
}

func TestAnOverlapReadFailureFailsTheListInsteadOfHidingTheWarning(t *testing.T) {
	repo, reader := birthdayPage(t)
	reader.activeErr = errors.New("db down")
	list := NewListWorkflowsWithEntryPoints(NewListWorkflowsUseCase(repo), NewEntryPoints(reader))
	if _, err := list.Execute(workflow.ListWorkflowsInput{WorkspaceID: "ws1"}); !errors.Is(err, reader.activeErr) {
		t.Fatalf("err = %v, want the read failure", err)
	}
}

func TestThePublishPreviewReadsTheStoredDraftAgainstTheOtherActiveWorkflows(t *testing.T) {
	repo, reader := birthdayPage(t)
	draft := workflow.Graph{Nodes: []workflow.Node{
		{ID: "form", Type: workflow.NodeTypeTriggerFormSubmitted, Config: map[string]interface{}{workflow.FormTriggerFormIDsKey: []interface{}{"f-1"}}},
		{ID: "bday", Type: workflow.NodeTypeTriggerBirthday},
		{ID: "end", Type: workflow.NodeTypeEnd},
	}}
	one := 1
	if err := repo.Create(&workflow.Workflow{ID: "wf-5", WorkspaceID: "ws1", Name: "Editado", Status: workflow.WorkflowStatusActive, Type: workflow.WorkflowTypeMessages, Graph: draft, Version: 1, PublishedVersion: &one}); err != nil {
		t.Fatal(err)
	}
	reader.sources = &workflow.EntryPointSources{Forms: map[string]workflow.EntryForm{"f-1": {ID: "f-1", Name: "Cadastro", Active: true}}}
	uc := NewScopedWorkflowsUseCase(ScopedWorkflowsDeps{Get: NewGetWorkflowUseCase(repo), EntryPoints: NewEntryPoints(reader)})

	preview, err := uc.PublishPreview(ownerScope("u-1"), "wf-5")
	if err != nil {
		t.Fatal(err)
	}
	if preview.State != workflow.RunningActive || len(preview.Conditions) != 2 || preview.Conditions[0].Targets[0].Name != "Cadastro" {
		t.Fatalf("preview = %+v", preview)
	}
	if len(preview.Overlaps) != 1 || len(preview.Overlaps[0].Workflows) != 2 {
		t.Fatalf("overlaps = %+v", preview.Overlaps)
	}
	if len(reader.triggerCalls) != 0 || len(reader.sourceCalls) != 1 || !reflect.DeepEqual(reader.sourceCalls[0].FormIDs, []string{"f-1"}) {
		t.Fatalf("the preview reads only the draft: triggers %v, sources %+v", reader.triggerCalls, reader.sourceCalls)
	}
}

func TestThePublishPreviewOfAnotherWorkspaceIsNotFound(t *testing.T) {
	repo, reader := birthdayPage(t)
	uc := NewScopedWorkflowsUseCase(ScopedWorkflowsDeps{Get: NewGetWorkflowUseCase(repo), EntryPoints: NewEntryPoints(reader)})
	scope := ownerScope("u-1")
	scope.WorkspaceID = "ws-other"
	if _, err := uc.PublishPreview(scope, "wf-1"); !errors.Is(err, workflow.ErrWorkflowNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestAnUnwiredPublishPreviewIsRefused(t *testing.T) {
	repo, _ := birthdayPage(t)
	uc := NewScopedWorkflowsUseCase(ScopedWorkflowsDeps{Get: NewGetWorkflowUseCase(repo)})
	if _, err := uc.PublishPreview(ownerScope("u-1"), "wf-1"); !errors.Is(err, ErrEntryPointsUnavailable) {
		t.Fatalf("err = %v, want unavailable", err)
	}
}
