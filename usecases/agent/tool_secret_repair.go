package agent_usecase

import (
	"context"
	"fmt"
	"reflect"

	"vozko/domain/agent"
	"vozko/domain/tools"
)

const toolSecretRepairStart = "00000000-0000-0000-0000-000000000000"

type ToolSecretRepair struct {
	toolSecretSealing
	store    agent.StoredToolScanner
	registry tools.Service
	batch    int
}

func NewToolSecretRepair(store agent.StoredToolScanner, registry tools.Service, sealer ToolSecretSealer, batch int) *ToolSecretRepair {
	if batch <= 0 {
		batch = 200
	}
	return &ToolSecretRepair{toolSecretSealing: toolSecretSealing{sealer: sealer}, store: store, registry: registry, batch: batch}
}

func (r *ToolSecretRepair) Run(ctx context.Context) (int, error) {
	moved := 0
	after := toolSecretRepairStart
	for {
		rows, err := r.store.AgentToolsAfter(ctx, after, r.batch)
		if err != nil {
			return moved, err
		}
		if len(rows) == 0 {
			return moved, nil
		}
		for _, row := range rows {
			after = row.AgentID
			sealed := make([]agent.ToolBinding, 0, len(row.Bindings))
			for _, binding := range row.Bindings {
				one := []agent.ToolBinding{binding}
				out, err := r.sealBindings(ctx, r.registry, row.WorkspaceID, "", row.AgentID, one, one)
				if err != nil {
					return moved, fmt.Errorf("agent %s: %w", row.AgentID, err)
				}
				sealed = append(sealed, out...)
			}
			if reflect.DeepEqual(sealed, row.Bindings) {
				continue
			}
			if err := r.store.SaveAgentTools(ctx, row.AgentID, sealed); err != nil {
				return moved, fmt.Errorf("agent %s: %w", row.AgentID, err)
			}
			moved++
		}
	}
}
