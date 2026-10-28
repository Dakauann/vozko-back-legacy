package workflow

import (
	"reflect"
	"testing"
)

func TestCustomToolsReadTheAgentConfigAndSkipNamelessEntries(t *testing.T) {
	config := map[string]interface{}{"custom_tools": []interface{}{
		map[string]interface{}{"name": "buscar_cep", "description": "Busca o endereço", "parameters": []interface{}{
			map[string]interface{}{"name": "cep", "type": " Integer ", "description": "CEP do cliente", "required": true},
			map[string]interface{}{"name": "turno", "type": "enum", "enum": []interface{}{"manhã", "", "tarde"}},
			map[string]interface{}{"name": "obs"},
			map[string]interface{}{"type": "string"},
		}},
		map[string]interface{}{"description": "sem nome"},
		"lixo",
	}}
	want := []CustomTool{{Name: "buscar_cep", Description: "Busca o endereço", Parameters: []CustomToolParam{
		{Name: "cep", Type: "integer", Description: "CEP do cliente", Required: true},
		{Name: "turno", Type: "enum", Enum: []string{"manhã", "tarde"}},
		{Name: "obs", Type: "string"},
	}}}
	if got := CustomToolsOf(config); !reflect.DeepEqual(got, want) {
		t.Fatalf("tools = %+v, want %+v", got, want)
	}
	if got := CustomToolsOf(map[string]interface{}{}); got != nil {
		t.Fatalf("no tools = %+v", got)
	}
}

func TestToolParameterTypesBecomeVariableTypes(t *testing.T) {
	cases := map[string]VariableType{
		"string": VariableTypeText, "number": VariableTypeNumber, "integer": VariableTypeNumber, "boolean": VariableTypeBoolean,
		"array": VariableTypeList, "object": VariableTypeObject, "date": VariableTypeDate, "time": VariableTypeText,
		"datetime": VariableTypeText, "email": VariableTypeText, "phone": VariableTypeText, "enum": VariableTypeText,
	}
	for raw, want := range cases {
		if got := (CustomToolParam{Type: raw}).VariableType(); got != want {
			t.Errorf("%s = %s, want %s", raw, got, want)
		}
	}
	if got := (CustomToolParam{Type: "desconhecido"}).VariableType(); got != VariableTypeUnknown {
		t.Errorf("an unknown type stays unknown: %s", got)
	}
}
