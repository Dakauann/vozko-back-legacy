package siteruntime

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"vozko/domain/siteform"
)

type runtimeContract struct {
	TrapField string `json:"trapField"`
}

func TestTheRuntimeAndTheFormInspectorShareTheTrapFieldName(t *testing.T) {
	raw, err := os.ReadFile("testdata/runtime_contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract runtimeContract
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.TrapField == "" || contract.TrapField != siteform.TrapField {
		t.Fatalf("fixture %q, inspector %q", contract.TrapField, siteform.TrapField)
	}
	runtime, err := Assets.ReadFile(RuntimeFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runtime), `export const TRAP_NAME = "`+siteform.TrapField+`";`) {
		t.Fatal("the runtime must inject the trap field the inspector ignores")
	}
}
