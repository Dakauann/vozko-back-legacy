package agenttool

import (
	"context"
	"testing"

	"vozko/domain/tools"
)

type plainHandler struct{ name string }

func (h plainHandler) Definition() tools.Definition { return tools.Definition{Name: h.name} }

func (h plainHandler) Execute(context.Context, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, nil
}

func (h plainHandler) ExecuteWithConfig(context.Context, map[string]interface{}, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, nil
}

type previewingHandler struct{ plainHandler }

func (previewingHandler) Preview(context.Context, map[string]interface{}, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, nil
}

type handlerCatalog struct {
	tools.Service
	handlers map[string]tools.Handler
}

func (c handlerCatalog) Handler(name string) (tools.Handler, bool) {
	h, ok := c.handlers[name]
	return h, ok
}

func TestEachToolTypeHasOneTestMode(t *testing.T) {
	cases := []struct {
		name     string
		typeName string
		handler  tools.Handler
		want     TestMode
	}{
		{"an HTTP request calls the real API", "http_request", plainHandler{"http_request"}, TestModeRun},
		{"the type name is matched without case", "HTTP_Request", plainHandler{"http_request"}, TestModeRun},
		{"a handler that previews shows a preview", "add_sheet_row", previewingHandler{plainHandler{"add_sheet_row"}}, TestModePreview},
		{"a handler without preview goes to a scenario", "transfer_to_human", plainHandler{"transfer_to_human"}, TestModeScenario},
		{"a missing handler fails closed to a scenario", "http_request", nil, TestModeScenario},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TestModeOf(c.typeName, c.handler); got != c.want {
				t.Fatalf("TestModeOf(%q) = %q, want %q", c.typeName, got, c.want)
			}
		})
	}
}

func TestTheRegistryDecidesTheTestModeOfATypeName(t *testing.T) {
	catalog := handlerCatalog{handlers: map[string]tools.Handler{
		"http_request":      plainHandler{"http_request"},
		"add_sheet_row":     previewingHandler{plainHandler{"add_sheet_row"}},
		"transfer_to_human": plainHandler{"transfer_to_human"},
	}}
	want := map[string]TestMode{
		"http_request":      TestModeRun,
		" Add_Sheet_Row ":   TestModePreview,
		"transfer_to_human": TestModeScenario,
		"unknown_type":      TestModeScenario,
	}
	for typeName, mode := range want {
		if got := TestModeIn(catalog, typeName); got != mode {
			t.Fatalf("TestModeIn(%q) = %q, want %q", typeName, got, mode)
		}
	}
	if got := TestModeIn(nil, "http_request"); got != TestModeScenario {
		t.Fatalf("no registry = %q", got)
	}
}
