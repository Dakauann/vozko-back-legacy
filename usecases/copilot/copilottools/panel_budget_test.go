package copilottools_test

import (
	"sort"
	"strconv"
	"strings"
	"testing"

	"vozko/domain/copilot"
	copilot_usecase "vozko/usecases/copilot"
	"vozko/usecases/copilot/copilottools/tooltest"
)

const (
	panelBytesPerToken       = 2.78
	panelFirstCallTokenCeil  = 20000
	screenFirstCallTokenCeil = 27000
)

func panelDriver(t *testing.T, view copilot.View) *copilot_usecase.Driver {
	t.Helper()
	cc := copilot.Context{WorkspaceID: "ws-1", UserID: "u-1", SystemAdmin: true, View: view}
	return copilot_usecase.NewDriver(cc, "m", copilot_usecase.NewRegistry(registeredTools(t)...), nil, nil, nil)
}

func panelTokens(chars int) int {
	return int(float64(chars)/panelBytesPerToken + 0.5)
}

func TestThePanelFirstCallFitsItsCeiling(t *testing.T) {
	for _, view := range []copilot.View{{}, {Surface: copilot.SurfaceAttendance}, {Surface: copilot.SurfaceLeads}} {
		d := panelDriver(t, view)
		system := len(d.SystemPrompt())
		schemas := 0
		var sizes []string
		for _, def := range d.Tools() {
			size := tooltest.WireChars(def)
			schemas += size
			sizes = append(sizes, def.Name+" "+strconv.Itoa(panelTokens(size)))
		}
		sort.Strings(sizes)
		tokens := panelTokens(system + schemas)
		t.Logf("panel %q first call: %d tokens (system %d, %d tools %d: %s)", view.Surface, tokens, panelTokens(system), len(d.Tools()), panelTokens(schemas), strings.Join(sizes, ", "))
		ceil := panelFirstCallTokenCeil
		if view.Surface != "" {
			ceil = screenFirstCallTokenCeil
		}
		if tokens > ceil {
			t.Errorf("the first panel call on %q costs %d tokens, over %d; defer area tools behind load_tools", view.Surface, tokens, ceil)
		}
	}
}
