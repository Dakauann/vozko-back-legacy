package container

import (
	"log"

	"vozko/domain/tools"
	"vozko/usecases/agentturn"
	automationtransfer_usecase "vozko/usecases/automationtransfer"
	ce_usecase "vozko/usecases/conversation_event"
	stage_usecase "vozko/usecases/stage"
	tools_usecase "vozko/usecases/tools"
)

func (c *Container) wireAutomationTransfer(turnAssembler *agentturn.Assembler) {
	registrar, ok := c.services.toolRegistry.(interface{ Register(tools.Handler) })
	if !ok {
		log.Fatalf("[automation-transfer] the agent tool registry cannot take the transfer_to_automation tool")
	}
	timeline := ce_usecase.NewLogger(c.services.crmTelemetryPublisher)
	service, err := automationtransfer_usecase.New(automationtransfer_usecase.Deps{
		Agents:      c.repositories.agent,
		Workflows:   c.repositories.workflow,
		Governance:  c.services.automationGate,
		Delegations: c.buttonDelegations(),
		Stages:      stage_usecase.NewAssignEntryStageUseCase(c.repositories.stage, c.repositories.pipeline, timeline),
		Placement:   c.repositories.stage,
		Remover:     stage_usecase.NewRemoveEntryStageUseCase(c.repositories.stage, timeline),
		Broadcaster: c.services.conversationHub,
		Timeline:    c.repositories.conversationEvent,
		Alerts:      c.automationNeeds().alerts,
		Needs:       c.automationNeeds().service,
		Values:      c.automationQueries(),
		Variables:   c.automationNeeds().variables,
	})
	if err != nil {
		log.Fatalf("[automation-transfer] %v", err)
	}
	registrar.Register(tools_usecase.NewTransferToAutomationTool(service))
	turnAssembler.TrackHandOffs(service)
}
