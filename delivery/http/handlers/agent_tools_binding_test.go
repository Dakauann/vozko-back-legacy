package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/domain/tools"
)

type catalogOnlyTools struct {
	tools.Service
	defs     []tools.Definition
	handlers map[string]tools.Handler
}

func (c catalogOnlyTools) Definitions() []tools.Definition { return c.defs }

func (c catalogOnlyTools) Handler(name string) (tools.Handler, bool) {
	h, ok := c.handlers[name]
	return h, ok
}

func TestTheToolCatalogTellsWhichConfigKeyMakesEachBindingItsOwnTool(t *testing.T) {
	h := &AgentHandler{toolRegistry: catalogOnlyTools{defs: []tools.Definition{
		{Name: "run_workflow", Instances: tools.PerTarget("workflow_id")},
		{Name: "http_request"},
	}}}
	rec := httptest.NewRecorder()
	h.ListTools(rec, httptest.NewRequest(http.MethodGet, "/agents/tools", nil))

	var body []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	byName := map[string]map[string]interface{}{}
	for _, tool := range body {
		byName[tool["name"].(string)] = tool
	}
	if byName["run_workflow"]["bindingKey"] != "workflow_id" {
		t.Fatalf("run_workflow = %+v", byName["run_workflow"])
	}
	if _, present := byName["http_request"]["bindingKey"]; present {
		t.Fatalf("a tool bound once per agent has no binding key: %+v", byName["http_request"])
	}
}

func TestTheToolCatalogTellsHowManyInstancesEachTypeAllows(t *testing.T) {
	h := &AgentHandler{toolRegistry: catalogOnlyTools{defs: []tools.Definition{
		{Name: "run_workflow", Instances: tools.PerTarget("workflow_id")},
		{Name: "http_request", Instances: tools.Many()},
		{Name: "finish_conversation"},
	}}}
	rec := httptest.NewRecorder()
	h.ListTools(rec, httptest.NewRequest(http.MethodGet, "/agents/tools", nil))
	var body []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := map[string]interface{}{}
	for _, tool := range body {
		got[tool["name"].(string)] = tool["instances"]
	}
	if got["run_workflow"] != "per_target" || got["http_request"] != "many" || got["finish_conversation"] != "single" {
		t.Fatalf("instances %v", got)
	}
}

type catalogHandler struct{ name string }

func (c catalogHandler) Definition() tools.Definition { return tools.Definition{Name: c.name} }

func (c catalogHandler) Execute(context.Context, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, nil
}

func (c catalogHandler) ExecuteWithConfig(context.Context, map[string]interface{}, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, nil
}

type previewCatalogHandler struct{ catalogHandler }

func (previewCatalogHandler) Preview(context.Context, map[string]interface{}, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, nil
}

func TestTheToolCatalogTellsHowEachTypeIsTested(t *testing.T) {
	h := &AgentHandler{toolRegistry: catalogOnlyTools{
		defs: []tools.Definition{{Name: "http_request"}, {Name: "add_sheet_row"}, {Name: "transfer_to_human"}, {Name: "orphan_type"}},
		handlers: map[string]tools.Handler{
			"http_request":      catalogHandler{"http_request"},
			"add_sheet_row":     previewCatalogHandler{catalogHandler{"add_sheet_row"}},
			"transfer_to_human": catalogHandler{"transfer_to_human"},
		},
	}}
	rec := httptest.NewRecorder()
	h.ListTools(rec, httptest.NewRequest(http.MethodGet, "/agents/tools", nil))
	var body []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := map[string]interface{}{}
	for _, tool := range body {
		got[tool["name"].(string)] = tool["testMode"]
	}
	if got["http_request"] != "run" || got["add_sheet_row"] != "preview" || got["transfer_to_human"] != "scenario" || got["orphan_type"] != "scenario" {
		t.Fatalf("test modes %v", got)
	}
}
