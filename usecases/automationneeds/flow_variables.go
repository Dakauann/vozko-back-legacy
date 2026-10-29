package automationneeds

import (
	"context"
	"fmt"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

type FillerKind string

const (
	FilledByDefault  FillerKind = "default"
	FilledByCampaign FillerKind = "campaign"
	FilledByHandOff  FillerKind = "handoff"
	FilledByTransfer FillerKind = "transfer"
)

type Filler struct {
	Kind  FillerKind
	Value string
}

type NodeRef struct {
	ID   string
	Name string
}

type FlowVariable struct {
	Need     domain.Need
	Required bool
	InFlow   bool
	Used     bool
	Nodes    []NodeRef
	FilledBy []Filler
}

func (q *Queries) FlowVariables(ctx context.Context, workspaceID, workflowID string) ([]FlowVariable, error) {
	draft, err := q.d.Workflows.FindByID(workflowID)
	if err != nil || draft == nil || draft.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("%w: workflow %s", domain.ErrNotFound, workflowID)
	}
	root := conversation.Automation{Kind: conversation.AutomationWorkflow, ID: draft.ID}
	vars, err := domain.VariablesOf(domain.OverlayWorkflow(q.d.Service.Source(), draft), workspaceID, root)
	if err != nil {
		return nil, err
	}
	labels, err := q.labels(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	voice := draft.Type == workflow.WorkflowTypeVoice
	nodes := draft.Graph.CampaignVarNodes()
	row := func(need domain.Need, required, used bool) FlowVariable {
		_, inFlow := draft.Graph.VariableUse(need.Name)
		out := FlowVariable{Need: need, Required: required, InFlow: inFlow, Used: used, FilledBy: fillersOf(voice, need, required)}
		for _, id := range nodes[need.Name] {
			out.Nodes = append(out.Nodes, NodeRef{ID: id, Name: nodeName(draft, id)})
		}
		return out
	}
	var rows []FlowVariable
	seen := map[string]bool{}
	for _, need := range labels.Apply(vars.Required) {
		seen[need.Name] = true
		rows = append(rows, row(need, true, true))
	}
	for _, need := range labels.Apply(vars.Optional) {
		seen[need.Name] = true
		rows = append(rows, row(need, false, true))
	}
	var unused domain.Needs
	for _, use := range draft.Graph.Variables {
		if !seen[use.Name] && !variabledecl.IsSystemKey(use.Name) {
			seen[use.Name] = true
			unused = append(unused, domain.Need{Name: use.Name, Default: use.Default})
		}
	}
	for _, need := range labels.Apply(unused) {
		rows = append(rows, row(need, need.Default == "", false))
	}
	return rows, nil
}

func (q *Queries) labels(ctx context.Context, workspaceID string) (Labels, error) {
	catalog, err := q.d.Service.declarations.Catalog(ctx, workspaceID)
	if err != nil {
		return Labels{}, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	return Labels{catalog: catalog, registry: q.d.Service.declarations.Registry()}, nil
}

func fillersOf(voice bool, need domain.Need, required bool) []Filler {
	var out []Filler
	if need.Default != "" && !required {
		out = append(out, Filler{Kind: FilledByDefault, Value: need.Default})
	}
	if voice {
		return out
	}
	return append(out, Filler{Kind: FilledByCampaign}, Filler{Kind: FilledByHandOff}, Filler{Kind: FilledByTransfer})
}

func nodeName(draft *workflow.Workflow, id string) string {
	if n := draft.Graph.FindNode(id); n != nil {
		if name := n.DisplayName(); name != "" {
			return name
		}
	}
	return ""
}
