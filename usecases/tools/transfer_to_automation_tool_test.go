package tools_usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/automationneeds"
	"vozko/domain/automationtransfer"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	"vozko/domain/shared"
	"vozko/domain/tools"
	"vozko/domain/workflow"
	"vozko/usecases/agentctx"
	automationtransfer_usecase "vozko/usecases/automationtransfer"
)

type transfersStub struct {
	offers     []automationtransfer_usecase.Offer
	offerErr   error
	bindingErr error
	done       automationtransfer_usecase.Done
	err        error
	requests   []automationtransfer_usecase.Request
	boundSelf  []string
}

func (s *transfersStub) Offer(string, string, map[string]interface{}) ([]automationtransfer_usecase.Offer, error) {
	return s.offers, s.offerErr
}

func (s *transfersStub) CheckBinding(_ string, selfAgentID string, _ map[string]interface{}) error {
	s.boundSelf = append(s.boundSelf, selfAgentID)
	return s.bindingErr
}

func (s *transfersStub) Transfer(_ context.Context, in automationtransfer_usecase.Request) (automationtransfer_usecase.Done, error) {
	s.requests = append(s.requests, in)
	return s.done, s.err
}

func transferSeed() map[string]interface{} {
	return map[string]interface{}{
		"__workspace_id": "ws-1", "__entry_id": "e-1", "__entry_type": "whatsapp", "__agent_id": "ag-prospeccao",
		automationtransfer.DestinationsKey: []interface{}{},
	}
}

func academicOffers() []automationtransfer_usecase.Offer {
	return []automationtransfer_usecase.Offer{
		{Destination: automationtransfer.Destination{ID: "centro"}, Label: "Agente Acadêmico · Polo Centro · Atendimento", Description: "Alunos do polo Centro."},
		{Destination: automationtransfer.Destination{ID: "matricula"}, Label: "Matrícula"},
	}
}

func TestTheTransferToolIsAMessagingToolWithADestinationList(t *testing.T) {
	def := NewTransferToAutomationTool(&transfersStub{}).Definition()

	assert.Equal(t, "transfer_to_automation", def.Name)
	assert.Equal(t, []tools.ToolVisibility{tools.VisibilityMessaging}, def.Visibility)
	assert.Equal(t, tools.InstancesSingle, def.Instances.Effective())
	assert.ElementsMatch(t, []string{"destination", "note"}, def.Required)
	assert.Equal(t, []string{automationtransfer.DestinationsKey}, def.RequiredConfig)
	assert.Equal(t, "array", def.ConfigSchema[automationtransfer.DestinationsKey].Type)
	assert.False(t, shared.HasLongDash(def.Description+def.DisplayDescription))
}

func TestTheModelChoosesOnlyAmongTheUsableDestinations(t *testing.T) {
	tool := NewTransferToAutomationTool(&transfersStub{offers: academicOffers()}).(tools.ContextualHandler)

	def := tool.DefinitionWithContext(tools.ToolContext{WorkspaceID: "ws-1", Agent: &agent.Agent{ID: "ag-prospeccao"}})

	param := def.Parameters["destination"]
	assert.Equal(t, []string{"centro", "matricula"}, param.Enum)
	assert.Contains(t, param.Description, "centro: Agente Acadêmico · Polo Centro · Atendimento. Alunos do polo Centro.")
	assert.Contains(t, param.Description, "matricula: Matrícula")
}

func TestWithNoUsableDestinationTheToolSaysNotToCallIt(t *testing.T) {
	for name, stub := range map[string]*transfersStub{"none": {}, "unreadable": {offerErr: automationtransfer.ErrConfigUnreadable}} {
		tool := NewTransferToAutomationTool(stub).(tools.ContextualHandler)
		def := tool.DefinitionWithContext(tools.ToolContext{WorkspaceID: "ws-1", Agent: &agent.Agent{ID: "ag-prospeccao"}})
		assert.Contains(t, def.Description, "Não chame", name)
		assert.Empty(t, def.Parameters["destination"].Enum, name)
	}
}

func TestATransferPassesTheChoiceAndTheNoteToTheSharedPath(t *testing.T) {
	stub := &transfersStub{done: automationtransfer_usecase.Done{Label: "Agente Acadêmico · Polo Centro · Atendimento", Record: automationtransfer.Record{To: conversation.Automation{Kind: conversation.AutomationAgent, ID: "ag-academico"}, ToName: "Agente Acadêmico"}}}
	tool := NewTransferToAutomationTool(stub)
	ctx := agentctx.WithAgent(context.Background(), &agent.Agent{ID: "ag-prospeccao", Name: "Agente Prospecção"})

	out, err := tool.ExecuteWithConfig(ctx, transferSeed(), map[string]interface{}{"destination": "centro", "note": "É aluno."})

	require.NoError(t, err)
	assert.False(t, out.IsError, out.Result)
	require.Len(t, stub.requests, 1)
	got := stub.requests[0]
	assert.Equal(t, "ws-1", got.WorkspaceID)
	assert.Equal(t, "e-1", got.EntryID)
	assert.Equal(t, "ag-prospeccao", got.AgentID)
	assert.Equal(t, "Agente Prospecção", got.AgentName)
	assert.Equal(t, "centro", got.DestinationID)
	assert.Equal(t, "É aluno.", got.Note)
	assert.Contains(t, out.Result, "Agente Acadêmico")
	assert.Contains(t, out.Result, "próxima mensagem")
}

