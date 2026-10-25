package container

import (
	"testing"

	agent_repository "vozko/infra/repositories/agent"
)

func TestAgentAccessorsUsedBeforeTheUseCasesExistNeverTouchThem(t *testing.T) {
	c := &Container{repositories: &repositories{agent: agent_repository.NewCachedRepository(agent_repository.NewRepository(nil), nil)}}
	first := c.publishedAgents()
	if first == nil || c.publishedAgents() != first {
		t.Fatal("the published view must be built once")
	}
	if c.useCases != nil {
		t.Fatal("the accessor must not create the use cases")
	}
	if c.publishedReader() == nil || c.agentVersions() == nil {
		t.Fatal("the agent repository must read and store versions")
	}
}
