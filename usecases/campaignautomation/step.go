package campaignautomation

import (
	"errors"
	"fmt"
	"strings"

	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	"vozko/domain/campaign"
	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

type Workflows interface {
	Execute(workflowID string) (*workflow.Workflow, error)
}

type Agents interface {
	Execute(agentID string) (*agent.Agent, error)
}

type Step struct {
	workflows    Workflows
	agents       Agents
	refuseSheets bool
	channel      shared.EntryType
}

func NewStep(workflows Workflows, agents Agents) (*Step, error) {
	if workflows == nil || agents == nil {
		return nil, campaign.ErrAutomationUnavailable
	}
	return &Step{workflows: workflows, agents: agents}, nil
}

func (s *Step) WithoutSheets() *Step {
	if s == nil {
		return nil
	}
	copied := *s
	copied.refuseSheets = true
	return &copied
}

func (s *Step) OnChannel(channel shared.EntryType) *Step {
	if s == nil {
		return nil
	}
	copied := *s
	copied.channel = channel
	return &copied
}

func (s *Step) refuseOffChannel(src automationneeds.Source, workspaceID string, root conversation.Automation) error {
	if s.channel == "" {
		return nil
	}
	walk, err := automationneeds.ChannelWalkOf(src, workspaceID, root, []shared.EntryType{s.channel})
	if err != nil {
		return fmt.Errorf("%w: %w", campaign.ErrAutomationUnavailable, err)
	}
	return automationneeds.ChannelRefusal(root, walk)
}

type Rule struct {
	needs    automationneeds.Needs
	optional automationneeds.Needs
}

func (r Rule) Needs() automationneeds.Needs {
	return r.needs
}

func (r Rule) Optional() automationneeds.Needs {
	return r.optional
}

func (r Rule) Check(entries []campaign.EntryMetadata) error {
	for _, entry := range entries {
		missing := r.needs.MissingMetadata(entry.Values())
		if len(missing) == 0 {
			continue
		}
		first := missing[0]
		if first.Owner().Kind == conversation.AutomationAgent {
			return fmt.Errorf("%s: %w: variable %q is missing or empty", entry.Label, campaign.ErrAgentVarsMissing, first.Name)
		}
		return fmt.Errorf("%w: key %q is missing or empty for %s", campaign.ErrWorkflowVarsMissing, first.Name, entry.Label)
	}
	return nil
}

func (r Rule) Requires() bool {
	return len(r.needs) > 0
}

func (r Rule) Missing(metadata map[string]any) []string {
	return r.needs.MissingMetadata(metadata).Names()
}

func (s *Step) Check(workspaceID string, a campaign.Automation, entries []campaign.EntryMetadata) error {
	rule, err := s.Rule(workspaceID, a)
	if err != nil {
		return err
	}
	return rule.Check(entries)
}

func (s *Step) Rule(workspaceID string, a campaign.Automation) (Rule, error) {
	if s == nil || s.workflows == nil || s.agents == nil {
		return Rule{}, campaign.ErrAutomationUnavailable
	}
	var vars automationneeds.Variables
	if id := strings.TrimSpace(a.WorkflowID); id != "" {
		wf, err := s.ownWorkflow(workspaceID, id)
		if err != nil {
			return Rule{}, err
		}
		src, root := automationneeds.OverlayWorkflow(stepSource{step: s}, wf), conversation.Automation{Kind: conversation.AutomationWorkflow, ID: id}
		if err := s.refuseOffChannel(src, workspaceID, root); err != nil {
			return Rule{}, err
		}
		found, err := s.needsOf(src, workspaceID, root)
		if err != nil {
			return Rule{}, err
		}
		vars = vars.Union(found)
	}
	if id := strings.TrimSpace(a.AgentID); id != "" {
		owned, err := s.ownAgent(workspaceID, id)
		if err != nil {
			return Rule{}, err
		}
		src, root := automationneeds.OverlayAgent(stepSource{step: s}, owned), conversation.Automation{Kind: conversation.AutomationAgent, ID: id}
		if err := s.refuseOffChannel(src, workspaceID, root); err != nil {
			return Rule{}, err
		}
		found, err := s.needsOf(src, workspaceID, root)
		if err != nil {
			return Rule{}, err
		}
		vars = vars.Union(found)
	}
	if s.refuseSheets && len(vars.Required.SheetNeeds())+len(vars.Optional.SheetNeeds()) > 0 {
		return Rule{}, campaign.ErrSheetsOfficialOnly
	}
	return Rule{needs: vars.Required, optional: vars.Optional}, nil
}

func (s *Step) needsOf(src automationneeds.Source, workspaceID string, root conversation.Automation) (automationneeds.Variables, error) {
	vars, err := automationneeds.VariablesOf(src, workspaceID, root)
	switch {
	case err == nil:
		return vars, nil
	case errors.Is(err, automationneeds.ErrNotFound):
		return automationneeds.Variables{}, fmt.Errorf("%w: %w", campaign.ErrAgentNotFound, err)
	}
	return automationneeds.Variables{}, fmt.Errorf("%w: %w", campaign.ErrAutomationUnavailable, err)
}

func (s *Step) ownWorkflow(workspaceID, workflowID string) (*workflow.Workflow, error) {
	wf, err := s.workflows.Execute(workflowID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", campaign.ErrWorkflowNotFound, err)
	}
	if wf == nil {
		return nil, campaign.ErrWorkflowNotFound
	}
	if wf.WorkspaceID != workspaceID {
		return nil, campaign.ErrWorkflowForbidden
	}
	return wf, nil
}

func (s *Step) ownAgent(workspaceID, agentID string) (*agent.Agent, error) {
	found, err := s.agents.Execute(agentID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", campaign.ErrAgentNotFound, err)
	}
	if found == nil || found.WorkspaceID != workspaceID {
		return nil, campaign.ErrAgentNotFound
	}
	return found, nil
}

type stepSource struct{ step *Step }

func (s stepSource) Agent(id string) (*agent.Agent, error) {
	found, err := s.step.agents.Execute(id)
	if errors.Is(err, agent.ErrAgentNotFound) {
		return nil, nil
	}
	return found, err
}

func (s stepSource) Workflow(id string) (*workflow.Workflow, error) {
	found, err := s.step.workflows.Execute(id)
	if errors.Is(err, workflow.ErrWorkflowNotFound) {
		return nil, nil
	}
	return found, err
}

type Checker interface {
	Check(workspaceID string, a campaign.Automation, entries []campaign.EntryMetadata) error
}

type Rules interface {
	Rule(workspaceID string, a campaign.Automation) (Rule, error)
}

type StoredEntries func(visit func(campaign.EntryMetadata) error) error

func Require(step Checker, workspaceID string, a campaign.Automation, entries []campaign.EntryMetadata) error {
	if !a.Configured() {
		return nil
	}
	if step == nil {
		return campaign.ErrAutomationUnavailable
	}
	return step.Check(workspaceID, a, entries)
}

func RequireStored(rules Rules, workspaceID string, before, after campaign.Automation, stored StoredEntries) error {
	if !after.Configured() || after == before {
		return nil
	}
	if rules == nil || stored == nil {
		return campaign.ErrAutomationUnavailable
	}
	rule, err := rules.Rule(workspaceID, after)
	if err != nil {
		return err
	}
	return stored(func(entry campaign.EntryMetadata) error {
		return rule.Check([]campaign.EntryMetadata{entry})
	})
}
