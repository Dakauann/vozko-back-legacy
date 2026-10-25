package agenttool

import (
	"strings"

	"vozko/domain/tools"
)

type TestMode string

const (
	TestModeRun      TestMode = "run"
	TestModePreview  TestMode = "preview"
	TestModeScenario TestMode = "scenario"
)

func TestModeOf(typeName string, handler tools.Handler) TestMode {
	if handler == nil {
		return TestModeScenario
	}
	if strings.EqualFold(strings.TrimSpace(typeName), httpToolType) {
		return TestModeRun
	}
	if _, ok := handler.(tools.Previewer); ok {
		return TestModePreview
	}
	return TestModeScenario
}

func TestModeIn(registry tools.Service, typeName string) TestMode {
	if registry == nil {
		return TestModeScenario
	}
	handler, ok := registry.Handler(strings.ToLower(strings.TrimSpace(typeName)))
	if !ok {
		return TestModeScenario
	}
	return TestModeOf(typeName, handler)
}
