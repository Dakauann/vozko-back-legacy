package workflow

import "testing"

func scheduleGraph(config map[string]interface{}) *Graph {
	return &Graph{Nodes: []Node{{ID: "s", Type: NodeTypeActionScheduleMessage, Config: config}}}
}

func TestLintRefusesLiteralSchedulingValuesWithTheRuntimeRule(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]interface{}
		field  string
	}{
		{"fine literal", map[string]interface{}{"when_mode": "in", "delay_amount": float64(4), "delay_unit": "hours"}, ""},
		{"fine variables", map[string]interface{}{"when_mode": "in", "delay_amount": "{{var.qtd}}", "delay_unit": "{{var.unidade}}"}, ""},
		{"zero amount", map[string]interface{}{"when_mode": "in", "delay_amount": float64(0), "delay_unit": "hours"}, "delay_amount"},
		{"bad unit", map[string]interface{}{"when_mode": "in", "delay_amount": float64(4), "delay_unit": "weeks"}, "delay_unit"},
		{"bad time", map[string]interface{}{"when_mode": "day", "days_from_today": float64(1), "time": "25:00"}, "time"},
		{"bad date", map[string]interface{}{"when_mode": "date", "date": "2026-02-30", "time": "{{var.hora}}"}, "date"},
		{"bad mode", map[string]interface{}{"when_mode": "logo"}, "when_mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := ScheduleIssues(scheduleGraph(tc.config))
			if tc.field == "" {
				if len(issues) != 0 {
					t.Fatalf("issues = %+v", issues)
				}
				return
			}
			if len(issues) != 1 || issues[0].Code != LintScheduleInvalid || issues[0].Field != tc.field || issues[0].Severity != SeverityBlocking || issues[0].Values["label"] == "" {
				t.Fatalf("issues = %+v", issues)
			}
		})
	}
}
