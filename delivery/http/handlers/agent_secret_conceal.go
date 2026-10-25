package handlers

import (
	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/tools"
)

func (h *AgentHandler) concealed(a *agent.Agent) *agent.Agent {
	if a == nil || len(a.InternalTools) == 0 {
		return a
	}
	var defs []tools.Definition
	if h.toolRegistry != nil {
		defs = h.toolRegistry.Definitions()
	}
	out := *a
	out.InternalTools = agenttool.ConcealBindings(defs, a.InternalTools)
	return &out
}
