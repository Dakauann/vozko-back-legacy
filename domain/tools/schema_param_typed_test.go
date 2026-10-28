package tools

import (
	"strings"
	"testing"
)

func TestTheSchemaSaysTheChoicesAndTheTimeFormatWithoutNewParameters(t *testing.T) {
	param, section, ok := SchemaObjectParameter("Campos", "Campos do fluxo", []interface{}{
		map[string]interface{}{"name": "quantidade", "type": "number", "required": true},
		map[string]interface{}{"name": "unidade", "type": "string", "enum": []interface{}{"minutes", "hours", "days"}},
		map[string]interface{}{"name": "horario", "type": "string", "format": "time"},
	})
	if !ok || param.Type != "object" {
		t.Fatalf("param = %+v", param)
	}
	for _, want := range []string{"quantidade: number (required)", "unidade: string (one of minutes, hours, days)", "horario: string (HH:MM)"} {
		if !strings.Contains(param.Description, want) {
			t.Fatalf("description %q misses %q", param.Description, want)
		}
	}
	if !strings.Contains(section, "unidade (string, one of minutes, hours, days)") || !strings.Contains(section, "horario (string, HH:MM)") {
		t.Fatalf("section = %q", section)
	}
}
