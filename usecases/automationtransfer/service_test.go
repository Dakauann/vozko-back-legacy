package automationtransfer_usecase

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vozko/domain/automationneeds"
	"vozko/domain/automationtransfer"
	"vozko/domain/conversation"
	ce "vozko/domain/conversation_event"
	"vozko/domain/workflow"
)

func TestATransferMovesTheStageThenDelegatesAndRecordsTheNote(t *testing.T) {
	h := newHarness()
	h.stages.current = h.stages.byID["st-entrada"]

	done, err := h.service.Transfer(t.Context(), h.request("centro", "É aluno do polo Centro e quer a segunda via."))

	require.NoError(t, err)
	assert.Equal(t, []string{"move:st-centro", "delegate"}, h.order)
	require.Len(t, h.delegations.delegated, 1)
	assert.Equal(t, agentAt("ag-academico"), h.delegations.delegated[0].Automation)
	assert.True(t, h.stages.moves[0].AllowCrossPipeline)
	assert.Equal(t, "ai:ag-prospeccao", h.stages.moves[0].ActorID)
	assert.Equal(t, []string{"entry-1"}, h.broadcasts.stages)
	assert.Equal(t, "Agente Acadêmico · Polo Centro · Atendimento", done.Label)

	require.Len(t, h.timeline.events, 1)
	event := h.timeline.events[0]
	assert.Equal(t, ce.EventAutomationTransferred, event.EventType)
	assert.NotEmpty(t, event.ID)
	assert.Equal(t, "ai:ag-prospeccao", event.ActorID)
	record, ok := automationtransfer.RecordOf(event.DetailsMap())
	require.True(t, ok)
	assert.Equal(t, "Agente Prospecção", record.FromName)
	assert.Equal(t, "Agente Acadêmico", record.ToName)
	assert.Equal(t, "É aluno do polo Centro e quer a segunda via.", record.Note)
	assert.Equal(t, "Polo Centro", record.Stage.PipelineName)
}

func TestATransferToAWorkflowUsesTheSameDelegation(t *testing.T) {
	h := newHarness()

	done, err := h.service.Transfer(t.Context(), h.request("matricula", "Quer se matricular."))

	require.NoError(t, err)
	assert.Equal(t, []string{"delegate"}, h.order)
	assert.Equal(t, workflowAt("wf-matricula"), h.delegations.delegated[0].Automation)
	assert.Equal(t, conversation.AutomationWorkflow, done.Record.To.Kind)
	assert.Empty(t, h.stages.moves)
}

func TestAConversationAlreadyAtTheStageIsNotMovedAgain(t *testing.T) {
	h := newHarness()
	h.stages.current = h.stages.byID["st-centro"]

	_, err := h.service.Transfer(t.Context(), h.request("centro", "Aluno."))

	require.NoError(t, err)
	assert.Equal(t, []string{"delegate"}, h.order)
	assert.Empty(t, h.broadcasts.stages)
}

func TestMissingVariablesLeaveTheConversationWhereItWasAndWarnTheTeam(t *testing.T) {
	h := newHarness()
	h.delegations.checkErr = &automationneeds.Refusal{
		Cause:      automationneeds.ErrConversationMissing,
		Automation: automationneeds.Link{Automation: workflowAt("wf-matricula"), Name: "Matrícula"},
		Needs:      automationneeds.Needs{{Name: "turma"}},
	}

	_, err := h.service.Transfer(t.Context(), h.request("matricula", "Quer se matricular."))

	refusal, unmet := automationneeds.RefusalOf(err)
	require.True(t, unmet)
	assert.Equal(t, []string{"turma"}, refusal.Needs.Names())
	assert.Empty(t, h.order)
	assert.Empty(t, h.timeline.events)
	require.Len(t, h.alerts.refusals, 1)
	assert.Equal(t, "Agente Prospecção", h.alerts.from[0])
}

func TestAFailedDelegationPutsTheConversationBackInItsStage(t *testing.T) {
	h := newHarness()
	h.stages.current = h.stages.byID["st-entrada"]
	h.delegations.delegateErr = errors.New("database unavailable")

	_, err := h.service.Transfer(t.Context(), h.request("centro", "Aluno."))

	require.Error(t, err)
	assert.Equal(t, []string{"move:st-centro", "delegate", "move:st-entrada"}, h.order)
	assert.Equal(t, "st-entrada", h.stages.current.StageID)
	assert.True(t, h.stages.moves[1].AllowCrossPipeline)
	assert.Empty(t, h.timeline.events)
}

