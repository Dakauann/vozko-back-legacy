package workflow

import (
	"errors"
	"reflect"
	"testing"
)

func scheduleInputs() map[string]interface{} {
	return map[string]interface{}{AgentCallInputsKey: []interface{}{
		map[string]interface{}{"name": "quantidade", "type": "number", "required": true},
		map[string]interface{}{"name": "unidade", "type": "string", "enum": []interface{}{"minutes", "hours", "days"}, "required": true},
		map[string]interface{}{"name": "horario", "type": "string", "format": "time"},
	}}
}

func TestAgentCallInputsCarryTheirChoicesAndTimeFormat(t *testing.T) {
	inputs, err := AgentCallInputsOf(scheduleInputs())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inputs[1].Enum, []string{"minutes", "hours", "days"}) || inputs[2].Format != AgentCallFormatTime {
		t.Fatalf("inputs = %+v", inputs)
	}
	bad := []map[string]interface{}{
		{"name": "x", "type": "number", "enum": []interface{}{"a"}},
		{"name": "x", "type": "string", "format": "color"},
		{"name": "x", "type": "boolean", "format": "time"},
		{"name": "x", "type": "string", "enum": []interface{}{"a", "a"}},
	}
	for _, field := range bad {
		if _, err := AgentCallInputsOf(map[string]interface{}{AgentCallInputsKey: []interface{}{field}}); !errors.Is(err, ErrAgentCallInputsInvalid) {
			t.Errorf("%v: err = %v", field, err)
		}
	}
}

func TestAgentCallValuesMustBeOneOfTheChoicesAndAValidTime(t *testing.T) {
	inputs, _ := AgentCallInputsOf(scheduleInputs())
	cleaned, missing, err := ValidateAgentCallInputs(inputs, map[string]interface{}{"quantidade": float64(4), "unidade": "minutes", "horario": "17:00"})
	if err != nil || len(missing) != 0 || cleaned["unidade"] != "minutes" {
		t.Fatalf("cleaned = %v missing = %v err = %v", cleaned, missing, err)
	}
	for _, values := range []map[string]interface{}{
		{"quantidade": float64(4), "unidade": "weeks"},
		{"quantidade": float64(4), "unidade": "hours", "horario": "25:00"},
		{"quantidade": "4", "unidade": "hours"},
	} {
		if _, _, err := ValidateAgentCallInputs(inputs, values); !errors.Is(err, ErrAgentCallInputType) {
			t.Errorf("%v: err = %v", values, err)
		}
	}
}
