package agent_usecase

import (
	"context"
	"strings"

	"vozko/domain/agent"
	"vozko/domain/tools"
	agenttool_usecase "vozko/usecases/agenttool"
)

type ToolSecretSealer interface {
	Seal(ctx context.Context, req agenttool_usecase.SealRequest) (map[string]interface{}, error)
}

type ToolSecretSealed interface {
	SealToolSecretsWith(sealer ToolSecretSealer)
}

type toolSecretSealing struct {
	sealer ToolSecretSealer
}

func (s *toolSecretSealing) SealToolSecretsWith(sealer ToolSecretSealer) {
	s.sealer = sealer
}

func (s *toolSecretSealing) sealBindings(ctx context.Context, registry tools.Service, workspaceID, userID, agentID string, previous, next []agent.ToolBinding) ([]agent.ToolBinding, error) {
	sealer := s.sealer
	if sealer == nil {
		sealer = agenttool_usecase.NewSealer(nil)
	}
	defs := definitionsByName(registry)
	out := make([]agent.ToolBinding, 0, len(next))
	for _, binding := range next {
		def, ok := defs[strings.ToLower(binding.Name)]
		if !ok || len(binding.Config) == 0 {
			out = append(out, binding)
			continue
		}
		sealed, err := sealer.Seal(ctx, agenttool_usecase.SealRequest{
			WorkspaceID: workspaceID,
			UserID:      userID,
			AgentID:     agentID,
			ToolName:    functionOf(def, binding),
			Sensitive:   def.SensitiveConfig(),
			Config:      binding.Config,
			Previous:    previousConfig(def, previous, binding),
		})
		if err != nil {
			return nil, err
		}
		binding.Config = sealed
		out = append(out, binding)
	}
	return out, nil
}

func functionOf(def tools.Definition, binding agent.ToolBinding) string {
	if binding.Function != "" {
		return binding.Function
	}
	return def.LegacyName(binding.Config)
}

func definitionsByName(registry tools.Service) map[string]tools.Definition {
	out := map[string]tools.Definition{}
	if registry == nil {
		return out
	}
	for _, def := range registry.Definitions() {
		out[strings.ToLower(strings.TrimSpace(def.Name))] = def
	}
	return out
}

func previousConfig(def tools.Definition, previous []agent.ToolBinding, binding agent.ToolBinding) map[string]interface{} {
	for _, b := range previous {
		if binding.ID != "" && b.ID == binding.ID {
			return b.Config
		}
	}
	if binding.ID != "" {
		return nil
	}
	target := def.TargetOf(binding.Config)
	for _, b := range previous {
		if strings.EqualFold(b.Name, def.Name) && def.TargetOf(b.Config) == target {
			return b.Config
		}
	}
	return nil
}
