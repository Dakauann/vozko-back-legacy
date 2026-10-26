package node_executors

import "vozko/domain/workflow"

type setVariableExecutor struct{}

func NewSetVariableExecutor() workflow.NodeExecutor {
	return &setVariableExecutor{}
}

func (e *setVariableExecutor) Definition() workflow.NodeDefinition {
	return workflow.NodeDefinition{
		Type:         workflow.NodeTypeActionSetVariable,
		TestBehavior: workflow.NodeTestPure,
		Category:     workflow.NodeCategoryLogic,
		Scopes:       []workflow.NodeScope{workflow.NodeScopeShared, workflow.NodeScopeVoice},
		Label:        "Definir Variável",
		Description:  "Define uma variável no estado do fluxo.",
		Icon:         "BracketsCurly",
		Guidance: workflow.NodeGuidance{
			When: "Para gravar/derivar uma variável de fluxo reutilizável adiante.",
			Behavior: "Grava a variável de fluxo (nome em 'variable') com o 'value' já interpolado; fica disponível adiante como {{var.<nome>}}. " +
				"Se o valor cita uma variável que não existe naquele ponto do fluxo, a variável não é gravada e o rastro mostra qual faltou.",
			Examples: []string{
				"config: {\"variable\":\"saudacao\",\"value\":\"Olá {{message}}\"}  // depois use {{var.saudacao}}",
			},
		},
		DefaultConfig: map[string]interface{}{
			"variable": "",
			"value":    "",
		},
		OutputKeys: []workflow.OutputKeyDefinition{
			{Key: "variable", Description: "Nome da variável definida", RepeatsConfig: true},
			{Key: "value", Description: "Valor atribuído", RepeatsConfig: true},
		},
		ConfigSchema: []workflow.ConfigField{
			{Key: "variable", Label: "Nome da Variável", Type: "text", Placeholder: "minha_variavel", Required: true},
			{Key: "value", Label: "Valor", Type: "text", Placeholder: "valor"},
		},
	}
}

func (e *setVariableExecutor) Execute(ctx *workflow.NodeContext) (*workflow.NodeResult, error) {
	varName, _ := ctx.Node.Config["variable"].(string)
	if varName == "" {
		return nil, workflow.ErrNodeConfigMissing
	}
	value, _ := ctx.Node.Config["value"]
	if strVal, ok := value.(string); ok {
		resolved, unresolved := workflow.InterpolateForWrite(strVal, ctx.State)
		if len(unresolved) > 0 {
			ctx.Trace.RecordUnresolved(unresolved...)
			return &workflow.NodeResult{
				Output: map[string]interface{}{"variable": varName, "value": nil, "error": workflow.UnresolvedWriteReason(unresolved)},
			}, nil
		}
		value = resolved
	}
	ctx.State.Set(varName, value)
	return &workflow.NodeResult{
		Output: map[string]interface{}{"variable": varName, "value": value},
	}, nil
}
