package agent_usecase

import (
	"context"
	"errors"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
)

type ToolValidator interface {
	Validate(ctx context.Context, a *agent.Agent, stored []agent.ToolBinding) agenttool.Report
}

type ToolValidated interface {
	ValidateToolsWith(validator ToolValidator, published agent.PublishedReader)
}

type toolValidation struct {
	validator ToolValidator
	published agent.PublishedReader
}

func (v *toolValidation) ValidateToolsWith(validator ToolValidator, published agent.PublishedReader) {
	v.validator = validator
	v.published = published
}

func (v *toolValidation) refuseBrokenActivation(ctx context.Context, wasActive bool, a *agent.Agent) error {
	if a == nil || wasActive || !a.IsActive || a.Archived {
		return nil
	}
	if v.published == nil || v.validator == nil {
		return agenttool.BlockedBy(agenttool.ErrNotActivatable, agenttool.Unavailable("a conferência das ferramentas não está ligada"))
	}
	live, err := v.published.FindPublished(a.ID)
	if errors.Is(err, agent.ErrAgentNotPublished) {
		return nil
	}
	if err != nil {
		return agenttool.BlockedBy(agenttool.ErrNotActivatable, agenttool.Unavailable("a versão publicada não pôde ser lida"))
	}
	return agenttool.BlockedBy(agenttool.ErrNotActivatable, v.validator.Validate(ctx, live, live.InternalTools))
}