func TestAFailedDelegationRemovesAStageTheConversationDidNotHave(t *testing.T) {
	h := newHarness()
	h.delegations.delegateErr = errors.New("database unavailable")

	_, err := h.service.Transfer(t.Context(), h.request("centro", "Aluno."))

	require.Error(t, err)
	assert.Equal(t, []string{"move:st-centro", "delegate", "remove:st-centro"}, h.order)
	assert.Empty(t, h.stages.current.StageID)
}

func TestACompensationThatFailsIsReported(t *testing.T) {
	h := newHarness()
	h.stages.current = h.stages.byID["st-entrada"]
	h.stages.failBack = true
	h.delegations.delegateErr = errors.New("database unavailable")

	_, err := h.service.Transfer(t.Context(), h.request("centro", "Aluno."))

	require.ErrorIs(t, err, ErrStageNotRestored)
}

func TestTheLoopCapCountsTransfersOfEveryKindInTheWindow(t *testing.T) {
	h := newHarness()
	h.pastTransfers(automationtransfer.LoopCap, time.Hour)

	_, err := h.service.Transfer(t.Context(), h.request("matricula", "De novo."))

	require.ErrorIs(t, err, automationtransfer.ErrLoop)
	assert.Empty(t, h.order)
	require.Len(t, h.timeline.asked, 1)
	assert.Equal(t, h.now.Add(-automationtransfer.LoopWindow), *h.timeline.asked[0].Since)
}

func TestTransfersOlderThanTheWindowDoNotCount(t *testing.T) {
	h := newHarness()
	h.pastTransfers(automationtransfer.LoopCap, automationtransfer.LoopWindow+time.Minute)

	_, err := h.service.Transfer(t.Context(), h.request("matricula", "Outro assunto."))

	require.NoError(t, err)
}

func TestAnUnreadableHistoryRefusesTheTransfer(t *testing.T) {
	h := newHarness()
	h.timeline.listErr = errors.New("connection reset")

	_, err := h.service.Transfer(t.Context(), h.request("matricula", "x"))

	require.Error(t, err)
	assert.Empty(t, h.order)
}

func TestOnlyTheAgentGoverningTheConversationMayTransferIt(t *testing.T) {
	cases := map[string]*governance{
		"another agent": {governing: agentAt("ag-academico"), governed: true},
		"a workflow":    {governing: workflowAt("wf-matricula"), governed: true},
		"a person":      {},
		"unreadable":    {err: errors.New("connection reset")},
	}
	for name, g := range cases {
		h := newHarness()
		*h.governance = *g
		_, err := h.service.Transfer(t.Context(), h.request("matricula", "x"))
		if g.err == nil {
			assert.ErrorIs(t, err, automationtransfer.ErrNotGoverning, name)
		} else {
			assert.Error(t, err, name)
		}
		assert.Empty(t, h.order, name)
	}
}

func TestTheModelCanOnlyPickAConfiguredDestinationWithANote(t *testing.T) {
	h := newHarness()

	_, err := h.service.Transfer(t.Context(), h.request("ag-academico", "x"))
	assert.ErrorIs(t, err, automationtransfer.ErrUnknownDestination)

	_, err = h.service.Transfer(t.Context(), h.request("centro", "  "))
	assert.ErrorIs(t, err, automationtransfer.ErrNoteMissing)

	req := h.request("", "x")
	req.EntryID = ""
	_, err = h.service.Transfer(t.Context(), req)
	assert.ErrorIs(t, err, automationtransfer.ErrNoConversation)
	assert.Empty(t, h.order)
}

func TestAStoredDestinationPointingAtTheAgentItselfIsRefused(t *testing.T) {
	h := newHarness()
	req := h.request("me", "x")
	req.Config = destinationsOf(map[string]interface{}{"id": "me", "kind": "agent", "automation_id": "ag-prospeccao"})

	_, err := h.service.Transfer(t.Context(), req)

	assert.ErrorIs(t, err, automationtransfer.ErrSelf)
	assert.Empty(t, h.order)
}

