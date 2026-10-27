package tools_usecase

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sm "vozko/domain/scheduled_message"
	"vozko/domain/tools"
)

type followUpsStub struct {
	scheduled []sm.FollowUpInput
	cancelled []sm.FollowUpCancel
	err       error
	result    *sm.FollowUpScheduled
	cancels   []*sm.ScheduledMessage
}

func (s *followUpsStub) Schedule(_ context.Context, in sm.FollowUpInput) (*sm.FollowUpScheduled, error) {
	s.scheduled = append(s.scheduled, in)
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	at := time.Date(2026, 10, 27, 13, 0, 0, 0, time.UTC)
	return &sm.FollowUpScheduled{
		Message: &sm.ScheduledMessage{ID: "f-1", Kind: sm.KindText, ScheduledAt: at, CancelOnReply: in.CancelOnReply},
		Local:   at.In(loc),
	}, nil
}

func (s *followUpsStub) Cancel(_ context.Context, in sm.FollowUpCancel) ([]*sm.ScheduledMessage, error) {
	s.cancelled = append(s.cancelled, in)
	return s.cancels, s.err
}

func followUpConfig() map[string]interface{} {
	return map[string]interface{}{
		"__workspace_id": "ws-1",
		"__entry_id":     "e-1",
		"__entry_type":   "whatsapp",
		"__agent_id":     "agent-1",
	}
}

func TestScheduleFollowUp_SchedulesForTheAgentInThisConversation(t *testing.T) {
	stub := &followUpsStub{}
	tool := NewScheduleFollowUpTool(stub)
	ctx := tools.WithCallID(context.Background(), "call-9")

	out, err := tool.ExecuteWithConfig(ctx, followUpConfig(), map[string]interface{}{
		"text":            "Oi, conseguiu enviar o comprovante?",
		"days_from_today": float64(1),
		"time":            "10:00",
	})

	require.NoError(t, err)
	require.False(t, out.IsError, out.Result)
	require.Len(t, stub.scheduled, 1)
	in := stub.scheduled[0]
	require.Equal(t, "ws-1", in.WorkspaceID)
	require.Equal(t, "e-1", in.EntryID)
	require.Equal(t, "whatsapp", in.EntryType)
	require.Equal(t, "ai:agent-1", in.Author)
	require.Equal(t, "Oi, conseguiu enviar o comprovante?", in.Text)
	require.Equal(t, 1, *in.When.DaysFromToday)
	require.Equal(t, "10:00", in.When.Time)
	require.True(t, in.CancelOnReply, "a follow-up is cancelled on a reply unless the agent says otherwise")
	require.Equal(t, "follow-up:agent-1:call-9", in.IdempotencyKey)

	result, ok := out.Result.(map[string]interface{})
	require.True(t, ok, "%T", out.Result)
	require.Equal(t, "f-1", result["follow_up_id"])
	require.Equal(t, "27/10/2026 10:00", result["scheduled_for"])
	require.Equal(t, "America/Sao_Paulo", result["time_zone"])
}

func TestScheduleFollowUp_UsesTheConfiguredFallbackTemplate(t *testing.T) {
	stub := &followUpsStub{}
	config := followUpConfig()
	config["fallback_template_id"] = "tpl-1"

	_, err := NewScheduleFollowUpTool(stub).ExecuteWithConfig(context.Background(), config, map[string]interface{}{
		"text":                      "Oi, conseguiu enviar o comprovante?",
		"in_minutes":                float64(240),
		"template_values":           []interface{}{"Ana"},
		"cancel_if_contact_replies": false,
	})

	require.NoError(t, err)
	in := stub.scheduled[0]
	require.Equal(t, 240, in.When.InMinutes)
	require.NotNil(t, in.Fallback)
	require.Equal(t, "tpl-1", in.Fallback.ID)
	require.Equal(t, []string{"Ana"}, in.Fallback.BodyParams)
	require.False(t, in.CancelOnReply)
}

func TestScheduleFollowUp_ARefusalReachesTheModelWithItsCode(t *testing.T) {
	stub := &followUpsStub{err: sm.ErrFollowUpLimitReached}

	out, err := NewScheduleFollowUpTool(stub).ExecuteWithConfig(context.Background(), followUpConfig(), map[string]interface{}{
		"text": "oi", "in_minutes": float64(60),
	})

	require.NoError(t, err)
	require.True(t, out.IsError)
	require.Equal(t, "follow_up_limit_reached", out.ErrorCode)
	require.Contains(t, out.Result, "cancel_follow_up")
}

