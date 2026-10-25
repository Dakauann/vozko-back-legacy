package container

import (
	"context"
	"log/slog"
	"time"

	"log"

	agent_domain "vozko/domain/agent"
	"vozko/domain/workspacesecret"
	"vozko/infra/crypto/piigorm"
	agenttool_repository "vozko/infra/repositories/agenttool"
	workspacesecret_repository "vozko/infra/repositories/workspacesecret"
	agent_usecase "vozko/usecases/agent"
	agenttool_usecase "vozko/usecases/agenttool"
	workspacesecret_usecase "vozko/usecases/workspacesecret"
)

func (c *Container) workspaceSecretStore() workspacesecret.UseCases {
	if c.workspaceSecretService == nil {
		c.workspaceSecretService = c.newWorkspaceSecrets()
	}
	return c.workspaceSecretService
}

func (c *Container) newWorkspaceSecrets() workspacesecret.UseCases {
	cipher := piigorm.Service()
	if cipher == nil {
		log.Fatalf("[container] workspace secrets need the PII encryption service")
	}
	svc, err := workspacesecret_usecase.New(workspacesecret_usecase.Deps{
		Secrets: workspacesecret_repository.New(c.db),
		Cipher:  cipher,
	})
	if err != nil {
		log.Fatalf("[container] workspace secrets: %v", err)
	}
	return svc
}

func (c *Container) sealAgentToolSecrets() {
	sealer := agenttool_usecase.NewSealer(c.workspaceSecretStore())
	for name, uc := range map[string]any{"create agent": c.useCases.createAgent, "update agent": c.useCases.updateAgent} {
		sealed, ok := uc.(agent_usecase.ToolSecretSealed)
		if !ok {
			log.Fatalf("[agent-tool-secrets] the %s use case cannot move tool credentials into the workspace secrets", name)
		}
		sealed.SealToolSecretsWith(sealer)
	}
	c.repairAgentToolSecrets(sealer)
}

const agentToolSecretRepairBatch = 200

func (c *Container) repairAgentToolSecrets(sealer agent_usecase.ToolSecretSealer) {
	ctx, cancel := context.WithTimeout(context.Background(), agentToolSecretRepairTimeout)
	defer cancel()
	invalidate := func(string) {}
	if cached, ok := c.repositories.agent.(interface{ Invalidate(string) }); ok {
		invalidate = cached.Invalidate
	}
	stores := map[string]agent_domain.StoredToolScanner{
		"live":      agenttool_repository.NewLiveToolScanner(c.db, invalidate),
		"revisions": agenttool_repository.NewRevisionScanner(c.db),
	}
	for _, scope := range []string{"live", "revisions"} {
		moved, err := agent_usecase.NewToolSecretRepair(stores[scope], c.services.toolRegistry, sealer, agentToolSecretRepairBatch).Run(ctx)
		if err != nil {
			slog.Error("agent.tool_secret_repair.failed", slog.String("repair", agentToolSecretRepairName), slog.String("scope", scope), slog.Int("agents_sealed", moved), slog.String("error", err.Error()))
			continue
		}
		slog.Info("agent.tool_secret_repair.done", slog.String("repair", agentToolSecretRepairName), slog.String("scope", scope), slog.Int("agents_sealed", moved))
	}
}

const (
	agentToolSecretRepairName    = "ag_tool_header_secrets"
	agentToolSecretRepairTimeout = 2 * time.Minute
)
