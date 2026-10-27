package workflow_usecase

import (
	"errors"

	"vozko/domain/shared"
	"vozko/domain/workflow"
)

var ErrEntryPointsUnavailable = errors.New("workflow: entry points cannot be read")

type EntryPointAttacher interface {
	Attach(workspaceID string, workflows []*workflow.Workflow) error
	Preview(workspaceID string, w *workflow.Workflow) (*workflow.RunSummary, error)
}

type EntryPoints struct {
	reader workflow.EntryPointReader
}

func NewEntryPoints(reader workflow.EntryPointReader) *EntryPoints {
	return &EntryPoints{reader: reader}
}

func (e *EntryPoints) Attach(workspaceID string, workflows []*workflow.Workflow) error {
	if len(workflows) == 0 {
		return nil
	}
	if e == nil || e.reader == nil {
		return ErrEntryPointsUnavailable
	}
	var published []string
	for _, w := range workflows {
		if w != nil && w.PublishedVersion != nil {
			published = append(published, w.ID)
		}
	}
	triggers := map[string][]workflow.Node{}
	if len(published) > 0 {
		read, err := e.reader.PublishedTriggers(published)
		if err != nil {
			return err
		}
		triggers = read
	}
	sources, err := e.reader.EntryPointSources(workflow.EntryPointQueryOf(workspaceID, workflows, triggers))
	if err != nil {
		return err
	}
	active, err := e.activeTriggersFor(workspaceID, workflows, triggers)
	if err != nil {
		return err
	}
	for _, w := range workflows {
		if w == nil {
			continue
		}
		running, facts := workflow.RunningTriggers(w, triggers), sources.FactsOf(w.ID)
		w.EntryPoints = workflow.EntryPointsOf(w, running, facts)
		summary := workflow.RunSummaryOf(w, running, facts, active)
		w.RunSummary = &summary
	}
	return nil
}

func (e *EntryPoints) activeTriggersFor(workspaceID string, workflows []*workflow.Workflow, triggers map[string][]workflow.Node) ([]workflow.ActiveTrigger, error) {
	for _, w := range workflows {
		if w != nil && workflow.RunningStateOf(w) == workflow.RunningActive && workflow.HasOverlappingTrigger(workflow.RunningTriggers(w, triggers)) {
			return e.reader.ActiveTriggers(workspaceID, workflow.OverlappingTriggerTypes())
		}
	}
	return nil, nil
}

func (e *EntryPoints) Preview(workspaceID string, w *workflow.Workflow) (*workflow.RunSummary, error) {
	if e == nil || e.reader == nil || w == nil {
		return nil, ErrEntryPointsUnavailable
	}
	draft := workflow.DraftTriggers(w)
	sources, err := e.reader.EntryPointSources(workflow.EntryPointQueryFor(workspaceID, map[string][]workflow.Node{w.ID: draft}))
	if err != nil {
		return nil, err
	}
	var active []workflow.ActiveTrigger
	if workflow.HasOverlappingTrigger(draft) {
		if active, err = e.reader.ActiveTriggers(workspaceID, workflow.OverlappingTriggerTypes()); err != nil {
			return nil, err
		}
	}
	preview := workflow.PublishPreviewOf(w, sources.FactsOf(w.ID), active)
	return &preview, nil
}

type listWithEntryPoints struct {
	list        workflow.ListWorkflowsUseCase
	entryPoints EntryPointAttacher
}

func NewListWorkflowsWithEntryPoints(list workflow.ListWorkflowsUseCase, entryPoints EntryPointAttacher) workflow.ListWorkflowsUseCase {
	return &listWithEntryPoints{list: list, entryPoints: entryPoints}
}

func (uc *listWithEntryPoints) Execute(input workflow.ListWorkflowsInput) (*shared.PaginatedResult[*workflow.Workflow], error) {
	page, err := uc.list.Execute(input)
	if err != nil || page == nil {
		return page, err
	}
	if uc.entryPoints == nil {
		return nil, ErrEntryPointsUnavailable
	}
	if err := uc.entryPoints.Attach(input.WorkspaceID, page.Items); err != nil {
		return nil, err
	}
	return page, nil
}
