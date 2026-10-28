package copilot_usecase

import (
	"context"
	"testing"

	"vozko/domain/ai"
	"vozko/domain/copilot"
	"vozko/usecases/agentloop"
)

type routingTool struct {
	fakeTool
}

func (r *routingTool) NeedsApproval(mode copilot.Mode) bool { return mode == copilot.ModeAsk }

func (r *routingTool) NeedsApprovalFor(_ copilot.Mode, args map[string]interface{}) bool {
	return args["section"] == "redirects"
}

func TestDriver_AnArgumentRuleAsksForApprovalInEveryMode(t *testing.T) {
	full := ownerCtx
	full.Mode = copilot.ModeFull
	for _, c := range []struct {
		section string
		paused  bool
	}{{"redirects", true}, {"pages", false}} {
		tool := &routingTool{fakeTool: fakeTool{name: "settings_x", meta: writeMeta}}
		drv := NewDriver(full, "m", NewRegistry(tool), &fakeAccess{}, openFunds{}, func() string { return "act-1" })
		e := agentloop.Engine{AI: &scriptAI{turns: [][]ai.ToolCall{{call("settings_x", map[string]interface{}{"section": c.section})}, {}}, texts: []string{"", "pronto"}}}
		out := e.Run(context.Background(), (&capture{}).emit, drv, DefaultConfig(full, ai.ModelInfo{}), &agentloop.Session{}, "mude")
		if paused := out.Kind == agentloop.OutcomePaused; paused != c.paused {
			t.Fatalf("%s: paused %v, got %+v", c.section, c.paused, out)
		}
		if c.paused && tool.calls != 0 {
			t.Fatalf("%s: never runs before approval", c.section)
		}
		if !c.paused && tool.calls != 1 {
			t.Fatalf("%s: runs in full mode", c.section)
		}
	}
}
