package copilot_usecase

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"vozko/domain/ai"
	"vozko/domain/copilot"
)

func registeredToolNames(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("copilottools/testdata/registry_tool_names.golden")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(raw))
}

func TestEveryAreaNamesOnlyRegisteredToolsAndEachToolOnce(t *testing.T) {
	registered := registeredToolNames(t)
	seen := map[string]string{}
	for i, area := range toolAreaCatalog {
		if i > 0 && toolAreaCatalog[i-1].Key >= area.Key {
			t.Errorf("areas must stay sorted by key: %s before %s", toolAreaCatalog[i-1].Key, area.Key)
		}
		if !slices.IsSorted(area.Tools) {
			t.Errorf("%s lists its tools out of order", area.Key)
		}
		for _, name := range area.Tools {
			if !slices.Contains(registered, name) {
				t.Errorf("%s names %s, which is not a registered tool", area.Key, name)
			}
			if previous, taken := seen[name]; taken {
				t.Errorf("%s is in %s and %s", name, previous, area.Key)
			}
			seen[name] = area.Key
		}
	}
	for surface, keys := range surfaceAreas {
		for _, key := range keys {
			if !slices.ContainsFunc(toolAreaCatalog, func(a toolArea) bool { return a.Key == key }) {
				t.Errorf("%s starts with the unknown area %s", surface, key)
			}
		}
	}
}

func areaDriver(view copilot.View) *Driver {
	cc := ownerCtx
	cc.View = view
	return NewDriver(cc, "m", NewRegistry(
		&fakeTool{name: "search_leads", meta: readMeta},
		&fakeTool{name: "list_agents", meta: readMeta},
		&fakeTool{name: "create_agent", meta: writeMeta},
		&fakeTool{name: "place_call", meta: readMeta},
	), &fakeAccess{}, openFunds{}, func() string { return "act-1" })
}

func TestTheLoaderAppearsOnlyWhenSomeToolLivesInAnArea(t *testing.T) {
	if got := offeredNames(driverWith(&fakeAccess{}, &fakeTool{name: "read_x", meta: readMeta})); got != "read_x" {
		t.Fatalf("a registry without area tools needs no loader, got %s", got)
	}
	if got := offeredNames(areaDriver(copilot.View{})); got != "load_tools,search_leads" {
		t.Fatalf("got %s", got)
	}
}

func TestAnAreaToolCalledBeforeItsAreaRunsAndLoadsTheArea(t *testing.T) {
	d := areaDriver(copilot.View{})
	step := d.Dispatch(context.Background(), call("list_agents", nil), (&capture{}).emit)
	if step.Result != `"done"` {
		t.Fatalf("result %q", step.Result)
	}
	if got := offeredNames(d); got != "create_agent,list_agents,load_tools,search_leads" {
		t.Fatalf("after the call the area stays loaded, got %s", got)
	}
}

func TestAreasUsedEarlierInTheConversationStayLoaded(t *testing.T) {
	d := areaDriver(copilot.View{})
	history := []ai.Message{
		{Role: ai.RoleUser, Content: "pedido"},
		{Role: ai.RoleAssistant, ToolCalls: []ai.ToolCall{{ID: "1", Name: "load_tools", Arguments: map[string]interface{}{"areas": []interface{}{"telefonia", "foguetes"}}}}},
		{Role: ai.RoleTool, ToolCallID: "1", Content: "{}"},
	}
	approved := []ai.Message{{Role: ai.RoleAssistant, ToolCalls: []ai.ToolCall{{ID: "2", Name: "create_agent"}}}}
	d.resume(history, approved)
	if got := offeredNames(d); got != "create_agent,list_agents,load_tools,place_call,search_leads" {
		t.Fatalf("got %s", got)
	}
}

func TestAWithheldToolStillNeedsApproval(t *testing.T) {
	d := areaDriver(copilot.View{})
	step := d.Dispatch(context.Background(), call("create_agent", map[string]interface{}{"name": "x"}), (&capture{}).emit)
	if step.Pause == nil {
		t.Fatalf("a mutating area tool must still pause for approval, got %+v", step)
	}
}

func TestTheEmailEditorNeverWithholdsTools(t *testing.T) {
	d := areaDriver(emailView)
	for _, def := range d.Tools() {
		if def.Name == loadToolsName {
			t.Fatal("the email editor has no loader")
		}
	}
	if !slices.Equal(defaultAreas(emailView), nil) {
		t.Fatal("the email editor starts with no area")
	}
}
