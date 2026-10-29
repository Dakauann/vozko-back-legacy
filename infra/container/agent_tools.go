package container

import (
	"context"
	"log"
	"log/slog"
	"vozko/delivery/http/automationneedshttp"
	"vozko/delivery/http/variabledeclhttp"

	deliveryHttp "vozko/delivery/http"
	"vozko/delivery/http/agentpublishhttp"
	"vozko/delivery/http/agentrunhttp"
	"vozko/delivery/http/agentscenariohttp"
	"vozko/delivery/http/agenttoolhttp"
	agent_domain "vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/infra/netguard"
	agent_repository "vozko/infra/repositories/agent"
	agenttool_repository "vozko/infra/repositories/agenttool"
	agent_usecase "vozko/usecases/agent"
	agenttool_usecase "vozko/usecases/agenttool"
)

func (c *Container) agentToolValidator() *agenttool_usecase.Validator {
	if c.agentToolValidatorService == nil {
		c.agentToolValidatorService = agenttool_usecase.NewValidator(agenttool_usecase.ValidatorDeps{
			Registry:  c.services.toolRegistry,
			Secrets:   c.workspaceSecretStore(),
			Hosts:     netguard.NewHostVerdicts(nil),
			Models:    c.services.ai,
			Variables: automationVariableRules{c: c},
		})
	}
	return c.agentToolValidatorService
}

func (c *Container) validateAgentTools() {
	validator := c.agentToolValidator()
	for name, uc := range map[string]any{"create agent": c.useCases.createAgent, "update agent": c.useCases.updateAgent} {
		validated, ok := uc.(agent_usecase.ToolValidated)
		if !ok {
			log.Fatalf("[agent-tools] the %s use case cannot validate its tools before an active save", name)
		}
		validated.ValidateToolsWith(validator, c.publishedReader())
	}
}

func (c *Container) agentTools() agent_domain.AgentToolsUseCase {
	if c.agentToolsService == nil {
		toolsUC := agent_usecase.NewAgentToolsUseCase(c.repositories.agent, c.useCases.updateAgent, c.services.toolRegistry, agenttool_repository.NewRevisionReader(c.db))
		toolsUC.(agent_usecase.ToolValidated).ValidateToolsWith(c.agentToolValidator(), c.publishedReader())
		c.agentToolsService = toolsUC
	}
	return c.agentToolsService
}

func (c *Container) agentToolChecks() agenttool.ToolsValidation {
	validation, ok := c.agentTools().(agenttool.ToolsValidation)
	if !ok {
		log.Fatalf("[agent-tools] the tools use case cannot validate drafts")
	}
	return validation
}

func (c *Container) agentToolHandler() *agenttoolhttp.Handler {
	handler, err := agenttoolhttp.NewHandler(agenttoolhttp.Deps{
		Tools:      c.agentTools(),
		Agents:     c.useCases.getAgent,
		Registry:   c.services.toolRegistry,
		Validation: c.agentToolChecks(),
		Tester:     agent_usecase.NewToolTester(c.repositories.agent, c.services.toolRegistry),
		MCPSources: c.mcpRegistry,
	})
	if err != nil {
		log.Fatalf("[agent-tools] %v", err)
	}
	return handler
}

func agentRoutes(c *Container) deliveryHttp.AgentRoutes {
	return deliveryHttp.AgentRoutes{
		Tools:          c.agentToolHandler(),
		Publishing:     agentpublishhttp.NewHandler(c.agentPublishing(), c.useCases.getAgent, c.services.toolRegistry),
		Runs:           agentrunhttp.NewHandler(c.agentRuns(), c.useCases.getAgent),
		Scenarios:      agentscenariohttp.NewHandler(c.agentScenarios(), c.useCases.getAgent),
		Needs:          automationneedshttp.NewHandler(c.automationQueries()),
		AgentVariables: automationneedshttp.NewAgentVariablesHandler(c.useCases.getAgent, c.automationQueries()),
		Variables:      variabledeclhttp.NewHandler(c.variableDeclarations()),
	}
}

func (c *Container) publishedAgents() agent_domain.Repository {
	if c.publishedAgentsView == nil {
		view, err := agent_repository.NewPublishedView(c.repositories.agent)
		if err != nil {
			log.Fatalf("[agents] the agent repository cannot read published versions: %v", err)
		}
		c.publishedAgentsView = view
	}
	return c.publishedAgentsView
}

func (c *Container) publishedReader() agent_domain.PublishedReader {
	reader, ok := c.repositories.agent.(agent_domain.PublishedReader)
	if !ok {
		log.Fatalf("[agents] the agent repository cannot read published versions")
	}
	return reader
}

func (c *Container) agentVersions() agent_domain.VersionRepository {
	versions, ok := c.repositories.agent.(agent_domain.VersionRepository)
	if !ok {
		log.Fatalf("[agents] the agent repository cannot store versions")
	}
	return versions
}

func (c *Container) agentPublishing() agent_usecase.AgentPublishing {
	if c.agentPublishingService == nil {
		c.agentPublishingService = agent_usecase.NewPublishAgentUseCase(agent_usecase.PublishDeps{
			Agents:    c.repositories.agent,
			Versions:  c.agentVersions(),
			Switching: c.agentSwitchingPublisher(),
			Planner:   c.automationPublishGuard(),
			Published: c.publishedReader(),
			Update:    c.useCases.updateAgent,
			Validator: c.agentToolValidator(),
		})
	}
	return c.agentPublishingService
}

func (c *Container) publishExistingAgents() {
	scanner, ok := c.repositories.agent.(agent_domain.UnpublishedScanner)
	if !ok {
		log.Fatalf("[agents] the agent repository cannot list unpublished agents")
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentToolSecretRepairTimeout)
	defer cancel()
	published, err := agent_usecase.NewPublishExistingAgents(c.repositories.agent, c.agentVersions(), scanner).Run(ctx)
	if err != nil {
		slog.Error("agent.publish_existing.failed", slog.String("repair", "ag_publish_existing_agents"), slog.Int("published", published), slog.String("error", err.Error()))
		return
	}
	slog.Info("agent.publish_existing.done", slog.String("repair", "ag_publish_existing_agents"), slog.Int("published", published))
}

func (c *Container) agentSwitchingPublisher() agent_usecase.SwitchingPublisher {
	switching, ok := c.repositories.agent.(agent_usecase.SwitchingPublisher)
	if !ok {
		log.Fatalf("[agents] the agent repository cannot publish and switch receptive channels off in one write")
	}
	return switching
}
