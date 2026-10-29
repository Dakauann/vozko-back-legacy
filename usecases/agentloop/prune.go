package agentloop

import (
	"fmt"

	"vozko/domain/ai"
)

const (
	prunedOutputNamed   = "[resultado antigo de %s removido para economizar contexto; chame a ferramenta de novo se precisar dele]"
	prunedOutputUnnamed = "[resultado antigo removido para economizar contexto]"
)

func prunedOutput(tool string) string {
	if tool == "" {
		return prunedOutputUnnamed
	}
	return fmt.Sprintf(prunedOutputNamed, tool)
}

type prunableOutput struct {
	index  int
	marker string
	saves  int
}

func PruneToolOutputs(history []ai.Message) []ai.Message {
	names := map[string]string{}
	for _, m := range history {
		for _, call := range m.ToolCalls {
			names[call.ID] = call.Name
		}
	}
	var older []prunableOutput
	seen, available := 0, 0
	for i := len(history) - 1; i >= 0; i-- {
		m := history[i]
		if m.Role != ai.RoleTool {
			continue
		}
		size := estimateTokens(m.Content)
		seen += size
		if seen <= ai.PruneProtectTokens {
			continue
		}
		marker := prunedOutput(names[m.ToolCallID])
		if saves := size - estimateTokens(marker); saves > 0 {
			older = append(older, prunableOutput{index: i, marker: marker, saves: saves})
			available += saves
		}
	}
	quota := available / ai.PruneMinimumTokens * ai.PruneMinimumTokens
	if quota == 0 {
		return history
	}
	out := append([]ai.Message(nil), history...)
	freed := 0
	for j := len(older) - 1; j >= 0 && freed < quota; j-- {
		o := older[j]
		out[o.index] = ai.Message{Role: ai.RoleTool, ToolCallID: history[o.index].ToolCallID, Content: o.marker}
		freed += o.saves
	}
	return out
}
