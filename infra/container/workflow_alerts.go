package container

import (
	"log"

	workflow_domain "vozko/domain/workflow"
	agenttool_repository "vozko/infra/repositories/agenttool"
	workflow_repository "vozko/infra/repositories/workflow"
	workflow_usecase "vozko/usecases/workflow"
)

func (c *Container) runtimeAlertStore() *workflow_repository.RuntimeAlertRepository {
	return workflow_repository.NewRuntimeAlertRepository(c.db)
}

func (c *Container) wireWorkflowRuntimeAlerts(steps *workflow_usecase.NodeExecutorRegistry) {
	store := c.runtimeAlertStore()
	workflows, ok := c.repositories.workflow.(workflow_usecase.WorkflowsByID)
	if !ok {
		log.Fatalf("[workflow-alerts] the workflow repository cannot read workflows by id")
	}
	alerts, err := workflow_usecase.NewRuntimeAlerts(workflow_usecase.RuntimeAlertsDeps{
		Alerts:     store,
		Signals:    store,
		Workflows:  workflows,
		Agents:     c.repositories.agent,
		Tools:      agenttool_repository.NewToolNames(c.db),
		Notices:    c.notifications().Publisher,
		Steps:      steps,
		Visibility: c.conversationVisibility(),
	})
	if err != nil {
		log.Fatalf("[workflow-alerts] %v", err)
	}
	c.useCases.workflowRuntimeAlerts = alerts
}

var _ workflow_domain.RuntimeAlertRecorder = (*workflow_repository.RuntimeAlertRepository)(nil)