func TestAWorkflowTargetTellsTheAgentToStopWriting(t *testing.T) {
	stub := &transfersStub{done: automationtransfer_usecase.Done{Label: "Matrícula", Record: automationtransfer.Record{To: conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "wf"}}}}

	out, err := NewTransferToAutomationTool(stub).ExecuteWithConfig(context.Background(), transferSeed(), map[string]interface{}{"destination": "matricula", "note": "x"})

	require.NoError(t, err)
	assert.Contains(t, out.Result, "Não envie mais mensagens")
}

func TestEveryRefusalReachesTheModelAsASentence(t *testing.T) {
	refusal := &automationneeds.Refusal{Cause: automationneeds.ErrConversationMissing, Automation: automationneeds.Link{Name: "Matrícula"}, Needs: automationneeds.Needs{{Name: "turma"}}}
	cases := map[string]struct {
		err  error
		want string
	}{
		"loop":       {automationtransfer.ErrLoop, "já foi transferida"},
		"unknown":    {automationtransfer.ErrUnknownDestination, "Escolha um dos destinos"},
		"note":       {automationtransfer.ErrNoteMissing, "note"},
		"governing":  {automationtransfer.ErrNotGoverning, "não está mais com você"},
		"variables":  {refusal, "turma"},
		"unusable":   {errors.Join(conversation.ErrAutomationUnusable, workflow.ErrWorkflowNotActive), "indisponível"},
		"target":     {automationtransfer.ErrTargetUnusable, "indisponível"},
		"stage":      {automationtransfer.ErrStageInvalid, "etapa"},
		"self":       {automationtransfer.ErrSelf, "indisponível"},
		"no context": {automationtransfer.ErrNoConversation, "durante um atendimento"},
		"restore":    {automationtransfer_usecase.ErrStageNotRestored, "Não foi possível transferir"},
		"outage":     {errors.New("connection reset"), "Não foi possível transferir"},
		"unreadable": {automationtransfer.ErrConfigUnreadable, "indisponível"},
	}
	for name, c := range cases {
		out, err := NewTransferToAutomationTool(&transfersStub{err: c.err}).ExecuteWithConfig(context.Background(), transferSeed(), map[string]interface{}{"destination": "centro", "note": "x"})
		require.NoError(t, err, name)
		assert.True(t, out.IsError, name)
		assert.Contains(t, out.Result, c.want, name)
		assert.False(t, strings.Contains(out.Result.(string), "connection reset"), name)
	}
}

func TestBindingProblemsCarryTheirCodes(t *testing.T) {
	cases := map[string]struct {
		err  error
		code string
	}{
		"none":        {automationtransfer.ErrNoDestinations, agenttool.CodeTransferNoDestinations},
		"self":        {automationtransfer.At(1, automationtransfer.ErrSelf), agenttool.CodeTransferSelf},
		"duplicate":   {automationtransfer.At(2, automationtransfer.ErrDuplicateDestination), agenttool.CodeTransferDuplicate},
		"unusable":    {automationtransfer.At(1, automationtransfer.ErrTargetUnusable), agenttool.CodeTransferTargetUnusable},
		"unpublished": {automationtransfer.At(1, workflow.ErrWorkflowNotPublished), agenttool.CodeTransferWorkflowDraft},
		"paused":      {automationtransfer.At(1, workflow.ErrWorkflowNotActive), agenttool.CodeTransferWorkflowPaused},
		"stage":       {automationtransfer.At(1, automationtransfer.ErrStageInvalid), agenttool.CodeTransferStageInvalid},
		"unreadable":  {automationtransfer.ErrConfigUnreadable, agenttool.CodeConfigInvalid},
	}
	for name, c := range cases {
		stub := &transfersStub{bindingErr: automationtransfer_usecase.Refused(c.err)}
		validator := NewTransferToAutomationTool(stub).(tools.BindingValidator)
		ctx := agentctx.WithAgent(context.Background(), &agent.Agent{ID: "ag-prospeccao"})
		problem, ok := tools.ProblemOf(validator.ValidateBinding(ctx, "ws-1", nil))
		require.True(t, ok, name)
		assert.Equal(t, c.code, problem.Code, name)
		assert.Equal(t, []string{"ag-prospeccao"}, stub.boundSelf, name)
	}
}

