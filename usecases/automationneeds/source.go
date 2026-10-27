package automationneeds

import (
	"errors"

	"vozko/domain/agent"
	"vozko/domain/workflow"
)

type PublishedAgents interface {
	FindByID(agentID string) (*agent.Agent, error)
}

type PublishedSource struct {
	Agents    PublishedAgents
	Workflows workflow.PublishedReader
}

func (s PublishedSource) Agent(id string) (*agent.Agent, error) {
	found, err := s.Agents.FindByID(id)
	if errors.Is(err, agent.ErrAgentNotFound) || errors.Is(err, agent.ErrAgentNotPublished) {
		return nil, nil
	}
	return found, err
}

func (s PublishedSource) Workflow(id string) (*workflow.Workflow, error) {
	found, err := workflow.LoadPublished(s.Workflows, id)
	if errors.Is(err, workflow.ErrWorkflowNotFound) || errors.Is(err, workflow.ErrWorkflowNotPublished) {
		return nil, nil
	}
	return found, err
}
