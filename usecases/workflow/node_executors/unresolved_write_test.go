package node_executors

import (
	"testing"

	"github.com/stretchr/testify/require"

	"vozko/domain/workflow"
	lead_usecase "vozko/usecases/lead"
)

func tracedUpdateLead(t *testing.T, stub *leadProfilesStub, config map[string]interface{}, state map[string]interface{}) (*workflow.NodeResult, *workflow.StepTrace) {
	t.Helper()
	ctx := stageNodeCtx(workflow.NodeTypeActionUpdateLead, config, moveEdges())
	ctx.Trace = &workflow.StepTrace{}
	for key, value := range state {
		ctx.State.Set(key, value)
	}
	result, err := NewUpdateLeadExecutor(stub).Execute(ctx)
	require.NoError(t, err)
	return result, ctx.Trace
}

func TestUpdateLeadNeverWritesANameFromAVariableTheRunNeverHad(t *testing.T) {
	stub := &leadProfilesStub{}
	result, trace := tracedUpdateLead(t, stub, map[string]interface{}{"name": "{{message}}"}, nil)

	require.Empty(t, stub.updates, "nothing may reach the lead when a variable is unresolved")
	require.Equal(t, "fail", result.NextNodeID)
	require.Equal(t, false, result.Output["success"])
	require.Contains(t, result.Output["error"], "message")
	require.Equal(t, []string{"message"}, trace.Unresolved())
}

func TestUpdateLeadNeverWritesAPlaceholderCarriedByAVariable(t *testing.T) {
	stub := &leadProfilesStub{}
	result, trace := tracedUpdateLead(t, stub,
		map[string]interface{}{"name": "{{var.nome}}", "district": "Centro"},
		map[string]interface{}{"nome": "{{message}}"})

	require.Empty(t, stub.updates)
	require.Equal(t, "fail", result.NextNodeID)
	require.Equal(t, []string{"message"}, trace.Unresolved())
}

func TestUpdateLeadRefusesAnUnresolvedCustomField(t *testing.T) {
	stub := &leadProfilesStub{}
	result, trace := tracedUpdateLead(t, stub, map[string]interface{}{
		"name": "Ana", "custom_fields": map[string]interface{}{"interesse": "{{last.interesse}}"},
	}, nil)

	require.Empty(t, stub.updates)
	require.Equal(t, "fail", result.NextNodeID)
	require.Equal(t, []string{"last.interesse"}, trace.Unresolved())
}

func TestUpdateLeadStillWritesTheReplyTheContactSent(t *testing.T) {
	stub := &leadProfilesStub{result: lead_usecase.ProfileResult{LeadID: "l-1", Changed: []string{"name"}}}
	result, trace := tracedUpdateLead(t, stub, map[string]interface{}{"name": "{{message}}"}, map[string]interface{}{"message": " Marcos "})

	require.Equal(t, "ok", result.NextNodeID)
	require.Equal(t, "Marcos", stub.updates[0].Profile.Name)
	require.Empty(t, trace.Unresolved())
}

func TestSetVariableNeverStoresAnUnresolvedPlaceholder(t *testing.T) {
	ctx := stageNodeCtx(workflow.NodeTypeActionSetVariable, map[string]interface{}{"variable": "nome", "value": "{{message}}"}, nil)
	ctx.Trace = &workflow.StepTrace{}

	result, err := NewSetVariableExecutor(nil).Execute(ctx)
	require.NoError(t, err)

	_, stored := ctx.State.Get("nome")
	require.False(t, stored, "the variable must stay unset rather than hold the literal placeholder")
	require.Contains(t, result.Output["error"], "message")
	require.Equal(t, []string{"message"}, ctx.Trace.Unresolved())
}

func TestSetVariableStoresAResolvedValue(t *testing.T) {
	ctx := stageNodeCtx(workflow.NodeTypeActionSetVariable, map[string]interface{}{"variable": "nome", "value": "Sr. {{message}}"}, nil)
	ctx.Trace = &workflow.StepTrace{}
	ctx.State.Set("message", "Paulo")

	result, err := NewSetVariableExecutor(nil).Execute(ctx)
	require.NoError(t, err)

	require.Equal(t, "Sr. Paulo", ctx.State.GetString("nome"))
	require.Equal(t, "Sr. Paulo", result.Output["value"])
	require.Empty(t, ctx.Trace.Unresolved())
}

func TestManageOpportunityNeverTitlesADealWithAnUnresolvedVariable(t *testing.T) {
	desk := &dealDeskStub{}
	ctx := stageNodeCtx(workflow.NodeTypeActionManageOpportunity, map[string]interface{}{
		"pipeline_id": "pl-deals", "action": "create", "title": "Processo de {{message}}",
	}, moveEdges())
	ctx.Trace = &workflow.StepTrace{}

	result, err := NewManageOpportunityExecutor(desk).Execute(ctx)
	require.NoError(t, err)

	require.Empty(t, desk.commands, "no deal may be written with a literal placeholder")
	require.Equal(t, "fail", result.NextNodeID)
	require.Contains(t, result.Output["error"], "message")
	require.Equal(t, []string{"message"}, ctx.Trace.Unresolved())
}
