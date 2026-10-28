package workflow

import (
	"encoding/json"
	"errors"
	"testing"
)

func sheetStep(variable any) *Node {
	config := map[string]interface{}{SheetStepSheetKey: "model-pag"}
	if variable != nil {
		config[SheetStepVariableKey] = variable
	}
	return &Node{ID: "n1", Type: NodeTypeActionAddSheetRow, Config: config}
}

func TestASheetStepNamesItsVariableWithOneConversationToken(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		want    string
		set     bool
		invalid bool
	}{
		{name: "fixed", value: nil},
		{name: "blank", value: "  "},
		{name: "a conversation variable", value: "{{campvars.planilha_franquia}}", want: "planilha_franquia", set: true},
		{name: "spaces inside the token", value: " {{ campvars.planilha }} ", want: "planilha", set: true},
		{name: "text around the token", value: "x {{campvars.planilha}}", set: true, invalid: true},
		{name: "another source", value: "{{lead.name}}", set: true, invalid: true},
		{name: "two tokens", value: "{{campvars.a}}{{campvars.b}}", set: true, invalid: true},
		{name: "not text", value: 3, set: true, invalid: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, set, err := SheetStepVariable(sheetStep(c.value))
			if set != c.set || got != c.want || (err != nil) != c.invalid {
				t.Fatalf("SheetStepVariable = %q %v %v", got, set, err)
			}
			if c.invalid && !errors.Is(err, ErrNodeSheetVariableInvalid) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	if SheetVariableToken("planilha") != "{{campvars.planilha}}" {
		t.Fatal("token")
	}
}

func TestASheetStepWritesIntoTheTabPinnedForItsVariable(t *testing.T) {
	state := NewRunState()
	PinSheetFiles(&state, map[string]SheetFile{"planilha": {SpreadsheetID: "sp-centro", Tabs: map[string]string{"model-pag": "centro-pag"}}})
	raw, err := state.JSON()
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := RunStateFromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := SheetStepTarget(sheetStep("{{campvars.planilha}}"), &reloaded); err != nil || got != "centro-pag" {
		t.Fatalf("pinned target = %q, %v", got, err)
	}
	if got, err := SheetStepTarget(sheetStep(nil), &reloaded); err != nil || got != "model-pag" {
		t.Fatalf("fixed target = %q, %v", got, err)
	}
	for name, node := range map[string]*Node{
		"variable not pinned": sheetStep("{{campvars.outra}}"),
		"tab not in the map":  {Type: NodeTypeActionAddSheetRow, Config: map[string]interface{}{SheetStepSheetKey: "model-neg", SheetStepVariableKey: "{{campvars.planilha}}"}},
		"broken variable":     sheetStep("{{lead.name}}"),
	} {
		if _, err := SheetStepTarget(node, &reloaded); !errors.Is(err, ErrSheetVariableUnresolved) {
			t.Fatalf("%s = %v", name, err)
		}
	}
	empty := NewRunState()
	if _, err := SheetStepTarget(sheetStep("{{campvars.planilha}}"), &empty); !errors.Is(err, ErrSheetVariableUnresolved) {
		t.Fatalf("nothing pinned = %v", err)
	}
	files := PinnedSheetFiles(&reloaded)
	shown, _ := json.Marshal(files)
	if files["planilha"].SpreadsheetID != "sp-centro" {
		t.Fatalf("pinned = %s", shown)
	}
}
