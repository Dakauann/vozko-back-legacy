package container

import (
	"log"

	automationneeds_repository "vozko/infra/repositories/automationneeds"
	automationneeds_usecase "vozko/usecases/automationneeds"
	conversationvars_usecase "vozko/usecases/conversationvars"
	workflow_usecase "vozko/usecases/workflow"
)

type automationNeedsWiring struct {
	variables *automationneeds_repository.ConversationVariables
	reader    *conversationvars_usecase.Reader
	service   *automationneeds_usecase.Service
	alerts    *automationneeds_usecase.Alerts
	runtime   *automationneeds_usecase.Runtime
	receptive *automationneeds_usecase.Receptive
	slots     *automationneeds_repository.Slots
	records   *automationneeds_repository.SwitchOffRecords
	publish   *automationneeds_usecase.PublishGuard
}

func (c *Container) automationNeeds() *automationNeedsWiring {
	if c.automationNeedsWiring != nil {
		return c.automationNeedsWiring
	}
	if c.services.conversationHistory == nil {
		log.Fatalf("[automation-needs] the conversation history must exist before the variables read path")
	}
	variables := automationneeds_repository.NewConversationVariables(c.db)
	reader, err := conversationvars_usecase.NewReader(conversationvars_usecase.ReaderDeps{
		Facts:     c.services.conversationHistory,
		Campaigns: variables,
		Store:     variables,
	})
	if err != nil {
		log.Fatalf("[automation-needs] %v", err)
	}
	service, err := automationneeds_usecase.NewService(automationneeds_usecase.ServiceDeps{
		Source: automationneeds_usecase.PublishedSource{Agents: c.publishedAgents(), Workflows: c.repositories.workflow},
		Values: reader,
	})
	if err != nil {
		log.Fatalf("[automation-needs] %v", err)
	}
	alerts, err := automationneeds_usecase.NewAlerts(c.notifications().Publisher)
	if err != nil {
		log.Fatalf("[automation-needs] %v", err)
	}
	runtime, err := automationneeds_usecase.NewRuntime(service, alerts)
	if err != nil {
		log.Fatalf("[automation-needs] %v", err)
	}
	records := automationneeds_repository.NewSwitchOffRecords(c.db)
	receptive, err := automationneeds_usecase.NewReceptive(service, records, alerts)
	if err != nil {
		log.Fatalf("[automation-needs] %v", err)
	}
	c.automationNeedsWiring = &automationNeedsWiring{variables: variables, reader: reader, service: service, alerts: alerts, runtime: runtime, receptive: receptive, slots: automationneeds_repository.NewSlots(c.db), records: records}
	return c.automationNeedsWiring
}

func (c *Container) automationPublishGuard() *automationneeds_usecase.PublishGuard {
	wiring := c.automationNeeds()
	if wiring.publish != nil {
		return wiring.publish
	}
	if c.services.assignmentService == nil {
		log.Fatalf("[automation-needs] the publish guard needs the assignment service to hand held conversations to the team")
	}
	containers, ok := c.repositories.wcCampaign.(automationneeds_usecase.ContainerCache)
	if !ok {
		log.Fatalf("[automation-needs] the official campaign repository cannot forget a cached receptive container")
	}
	guard, err := automationneeds_usecase.NewPublishGuard(automationneeds_usecase.PublishGuardDeps{
		Service:    wiring.service,
		Slots:      wiring.slots,
		Alerts:     wiring.alerts,
		Held:       automationneeds_repository.NewHeld(c.db),
		HandOff:    c.services.assignmentService,
		Containers: containers,
	})
	if err != nil {
		log.Fatalf("[automation-needs] %v", err)
	}
	wiring.publish = guard
	return guard
}

func (c *Container) workflowSwitchingPublisher() workflow_usecase.WorkflowSwitchingPublisher {
	switching, ok := c.repositories.workflow.(workflow_usecase.WorkflowSwitchingPublisher)
	if !ok {
		log.Fatalf("[automation-needs] the workflow repository cannot publish and switch receptive channels off in one write")
	}
	return switching
}

func (c *Container) automationQueries() *automationneeds_usecase.Queries {
	wiring := c.automationNeeds()
	if c.services.conversationAuthImpl == nil {
		log.Fatalf("[automation-needs] the conversation access check must exist before the variables form")
	}
	queries, err := automationneeds_usecase.NewQueries(automationneeds_usecase.QueriesDeps{
		Service:   wiring.service,
		Guard:     c.automationPublishGuard(),
		Agents:    c.repositories.agent,
		Workflows: c.repositories.workflow,
		Records:   wiring.records,
		Store:     wiring.variables,
		Access:    c.services.conversationAuthImpl,
	})
	if err != nil {
		log.Fatalf("[automation-needs] %v", err)
	}
	return queries
}
