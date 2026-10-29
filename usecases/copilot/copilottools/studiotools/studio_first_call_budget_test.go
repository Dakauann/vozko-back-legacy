package studiotools_test

import (
	"context"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"vozko/domain/advertising"
	"vozko/domain/ai"
	"vozko/domain/copilot"
	"vozko/domain/tools"
	copilot_usecase "vozko/usecases/copilot"
	"vozko/usecases/copilot/copilottools/adtools"
	"vozko/usecases/copilot/copilottools/attendancetools"
	"vozko/usecases/copilot/copilottools/platformtools"
	"vozko/usecases/copilot/copilottools/skilltools"
	"vozko/usecases/copilot/copilottools/studiotools"
	"vozko/usecases/copilot/copilottools/tooltest"
	"vozko/usecases/copilot/skills"
)

const studioBytesPerToken = 2.78

var studioFirstCallTokenCeil = map[copilot.StudioKind]int{copilot.StudioImage: 17400, copilot.StudioVideo: 20600}

func studioTokens(chars int) int {
	return int(float64(chars)/studioBytesPerToken + 0.5)
}

func studioFirstCallDriver(t *testing.T, kind copilot.StudioKind) *copilot_usecase.Driver {
	t.Helper()
	library, err := skills.Load()
	if err != nil {
		t.Fatal(err)
	}
	toolset := append(studiotools.StudioTools(studiotools.StudioDeps{}), studiotools.NewNewStudioProjectTool(studiotools.StudioDeps{}))
	toolset = append(toolset, adtools.AllAdTools(adtools.AdsDeps{Accounts: noAdAccounts{}}, adtools.AdManageDeps{}, adtools.AdGrowthDeps{})...)
	toolset = append(toolset, skilltools.NewLoadSkillTool(library), platformtools.NewAskUserTool(), attendancetools.NewCalculateTool())
	view := copilot.View{Surface: copilot.SurfaceStudio, ProjectID: "5f0c7c1e-1d2a-4b8e-9d11-3a2b1c0d9e8f", ProjectKind: kind}
	cc := copilot.Context{WorkspaceID: "ws-1", UserID: "u-1", SystemAdmin: true, View: view}
	return copilot_usecase.NewDriver(cc, "m", copilot_usecase.NewRegistry(toolset...), nil, nil, nil)
}

func TestTheStudioFirstCallFitsItsCeiling(t *testing.T) {
	for _, kind := range []copilot.StudioKind{copilot.StudioImage, copilot.StudioVideo} {
		d := studioFirstCallDriver(t, kind)
		system := len(d.SystemPrompt())
		schemas := 0
		var sizes []string
		for _, def := range d.Tools() {
			size := tooltest.WireChars(def)
			schemas += size
			sizes = append(sizes, def.Name+" "+strconv.Itoa(studioTokens(size)))
		}
		sort.Strings(sizes)
		tokens := studioTokens(system + schemas)
		t.Logf("studio %s first call: %d tokens (system %d, %d tools %d: %s)", kind, tokens, studioTokens(system), len(d.Tools()), studioTokens(schemas), strings.Join(sizes, ", "))
		if tokens > studioFirstCallTokenCeil[kind] {
			t.Errorf("the first %s studio call costs %d tokens, over %d; keep ad tools behind their loader and edit fields in studio_op_guide", kind, tokens, studioFirstCallTokenCeil[kind])
		}
	}
}

func TestTheStudioPromptReadsTheOutlineThenOnlyWhatItEdits(t *testing.T) {
	for kind, ranged := range map[copilot.StudioKind]string{copilot.StudioImage: "artboard_id", copilot.StudioVideo: "track_id"} {
		prompt := studioFirstCallDriver(t, kind).SystemPrompt()
		if !strings.Contains(prompt, "esboço") || !strings.Contains(prompt, ranged) || !strings.Contains(prompt, "changes") {
			t.Errorf("the %s studio prompt does not say to read the outline and then only what it edits", kind)
		}
	}
}

var studioAdRoute = []string{"ad_account_readiness", "ad_placement_look", "connect_ad_account", "create_ad", "list_ad_accounts", "list_ad_pages", "list_lead_forms", "save_ad_draft", "search_ad_locations"}

func offeredOn(d *copilot_usecase.Driver) map[string]tools.Definition {
	offered := map[string]tools.Definition{}
	for _, def := range d.Tools() {
		offered[def.Name] = def
	}
	return offered
}

func TestTheStudioLoadsItsAdToolsOnlyWhenAsked(t *testing.T) {
	for _, kind := range []copilot.StudioKind{copilot.StudioImage, copilot.StudioVideo} {
		d := studioFirstCallDriver(t, kind)
		before := offeredOn(d)
		for _, name := range studioAdRoute {
			if _, ok := before[name]; ok {
				t.Fatalf("%s: %s is in the first studio call; ad tools load on demand", kind, name)
			}
		}
		loader, ok := before["load_tools"]
		if !ok || !slices.Equal(loader.Parameters["areas"].Items.Enum, []string{"anuncios"}) {
			t.Fatalf("%s: the studio must offer load_tools with only the ad area, got %+v", kind, loader.Parameters["areas"])
		}
		step := d.Dispatch(context.Background(), ai.ToolCall{ID: "c1", Name: "load_tools", Arguments: map[string]interface{}{"areas": []interface{}{"anuncios"}}}, func(string, interface{}) {})
		if !strings.Contains(step.Result, "create_ad") {
			t.Fatalf("%s: loading the ad area answered %s", kind, step.Result)
		}
		after := offeredOn(d)
		for _, name := range studioAdRoute {
			if _, ok := after[name]; !ok {
				t.Errorf("%s: %s is still missing after load_tools(anuncios)", kind, name)
			}
		}
		if _, ok := after["turn_on_ad"]; ok {
			t.Errorf("%s: loading ads on the studio must keep the studio route only", kind)
		}
		ran := d.Dispatch(context.Background(), ai.ToolCall{ID: "c2", Name: "list_ad_accounts", Arguments: map[string]interface{}{}}, func(string, interface{}) {})
		if strings.Contains(ran.Result, "INDISPONÍVEL") || strings.Contains(ran.Result, "desconhecida") {
			t.Errorf("%s: a loaded ad tool must run on the studio, got %s", kind, ran.Result)
		}
	}
}

type noAdAccounts struct{}

func (noAdAccounts) List(context.Context, string) ([]*advertising.AdAccount, error) { return nil, nil }