func TestAStageThatIsGoneRefusesBeforeAnythingChanges(t *testing.T) {
	h := newHarness()
	delete(h.stages.byID, "st-centro")

	_, err := h.service.Transfer(t.Context(), h.request("centro", "x"))

	assert.ErrorIs(t, err, automationtransfer.ErrStageInvalid)
	assert.Empty(t, h.order)
	assert.Empty(t, h.delegations.checked)
}

func TestARefusedTargetIsRefusedByTheSharedCheck(t *testing.T) {
	h := newHarness()
	h.delegations.checkErr = errors.Join(conversation.ErrAutomationUnusable, workflow.ErrWorkflowNotActive)

	_, err := h.service.Transfer(t.Context(), h.request("matricula", "x"))

	assert.ErrorIs(t, err, workflow.ErrWorkflowNotActive)
	assert.Empty(t, h.order)
}

func TestATimelineWriteFailureDoesNotUndoADoneTransfer(t *testing.T) {
	h := newHarness()
	h.timeline.saveErr = errors.New("connection reset")

	done, err := h.service.Transfer(t.Context(), h.request("matricula", "x"))

	require.NoError(t, err)
	assert.Equal(t, "Matrícula", done.Label)
	assert.Len(t, h.delegations.delegated, 1)
}

func TestTheModelSeesOnlyDestinationsThatCanTakeTheConversationNow(t *testing.T) {
	h := newHarness()
	config := destinationsOf(
		map[string]interface{}{"id": "centro", "kind": "agent", "automation_id": "ag-academico", "stage_id": "st-centro"},
		map[string]interface{}{"id": "norte", "kind": "agent", "automation_id": "ag-academico", "stage_id": "st-norte", "description": "Alunos do polo Norte."},
		map[string]interface{}{"id": "matricula", "kind": "workflow", "automation_id": "wf-matricula"},
		map[string]interface{}{"id": "off", "kind": "agent", "automation_id": "ag-off"},
		map[string]interface{}{"id": "paused", "kind": "workflow", "automation_id": "wf-paused"},
		map[string]interface{}{"id": "ura", "kind": "workflow", "automation_id": "wf-ura"},
		map[string]interface{}{"id": "gone", "kind": "agent", "automation_id": "ag-academico", "stage_id": "st-gone"},
		map[string]interface{}{"id": "me", "kind": "agent", "automation_id": "ag-prospeccao"},
	)

	offers, err := h.service.Offer("ws-1", "ag-prospeccao", config)

	require.NoError(t, err)
	var ids, labels, descriptions []string
	for _, o := range offers {
		ids = append(ids, o.Destination.ID)
		labels = append(labels, o.Label)
		descriptions = append(descriptions, o.Description)
	}
	assert.Equal(t, []string{"centro", "norte", "matricula"}, ids)
	assert.Equal(t, []string{"Agente Acadêmico · Polo Centro · Atendimento", "Agente Acadêmico · Polo Norte · Atendimento", "Matrícula"}, labels)
	assert.Equal(t, []string{"Atende alunos matriculados.", "Alunos do polo Norte.", "Faz a matrícula de novos alunos."}, descriptions)
}

