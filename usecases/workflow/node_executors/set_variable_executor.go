package node_executors

import (
	"context"

	"vozko/domain/workflow"
)

type ValueRules interface {
	Rejected(ctx context.Context, workspaceID string, values map[string]string) ([]string, error)
}

type setVariableExecutor struct {
	rules ValueRules
}

func NewSetVariableExecutor(rules ValueRules) workflow.NodeExecutor {
	return &setVariableExecutor{rules: rules}
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
			{Key: "variable_label", Label: "Nome amigável", Type: "text", Placeholder: "Saudação", Description: "Como a variável aparece nas listas e nos campos."},
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
	if refused := e.checkConversationValue(ctx, varName, value); refused != "" {
		return &workflow.NodeResult{
			Output: map[string]interface{}{"variable": varName, "value": nil, "error": refused, "error_code": refused},
		}, nil
	}
	ctx.State.Set(varName, value)
	return &workflow.NodeResult{
		Output: map[string]interface{}{"variable": varName, "value": value},
	}, nil
}

const (
	variableValueInvalid     = "variable_value_invalid"
	variableRulesUnavailable = "variable_rules_unavailable"
)

func (e *setVariableExecutor) checkConversationValue(ctx *workflow.NodeContext, varName string, value interface{}) string {
	name, ok := workflow.CampaignVarOf(varName)
	if !ok {
		return ""
	}
	if e.rules == nil || ctx.Run == nil {
		return variableRulesUnavailable
	}
	rejected, err := e.rules.Rejected(context.Background(), ctx.Run.WorkspaceID, map[string]string{name: workflow.TemplateText(value)})
	switch {
	case err != nil:
		return variableRulesUnavailable
	case len(rejected) > 0:
		return variableValueInvalid
	}
	return ""
}
