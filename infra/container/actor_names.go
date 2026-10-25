package container

import (
	"log"

	"vozko/usecases/actornames"
)

func (c *Container) actorNames() actornames.Directory {
	return actornames.Directory{Users: c.repositories.user, Agents: c.repositories.agent, Workflows: c.workflowNames()}
}

func (c *Container) workflowNames() actornames.WorkflowLookup {
	workflows, ok := c.repositories.workflow.(actornames.WorkflowLookup)
	if !ok {
		log.Fatalf("[actor-names] the workflow repository cannot read workflows by id, workflow actors would show no name")
	}
	return workflows
}
