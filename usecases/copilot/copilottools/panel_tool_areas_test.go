package copilottools_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"vozko/domain/ai"
	"vozko/domain/copilot"
	copilot_usecase "vozko/usecases/copilot"
)

var panelCore = []string{
	"ask_user", "calculate", "get_lead", "list_departments", "load_skill", "load_tools", "offer_action", "open_screen", "query_dataset",
	"read_conversation", "render_chart", "search_conversations", "search_leads",
}

func offeredNames(d *copilot_usecase.Driver) []string {
	var names []string
	for _, def := range d.Tools() {
		names = append(names, def.Name)
	}
	return names
}

func loadAreas(t *testing.T, d *copilot_usecase.Driver, areas ...interface{}) agentloopResult {
	t.Helper()
	step := d.Dispatch(context.Background(), ai.ToolCall{ID: "c1", Name: "load_tools", Arguments: map[string]interface{}{"areas": areas}}, func(string, interface{}) {})
	return agentloopResult(step.Result)
}

type agentloopResult string

func panelAreaKeys(t *testing.T) []string {
	t.Helper()
	for _, def := range panelDriver(t, copilot.View{}).Tools() {
		if def.Name == "load_tools" {
			return def.Parameters["areas"].Items.Enum
		}
	}
	t.Fatal("load_tools is not offered on the panel")
	return nil
}

func TestThePanelStartsWithTheCoreToolsOnly(t *testing.T) {
	if got := offeredNames(panelDriver(t, copilot.View{})); !slices.Equal(got, panelCore) {
		t.Fatalf("the panel starts with %v, want %v", got, panelCore)
	}
}

func TestEveryRegisteredToolIsReachable(t *testing.T) {
	reached := map[string]bool{}
	for _, entry := range snapshotViews {
		d := panelDriver(t, entry.view)
		for _, def := range d.Tools() {
			if def.Name != "load_tools" {
				continue
			}
			var args []interface{}
			for _, key := range def.Parameters["areas"].Items.Enum {
				args = append(args, key)
			}
			loadAreas(t, d, args...)
		}
		for _, name := range offeredNames(d) {
			reached[name] = true
		}
	}
	site := panelDriver(t, copilot.View{Surface: copilot.SurfaceSite, ProjectID: "3b9a7c2d-5e6f-4a1b-8c9d-0e1f2a3b4c5d"})
	for _, name := range offeredNames(site) {
		reached[name] = true
	}
	for _, tool := range registeredTools(t) {
		if name := tool.Definition().Name; !reached[name] {
			t.Errorf("%s is not reachable on any surface", name)
		}
	}
}

func TestEveryPanelToolIsCoreOrInExactlyOneArea(t *testing.T) {
	areas := panelAreaKeys(t)
	owner := map[string]string{}
	for _, key := range areas {
		d := panelDriver(t, copilot.View{})
		loadAreas(t, d, key)
		for _, name := range offeredNames(d) {
			if slices.Contains(panelCore, name) {
				continue
			}
			if previous, taken := owner[name]; taken {
				t.Errorf("%s is in %s and %s", name, previous, key)
			}
			owner[name] = key
		}
	}
	for _, tool := range registeredTools(t) {
		name := tool.Definition().Name
		if (copilot.View{}).Offers(tool) && owner[name] == "" && !slices.Contains(panelCore, name) {
			t.Errorf("%s is offered on the panel but belongs to no area", name)
		}
	}
}

func TestLoadingAnAreaAddsItsToolsSortedForTheRestOfTheAnswer(t *testing.T) {
	d := panelDriver(t, copilot.View{})
	result := loadAreas(t, d, "anuncios-publicos")
	var loaded struct {
		Areas []string `json:"areas"`
		Tools []string `json:"tools"`
	}
	if err := json.Unmarshal([]byte(result), &loaded); err != nil {
		t.Fatalf("%s: %v", result, err)
	}
	audiences := []string{"create_customer_list_audience", "create_lookalike_audience", "create_saved_audience", "delete_ad_audience", "list_ad_audiences"}
	if !slices.Equal(loaded.Areas, []string{"anuncios-publicos"}) || !slices.Equal(loaded.Tools, audiences) {
		t.Fatalf("load result %s", result)
	}
	want := slices.Sorted(slices.Values(append(slices.Clone(panelCore), audiences...)))
	if got := offeredNames(d); !slices.Equal(got, want) {
		t.Fatalf("after loading, the panel offers %v, want %v", got, want)
	}
	loadAreas(t, d, "telefonia")
	got := offeredNames(d)
	if !slices.IsSorted(got) || !slices.Contains(got, "list_ad_audiences") || !slices.Contains(got, "place_call") {
		t.Fatalf("a second load keeps the first area and stays sorted: %v", got)
	}
}

func TestAnUnknownAreaIsRefusedWithACodeAndLoadsNothing(t *testing.T) {
	d := panelDriver(t, copilot.View{})
	result := string(loadAreas(t, d, "anuncios", "foguetes"))
	if !strings.HasPrefix(result, "error: unknown_tool_area:") || !strings.Contains(result, "foguetes") || !strings.Contains(result, "anuncios-gestao") {
		t.Fatalf("result %q", result)
	}
	if got := offeredNames(d); !slices.Equal(got, panelCore) {
		t.Fatalf("a refused load must load nothing, got %v", got)
	}
}

func TestTheMetricsScreenStartsWithTheAttendanceArea(t *testing.T) {
	got := offeredNames(panelDriver(t, copilot.View{Surface: copilot.SurfaceAttendance}))
	if !slices.Contains(got, "attendance_metrics") || !slices.Contains(got, "member_activity") || slices.Contains(got, "create_ad") {
		t.Fatalf("the metrics screen offers %v", got)
	}
	leads := offeredNames(panelDriver(t, copilot.View{Surface: copilot.SurfaceLeads}))
	if !slices.Contains(leads, "prepare_lead_action") || slices.Contains(leads, "attendance_metrics") {
		t.Fatalf("the leads screen offers %v", leads)
	}
}

func TestOnlyThePanelAndTheStudioOfferTheLoader(t *testing.T) {
	for _, entry := range snapshotViews {
		if !entry.view.Focused() || entry.view.OnStudio() {
			continue
		}
		if slices.Contains(offeredNames(panelDriver(t, entry.view)), "load_tools") {
			t.Errorf("%s offers load_tools", entry.name)
		}
	}
}

func TestTheStudioLoaderOffersOnlyTheAdArea(t *testing.T) {
	for _, entry := range snapshotViews {
		if !entry.view.OnStudio() {
			continue
		}
		d := panelDriver(t, entry.view)
		result := string(loadAreas(t, d, "telefonia"))
		if !strings.HasPrefix(result, "error: unknown_tool_area:") || strings.Contains(result, "telefonia,") {
			t.Fatalf("%s loaded a panel area: %s", entry.name, result)
		}
		for _, name := range offeredNames(d) {
			if name == "place_call" {
				t.Fatalf("%s offers a panel tool", entry.name)
			}
		}
	}
}
