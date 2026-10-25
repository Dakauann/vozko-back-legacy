package tools_usecase

import (
	"context"
	"fmt"
	"log"
	"strings"

	"vozko/domain/calltrace"
	"vozko/domain/httpcall"
	"vozko/domain/tools"
)

func SimulationRunsForReal(name string, config map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case SearchKnowledgeBaseToolName, CheckCalendarAvailabilityToolName, ValidateCEPToolName:
		return true
	case httpRequestToolName:
		method, _ := config["method"].(string)
		return httpcall.ReadOnlyMethod(method)
	}
	return false
}

type SimulatedToolService struct {
	real tools.Service
}

func NewSimulatedToolService(real tools.Service) *SimulatedToolService {
	return &SimulatedToolService{real: real}
}

func (s *SimulatedToolService) Definitions() []tools.Definition {
	if s.real == nil {
		return nil
	}
	return s.real.Definitions()
}

func (s *SimulatedToolService) DefinitionsFor(v tools.ToolVisibility) []tools.Definition {
	if s.real == nil {
		return nil
	}
	return s.real.DefinitionsFor(v)
}

func (s *SimulatedToolService) Handler(name string) (tools.Handler, bool) {
	if s.real == nil {
		return nil, false
	}
	return s.real.Handler(name)
}

func (s *SimulatedToolService) Execute(ctx context.Context, name string, params map[string]interface{}) (tools.ExecutionResult, error) {
	if mock, ok := tools.MockFrom(ctx); ok {
		return mocked(mock), nil
	}
	if s.real == nil || !SimulationRunsForReal(name, nil) {
		return s.stub(name), nil
	}
	return s.real.Execute(ctx, name, params)
}

func (s *SimulatedToolService) ExecuteWithConfig(ctx context.Context, name string, config, params map[string]interface{}) (tools.ExecutionResult, error) {
	if mock, ok := tools.MockFrom(ctx); ok {
		return mocked(mock), nil
	}
	if s.real == nil {
		return s.stub(name), nil
	}
	if !SimulationRunsForReal(name, config) {
		if handler, ok := s.real.Handler(name); ok {
			if previewer, ok := handler.(tools.Previewer); ok {
				res, err := previewer.Preview(ctx, config, params)
				res.Simulated = true
				return res, err
			}
		}
		return s.stub(name), nil
	}
	return s.real.ExecuteWithConfig(ctx, name, config, params)
}

func mocked(mock tools.Mock) tools.ExecutionResult {
	return tools.ExecutionResult{
		Result:    mock.Result,
		IsError:   mock.IsError,
		Simulated: true,
		Mocked:    true,
		Trace:     &calltrace.ToolTrace{MappedOutput: mock.Result},
	}
}

func (s *SimulatedToolService) stub(name string) tools.ExecutionResult {
	log.Printf("[agent-simulator] intercepted tool call %q (no side effects)", name)
	return tools.ExecutionResult{
		Result: fmt.Sprintf(
			"(SIMULAÇÃO) A ferramenta %q teria sido executada com os parâmetros informados; nenhuma ação real foi realizada. Considere a ação concluída com sucesso e continue a conversa normalmente.",
			name,
		),
		Simulated: true,
	}
}

var _ tools.Service = (*SimulatedToolService)(nil)