func TestABindingThatCannotBeCheckedIsNoProblem(t *testing.T) {
	validator := NewTransferToAutomationTool(&transfersStub{bindingErr: errors.New("connection reset")}).(tools.BindingValidator)
	err := validator.ValidateBinding(context.Background(), "ws-1", nil)
	_, isProblem := tools.ProblemOf(err)
	assert.Error(t, err)
	assert.False(t, isProblem)
}

func TestSavingTheToolGivesEveryDestinationAnID(t *testing.T) {
	normalizer := NewTransferToAutomationTool(&transfersStub{}).(tools.ConfigNormalizer)
	out, err := normalizer.NormalizeConfig(map[string]interface{}{automationtransfer.DestinationsKey: []interface{}{
		map[string]interface{}{"kind": "agent", "automation_id": "ag-academico"},
	}})
	require.NoError(t, err)
	list, err := automationtransfer.Parse(out)
	require.NoError(t, err)
	assert.NotEmpty(t, list[0].ID)

	_, err = normalizer.NormalizeConfig(map[string]interface{}{automationtransfer.DestinationsKey: "x"})
	assert.ErrorIs(t, err, automationtransfer.ErrConfigUnreadable)
}

func TestTheModelIsToldWhatToInformForEachDestination(t *testing.T) {
	offers := academicOffers()
	offers[1].Informs = []automationtransfer.VariableSource{{Name: "turma", Description: "A turma escolhida"}, {Name: "turno", Description: "Manhã ou noite"}}
	tool := NewTransferToAutomationTool(&transfersStub{offers: offers}).(tools.ContextualHandler)

	def := tool.DefinitionWithContext(tools.ToolContext{WorkspaceID: "ws-1", Agent: &agent.Agent{ID: "ag-prospeccao"}})

	assert.Contains(t, def.Parameters["destination"].Description, "matricula: Matrícula. Informe em variables: turma (A turma escolhida); turno (Manhã ou noite).")
	assert.Equal(t, "object", def.Parameters["variables"].Type)
	assert.NotContains(t, def.Required, "variables")
}

func TestTheValuesTheModelInformsReachTheSharedPath(t *testing.T) {
	stub := &transfersStub{done: automationtransfer_usecase.Done{Label: "Matrícula", Record: automationtransfer.Record{To: conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "wf"}}}}

	_, err := NewTransferToAutomationTool(stub).ExecuteWithConfig(context.Background(), transferSeed(), map[string]interface{}{
		"destination": "matricula", "note": "x", "variables": map[string]interface{}{"turma": "Noite", "ano": float64(2027), "lista": []interface{}{"a"}},
	})

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"turma": "Noite", "ano": "2027"}, stub.requests[0].Provided)
}

func TestAMissingVariableIsNamedSoTheAgentCanAskTheCustomer(t *testing.T) {
	cases := map[string]struct {
		err  error
		want []string
	}{
		"not informed": {&conversationvars.VariablesError{Cause: automationtransfer.ErrModelValueMissing, Names: []string{"turma", "turno"}}, []string{"variables", "turma, turno", "pergunte ao cliente"}},
		"came empty":   {&conversationvars.VariablesError{Cause: conversationvars.ErrValueEmpty, Names: []string{"curso"}}, []string{"curso", "não tem valor"}},
		"too long":     {&conversationvars.VariablesError{Cause: conversationvars.ErrValueInvalid, Names: []string{"obs"}}, []string{"obs", "2000"}},
	}
	for name, c := range cases {
		out, err := NewTransferToAutomationTool(&transfersStub{err: c.err}).ExecuteWithConfig(context.Background(), transferSeed(), map[string]interface{}{"destination": "matricula", "note": "x"})
		require.NoError(t, err, name)
		assert.True(t, out.IsError, name)
		for _, want := range c.want {
			assert.Contains(t, out.Result, want, name)
		}
	}
}

func TestMappingProblemsCarryTheirCodesAndNames(t *testing.T) {
	cases := map[string]struct {
		cause error
		code  string
		says  string
	}{
		"new need":   {automationtransfer.ErrVariableUnmapped, agenttool.CodeTransferVariableUnmapped, "o destino agora precisa de curso"},
		"not needed": {automationtransfer.ErrVariableNotNeeded, agenttool.CodeTransferVariableNotNeeded, "o destino não precisa mais de curso"},
		"source":     {automationtransfer.ErrVariableSource, agenttool.CodeTransferVariableSource, "a origem de curso não é válida"},
	}
	for name, c := range cases {
		err := automationtransfer_usecase.Refused(automationtransfer.At(2, &conversationvars.VariablesError{Cause: c.cause, Names: []string{"curso"}}))
		validator := NewTransferToAutomationTool(&transfersStub{bindingErr: err}).(tools.BindingValidator)
		problem, ok := tools.ProblemOf(validator.ValidateBinding(context.Background(), "ws-1", nil))
		require.True(t, ok, name)
		assert.Equal(t, c.code, problem.Code, name)
		assert.Equal(t, "destino 2: "+c.says, problem.Message, name)
	}
}
