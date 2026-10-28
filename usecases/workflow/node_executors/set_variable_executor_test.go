package node_executors

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"vozko/domain/workflow"
)

type valueRulesStub struct {
	rejected []string
	err      error
	asked    map[string]string
}

func (v *valueRulesStub) Rejected(_ context.Context, _ string, values map[string]string) ([]string, error) {
	v.asked = values
	return v.rejected, v.err
}

func TestSetVariableOnConversationDataChecksTheDeclaredType(t *testing.T) {
	rules := &valueRulesStub{rejected: []string{"valor"}}
	ctx := stageNodeCtx(workflow.NodeTypeActionSetVariable, map[string]interface{}{"variable": "campvars.valor", "value": "dez"}, nil)
	ctx.Trace = &workflow.StepTrace{}
	result, err := NewSetVariableExecutor(rules).Execute(ctx)
	require.NoError(t, err)
	_, stored := ctx.State.Get("campvars.valor")
	require.False(t, stored)
	require.Equal(t, "variable_value_invalid", result.Output["error_code"])
	require.Equal(t, map[string]string{"valor": "dez"}, rules.asked)

	rules.rejected = nil
	ctx = stageNodeCtx(workflow.NodeTypeActionSetVariable, map[string]interface{}{"variable": "campvars.valor", "value": "10"}, nil)
	ctx.Trace = &workflow.StepTrace{}
	_, err = NewSetVariableExecutor(rules).Execute(ctx)
	require.NoError(t, err)
	require.Equal(t, "10", ctx.State.GetString("campvars.valor"))
}

func TestSetVariableOnConversationDataFailsClosedWithoutTheRules(t *testing.T) {
	for name, rules := range map[string]ValueRules{"unwired": nil, "unreadable": &valueRulesStub{err: errors.New("down")}} {
		ctx := stageNodeCtx(workflow.NodeTypeActionSetVariable, map[string]interface{}{"variable": "campvars.valor", "value": "10"}, nil)
		ctx.Trace = &workflow.StepTrace{}
		result, err := NewSetVariableExecutor(rules).Execute(ctx)
		require.NoError(t, err, name)
		_, stored := ctx.State.Get("campvars.valor")
		require.False(t, stored, name)
		require.Equal(t, "variable_rules_unavailable", result.Output["error_code"], name)
	}
}

func TestSetVariableOnARunVariableNeedsNoRules(t *testing.T) {
	ctx := stageNodeCtx(workflow.NodeTypeActionSetVariable, map[string]interface{}{"variable": "saudacao", "value": "Oi"}, nil)
	ctx.Trace = &workflow.StepTrace{}
	_, err := NewSetVariableExecutor(nil).Execute(ctx)
	require.NoError(t, err)
	require.Equal(t, "Oi", ctx.State.GetString("saudacao"))
}