func TestScheduleFollowUp_OnlyWorksInsideAnAgentsConversation(t *testing.T) {
	stub := &followUpsStub{}
	tool := NewScheduleFollowUpTool(stub)

	out, err := tool.ExecuteWithConfig(context.Background(), map[string]interface{}{"__agent_id": "agent-1"}, map[string]interface{}{"text": "oi", "in_minutes": float64(60)})
	require.NoError(t, err)
	require.True(t, out.IsError)

	config := followUpConfig()
	delete(config, "__agent_id")
	out, err = tool.ExecuteWithConfig(context.Background(), config, map[string]interface{}{"text": "oi", "in_minutes": float64(60)})
	require.NoError(t, err)
	require.True(t, out.IsError)
	require.Empty(t, stub.scheduled)
}

func TestScheduleFollowUp_RejectsAFractionalOrWrongTypedTime(t *testing.T) {
	stub := &followUpsStub{}
	out, err := NewScheduleFollowUpTool(stub).ExecuteWithConfig(context.Background(), followUpConfig(), map[string]interface{}{
		"text": "oi", "in_minutes": "quatro horas",
	})
	require.NoError(t, err)
	require.True(t, out.IsError)
	require.Equal(t, "follow_up_when_invalid", out.ErrorCode)
	require.Empty(t, stub.scheduled)
}

func TestCancelFollowUp_CancelsByIDOrEverythingTheAgentScheduledHere(t *testing.T) {
	stub := &followUpsStub{cancels: []*sm.ScheduledMessage{{ID: "f-1"}, {ID: "f-2"}}}
	tool := NewCancelFollowUpTool(stub)

	out, err := tool.ExecuteWithConfig(context.Background(), followUpConfig(), map[string]interface{}{"follow_up_id": "f-1"})
	require.NoError(t, err)
	require.False(t, out.IsError, out.Result)
	require.Equal(t, sm.FollowUpCancel{WorkspaceID: "ws-1", EntryID: "e-1", EntryType: "whatsapp", Author: "ai:agent-1", ID: "f-1"}, stub.cancelled[0])

	out, err = tool.ExecuteWithConfig(context.Background(), followUpConfig(), map[string]interface{}{"all": true})
	require.NoError(t, err)
	require.False(t, out.IsError, out.Result)
	require.Equal(t, "", stub.cancelled[1].ID)
	result := out.Result.(map[string]interface{})
	require.Equal(t, []string{"f-1", "f-2"}, result["cancelled_ids"])
}

func TestCancelFollowUp_NeedsAnIDOrAll(t *testing.T) {
	stub := &followUpsStub{}
	out, err := NewCancelFollowUpTool(stub).ExecuteWithConfig(context.Background(), followUpConfig(), map[string]interface{}{})
	require.NoError(t, err)
	require.True(t, out.IsError)
	require.Empty(t, stub.cancelled)
}

func TestCancelFollowUp_AnUnknownIDIsCoded(t *testing.T) {
	stub := &followUpsStub{err: sm.ErrNotFound}
	out, err := NewCancelFollowUpTool(stub).ExecuteWithConfig(context.Background(), followUpConfig(), map[string]interface{}{"follow_up_id": "f-9"})
	require.NoError(t, err)
	require.True(t, out.IsError)
	require.Equal(t, "follow_up_not_found", out.ErrorCode)
}

func TestFollowUpTools_AreOneOfAKindMessagingTools(t *testing.T) {
	for _, tool := range []tools.Handler{NewScheduleFollowUpTool(&followUpsStub{}), NewCancelFollowUpTool(&followUpsStub{})} {
		def := tool.Definition()
		require.True(t, def.IsVisibleIn(tools.VisibilityMessaging), def.Name)
		require.False(t, def.IsVisibleIn(tools.VisibilityAnalysis), def.Name)
		require.Equal(t, tools.InstancesSingle, def.Instances.Effective(), def.Name)
	}
	require.Equal(t, "schedule_follow_up", ScheduleFollowUpToolName)
	require.Equal(t, "cancel_follow_up", CancelFollowUpToolName)
}

func TestFollowUpTools_RefuseAnUnwiredService(t *testing.T) {
	require.Panics(t, func() { NewScheduleFollowUpTool(nil) })
	require.Panics(t, func() { NewCancelFollowUpTool(nil) })
}

func TestEveryFollowUpCodeTellsTheModelWhatToDo(t *testing.T) {
	for _, code := range sm.FollowUpErrorCodes() {
		_, ok := followUpAdvices[code]
		require.True(t, ok, "no advice for %s", code)
	}
}
