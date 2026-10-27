package container

import (
	"log"

	workflow_repository "vozko/infra/repositories/workflow"
	ia_usecase "vozko/usecases/inbox_assignment"
	workflow_usecase "vozko/usecases/workflow"
)

func (c *Container) automationDirectory() ia_usecase.AutomationDirectory {
	return ia_usecase.AutomationDirectory{Agents: c.repositories.agent, Workflows: c.repositories.workflow}
}

func (c *Container) automationToggle() *ia_usecase.OperatorAutomationToggle {
	return ia_usecase.NewOperatorAutomationToggle(c.services.conversationAutomation, c.services.assignmentService, c.services.conversationAuth, c.automationNeeds().service)
}

func (c *Container) automationDelegation(toggle *ia_usecase.OperatorAutomationToggle) *ia_usecase.AutomationDelegation {
	runs := workflow_usecase.NewConversationRunSwitch(workflow_repository.NewConversationRunCanceller(c.db), c.tapWorkflows(), c.repositories.workflowRunLog)
	return ia_usecase.NewAutomationDelegation(c.services.delegations, toggle, c.automationDirectory(), runs, c.automationNeeds().service)
}

func (c *Container) buttonDelegations() *ia_usecase.AutomationDelegation {
	if c.services.conversationAutomation == nil || c.services.assignmentService == nil || c.services.delegations == nil {
		log.Fatalf("[lead-exit] handing a conversation to an automation from a button needs the conversation automation, the assignment service and the delegations")
	}
	return c.automationDelegation(c.automationToggle())
}
