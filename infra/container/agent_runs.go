package container

import (
	"log"

	"vozko/domain/agentrun"
	"vozko/domain/agentscenario"
	"vozko/domain/ai"
	agentrun_repository "vozko/infra/repositories/agentrun"
	agentscenario_repository "vozko/infra/repositories/agentscenario"
	agentrun_usecase "vozko/usecases/agentrun"
	agentscenario_usecase "vozko/usecases/agentscenario"
)

func (c *Container) agentRunStore() agentrun.Repository {
	return agentrun_repository.NewRepository(c.db)
}

func (c *Container) agentTurnAI() ai.Service {
	return agentrun_usecase.RecordAgentRuns(c.services.ai, c.services.aiAttendanceService, c.agentRunStore(), c.services.metrics)
}

func (c *Container) sandboxedAgentTurnAI(sandboxed ai.Service) ai.Service {
	return agentrun_usecase.RecordAgentRuns(sandboxed, nil, c.agentRunStore(), c.services.metrics)
}

func (c *Container) agentRuns() agentrun_usecase.Runs {
	return agentrun_usecase.NewRuns(c.agentRunStore(), c.conversationVisibility())
}

func (c *Container) agentScenarioStore() agentscenario.Repository {
	return agentscenario_repository.NewRepository(c.db)
}

func (c *Container) agentScenarios() *agentscenario_usecase.Service {
	judge, err := agentscenario_usecase.NewJudge(c.services.ai)
	if err != nil {
		log.Fatalf("[agent-scenarios] %v", err)
	}
	svc, err := agentscenario_usecase.New(agentscenario_usecase.Deps{
		Scenarios: c.agentScenarioStore(),
		Simulate:  c.useCases.simulateAgentTurn,
		Agents:    c.repositories.agent,
		Runs:      c.agentRuns(),
		Judge:     judge,
	})
	if err != nil {
		log.Fatalf("[agent-scenarios] %v", err)
	}
	return svc
}