func TestCheckingTheBindingNamesEachProblem(t *testing.T) {
	cases := map[string]struct {
		item map[string]interface{}
		want error
	}{
		"inactive agent":        {map[string]interface{}{"id": "a", "kind": "agent", "automation_id": "ag-off"}, automationtransfer.ErrTargetUnusable},
		"other workspace":       {map[string]interface{}{"id": "a", "kind": "agent", "automation_id": "ag-other"}, automationtransfer.ErrTargetUnusable},
		"missing agent":         {map[string]interface{}{"id": "a", "kind": "agent", "automation_id": "ag-none"}, automationtransfer.ErrTargetUnusable},
		"unpublished workflow":  {map[string]interface{}{"id": "a", "kind": "workflow", "automation_id": "wf-draft"}, workflow.ErrWorkflowNotPublished},
		"paused workflow":       {map[string]interface{}{"id": "a", "kind": "workflow", "automation_id": "wf-paused"}, workflow.ErrWorkflowNotActive},
		"voice workflow":        {map[string]interface{}{"id": "a", "kind": "workflow", "automation_id": "wf-ura"}, automationtransfer.ErrTargetUnusable},
		"missing workflow":      {map[string]interface{}{"id": "a", "kind": "workflow", "automation_id": "wf-none"}, automationtransfer.ErrTargetUnusable},
		"bad stage":             {map[string]interface{}{"id": "a", "kind": "agent", "automation_id": "ag-academico", "stage_id": "st-gone"}, automationtransfer.ErrStageInvalid},
		"itself":                {map[string]interface{}{"id": "a", "kind": "agent", "automation_id": "ag-prospeccao"}, automationtransfer.ErrSelf},
		"a callable workflow":   {map[string]interface{}{"id": "a", "kind": "workflow", "automation_id": "wf-matricula"}, nil},
		"an agent with a stage": {map[string]interface{}{"id": "a", "kind": "agent", "automation_id": "ag-academico", "stage_id": "st-norte"}, nil},
	}
	for name, c := range cases {
		h := newHarness()
		err := h.service.CheckBinding("ws-1", "ag-prospeccao", destinationsOf(c.item))
		if c.want == nil {
			assert.NoError(t, err, name)
			continue
		}
		assert.ErrorIs(t, err, c.want, name)
		assert.True(t, Problem(err), name)
	}
}

func TestAnUnreadableStoreIsNoBindingProblem(t *testing.T) {
	h := newHarness()
	for _, item := range []map[string]interface{}{
		{"id": "a", "kind": "agent", "automation_id": "broken"},
		{"id": "a", "kind": "workflow", "automation_id": "broken"},
	} {
		err := h.service.CheckBinding("ws-1", "ag-prospeccao", destinationsOf(item))
		require.Error(t, err)
		assert.False(t, Problem(err), "%v", item)
	}
	h.stages.readErr = errors.New("connection reset")
	err := h.service.CheckBinding("ws-1", "ag-prospeccao", destinationsOf(map[string]interface{}{"id": "a", "kind": "agent", "automation_id": "ag-academico", "stage_id": "st-norte"}))
	require.Error(t, err)
	assert.False(t, Problem(err))
}

func TestAnEmptyListIsABindingProblem(t *testing.T) {
	h := newHarness()
	err := h.service.CheckBinding("ws-1", "ag-prospeccao", map[string]interface{}{})
	assert.ErrorIs(t, err, automationtransfer.ErrNoDestinations)
	assert.True(t, Problem(err))
}

func TestTheTargetAgentReadsTheLatestTransferToIt(t *testing.T) {
	h := newHarness()
	_, err := h.service.Transfer(t.Context(), h.request("centro", "É aluno e quer a segunda via."))
	require.NoError(t, err)
	h.timeline.events[0].CreatedAt = h.now.Add(-2 * time.Minute)

	line := h.service.HandOffLine("ws-1", "entry-1", "whatsapp", "ag-academico")

	assert.True(t, strings.Contains(line, "Agente Prospecção") && strings.Contains(line, "segunda via") && strings.Contains(line, "2 min"), line)
	assert.Empty(t, h.service.HandOffLine("ws-1", "entry-1", "whatsapp", "ag-prospeccao"))
	assert.Equal(t, h.now.Add(-automationtransfer.NoteLifetime), *h.timeline.asked[len(h.timeline.asked)-1].Since)
}

func TestAnUnreadableTimelineGivesTheTargetNoLine(t *testing.T) {
	h := newHarness()
	h.timeline.listErr = errors.New("connection reset")
	assert.Empty(t, h.service.HandOffLine("ws-1", "entry-1", "whatsapp", "ag-academico"))
}

func TestABindingProblemNamesTheDestination(t *testing.T) {
	h := newHarness()
	config := destinationsOf(
		map[string]interface{}{"id": "a", "kind": "workflow", "automation_id": "wf-matricula"},
		map[string]interface{}{"id": "b", "kind": "workflow", "automation_id": "wf-paused"},
	)

	err := h.service.CheckBinding("ws-1", "ag-prospeccao", config)

	position, ok := automationtransfer.PositionOf(err)
	assert.True(t, ok && position == 2, "%v", err)
	assert.True(t, Problem(err))
}
