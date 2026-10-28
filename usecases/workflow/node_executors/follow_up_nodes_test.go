package node_executors

import (
	"context"
	"errors"
	"testing"
	"time"

	sm "vozko/domain/scheduled_message"
	"vozko/domain/workflow"
)

type followUpBook struct {
	scheduled []sm.FollowUpInput
	cancelled []sm.FollowUpCancel
	err       error
	cancels   []*sm.ScheduledMessage
}

func (b *followUpBook) Schedule(_ context.Context, in sm.FollowUpInput) (*sm.FollowUpScheduled, error) {
	b.scheduled = append(b.scheduled, in)
	if b.err != nil {
		return nil, b.err
	}
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	at := time.Date(2026, 10, 27, 13, 0, 0, 0, time.UTC)
	return &sm.FollowUpScheduled{Message: &sm.ScheduledMessage{ID: "f-1", Kind: sm.KindText, ScheduledAt: at}, Local: at.In(loc)}, nil
}

func (b *followUpBook) Cancel(_ context.Context, in sm.FollowUpCancel) ([]*sm.ScheduledMessage, error) {
	b.cancelled = append(b.cancelled, in)
	return b.cancels, b.err
}

func followUpNodeContext(nodeType workflow.NodeType, config map[string]interface{}) *workflow.NodeContext {
	state := workflow.NewRunState()
	state.Set("nome", "Ana")
	state.Set("dia", "2026-10-30")
	state.Set("lembrete_id", "f-7")
	node := &workflow.Node{ID: "n1", Type: nodeType, Config: config}
	graph := &workflow.Graph{
		Nodes: []workflow.Node{*node, {ID: "ok"}, {ID: "bad"}},
		Edges: []workflow.Edge{{Source: "n1", Target: "ok", Label: "sucesso"}, {Source: "n1", Target: "bad", Label: "erro"}},
	}
	run := &workflow.WorkflowRun{ID: "run-1", WorkflowID: "wf-1", WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: "whatsapp", CurrentNodeID: "n1", State: state}
	return &workflow.NodeContext{Run: run, Node: node, Graph: graph, Workflow: &workflow.Workflow{ID: "wf-1", WorkspaceID: "ws-1"}, State: &run.State}
}

func TestScheduleMessageNodeSchedulesAsTheWorkflow(t *testing.T) {
	book := &followUpBook{}
	result, err := NewScheduleMessageExecutor(book).Execute(followUpNodeContext(workflow.NodeTypeActionScheduleMessage, map[string]interface{}{
		"text": "Oi {{var.nome}}, conseguiu enviar o comprovante?", "when_mode": "in", "delay_amount": float64(4), "delay_unit": "hours",
		"cancel_on_reply": true,
	}))
	if err != nil || result.NextNodeID != "ok" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	in := book.scheduled[0]
	if in.Author != "workflow:wf-1" || in.EntryID != "entry-1" || in.EntryType != "whatsapp" || in.WorkspaceID != "ws-1" {
		t.Fatalf("input = %+v", in)
	}
	if in.Text != "Oi Ana, conseguiu enviar o comprovante?" || in.When.InMinutes != 240 || !in.CancelOnReply {
		t.Fatalf("input = %+v", in)
	}
	if in.IdempotencyKey != "follow-up:run-1:n1#0" {
		t.Fatalf("key = %q", in.IdempotencyKey)
	}
	if result.Output["follow_up_id"] != "f-1" || result.Output["scheduled_for"] != "27/10/2026 10:00" {
		t.Fatalf("output = %+v", result.Output)
	}
}

func TestScheduleMessageNodeReadsADayAndAnHour(t *testing.T) {
	book := &followUpBook{}
	_, err := NewScheduleMessageExecutor(book).Execute(followUpNodeContext(workflow.NodeTypeActionScheduleMessage, map[string]interface{}{
		"text": "oi", "when_mode": "date", "date": "{{var.dia}}", "time": "09:30",
		"fallback_template_id": "tpl-1", "fallback_params": []interface{}{"{{var.nome}}"},
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	in := book.scheduled[0]
	if in.When.Date != "2026-10-30" || in.When.Time != "09:30" || in.When.InMinutes != 0 {
		t.Fatalf("when = %+v", in.When)
	}
	if in.Fallback == nil || in.Fallback.ID != "tpl-1" || len(in.Fallback.BodyParams) != 1 || in.Fallback.BodyParams[0] != "Ana" {
		t.Fatalf("fallback = %+v", in.Fallback)
	}
	if !in.CancelOnReply {
		t.Fatal("cancel on reply is on unless the node turns it off")
	}

	tomorrow := &followUpBook{}
	if _, err := NewScheduleMessageExecutor(tomorrow).Execute(followUpNodeContext(workflow.NodeTypeActionScheduleMessage, map[string]interface{}{
		"text": "oi", "when_mode": "day", "days_from_today": float64(1), "time": "10:00", "cancel_on_reply": false,
	})); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := tomorrow.scheduled[0]; got.When.DaysFromToday == nil || *got.When.DaysFromToday != 1 || got.When.Time != "10:00" || got.CancelOnReply {
		t.Fatalf("input = %+v", got)
	}
}

func TestScheduleMessageNodeRoutesARefusalToErro(t *testing.T) {
	book := &followUpBook{err: sm.ErrFollowUpLimitReached}
	result, err := NewScheduleMessageExecutor(book).Execute(followUpNodeContext(workflow.NodeTypeActionScheduleMessage, map[string]interface{}{
		"text": "oi", "when_mode": "in", "delay_amount": float64(30), "delay_unit": "minutes",
	}))
	if err != nil || result.NextNodeID != "bad" || result.Output["error_code"] != "follow_up_limit_reached" {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestScheduleMessageNodeNeverSchedulesAnUnresolvedDate(t *testing.T) {
	book := &followUpBook{}
	result, err := NewScheduleMessageExecutor(book).Execute(followUpNodeContext(workflow.NodeTypeActionScheduleMessage, map[string]interface{}{
		"text": "oi", "when_mode": "date", "date": "{{var.sem_valor}}", "time": "10:00",
	}))
	if err != nil || result.NextNodeID != "bad" || result.Output["error_code"] != "follow_up_variable_missing" || len(book.scheduled) != 0 {
		t.Fatalf("result = %+v, %v, scheduled %d", result, err, len(book.scheduled))
	}
}

func TestScheduleMessageNodeRetriesAnOutage(t *testing.T) {
	book := &followUpBook{err: errors.New("db down")}
	if _, err := NewScheduleMessageExecutor(book).Execute(followUpNodeContext(workflow.NodeTypeActionScheduleMessage, map[string]interface{}{
		"text": "oi", "when_mode": "in", "delay_amount": float64(30), "delay_unit": "minutes",
	})); err == nil {
		t.Fatal("an outage must be retried by the engine with the same key")
	}
}

func TestScheduleMessageNodeNeedsAConversation(t *testing.T) {
	book := &followUpBook{}
	ctx := followUpNodeContext(workflow.NodeTypeActionScheduleMessage, map[string]interface{}{"text": "oi", "when_mode": "in", "delay_amount": float64(30)})
	ctx.Run.EntryType = workflow.EntryTypeLead
	result, err := NewScheduleMessageExecutor(book).Execute(ctx)
	if err != nil || result.NextNodeID != "bad" || result.Output["error_code"] != "follow_up_needs_conversation" || len(book.scheduled) != 0 {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestCancelScheduledMessagesNodeCancelsByIDOrEverythingTheWorkflowScheduled(t *testing.T) {
	book := &followUpBook{cancels: []*sm.ScheduledMessage{{ID: "f-7"}}}
	result, err := NewCancelScheduledMessagesExecutor(book).Execute(followUpNodeContext(workflow.NodeTypeActionCancelScheduledMessages, map[string]interface{}{
		"scope": "id", "follow_up_id": "{{var.lembrete_id}}",
	}))
	if err != nil || result.NextNodeID != "ok" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if got := book.cancelled[0]; got.ID != "f-7" || got.Author != "workflow:wf-1" || got.EntryID != "entry-1" {
		t.Fatalf("cancel = %+v", got)
	}
	if result.Output["cancelled"] != 1 {
		t.Fatalf("output = %+v", result.Output)
	}

	all := &followUpBook{}
	if _, err := NewCancelScheduledMessagesExecutor(all).Execute(followUpNodeContext(workflow.NodeTypeActionCancelScheduledMessages, map[string]interface{}{"scope": "mine"})); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if all.cancelled[0].ID != "" {
		t.Fatalf("cancel = %+v", all.cancelled[0])
	}
}

func TestCancelScheduledMessagesNodeRoutesAnUnknownIDToErro(t *testing.T) {
	book := &followUpBook{err: sm.ErrNotFound}
	result, err := NewCancelScheduledMessagesExecutor(book).Execute(followUpNodeContext(workflow.NodeTypeActionCancelScheduledMessages, map[string]interface{}{"scope": "id", "follow_up_id": "f-9"}))
	if err != nil || result.NextNodeID != "bad" || result.Output["error_code"] != "follow_up_not_found" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	empty := &followUpBook{}
	result, err = NewCancelScheduledMessagesExecutor(empty).Execute(followUpNodeContext(workflow.NodeTypeActionCancelScheduledMessages, map[string]interface{}{"scope": "id", "follow_up_id": "{{var.sem_valor}}"}))
	if err != nil || result.NextNodeID != "bad" || len(empty.cancelled) != 0 {
		t.Fatalf("an empty id must not cancel everything: %+v, %v", result, err)
	}
}

func TestFollowUpNodesDeclareWhatTheyDo(t *testing.T) {
	for _, def := range []workflow.NodeDefinition{NewScheduleMessageExecutor(&followUpBook{}).Definition(), NewCancelScheduledMessagesExecutor(&followUpBook{}).Definition()} {
		if err := workflow.TestBehaviorConsistent(def); err != nil {
			t.Fatalf("%s: %v", def.Type, err)
		}
		if !def.NeedsConversation || def.TestBehavior != workflow.NodeTestWritesData {
			t.Fatalf("%s: %+v", def.Type, def)
		}
		if len(def.Outputs) != 2 || def.Outputs[1].ID != "erro" || !def.Outputs[1].Optional {
			t.Fatalf("%s outputs = %+v", def.Type, def.Outputs)
		}
	}
}

func TestScheduleMessageNodeReadsEverySchedulingFieldFromVariables(t *testing.T) {
	book := &followUpBook{}
	ctx := followUpNodeContext(workflow.NodeTypeActionScheduleMessage, map[string]interface{}{
		"text": "oi", "when_mode": "in", "delay_amount": "{{var.args.amount}}", "delay_unit": "{{var.args.unit}}",
	})
	ctx.State.Set("args", map[string]interface{}{"amount": "4", "unit": "minutes"})
	result, err := NewScheduleMessageExecutor(book).Execute(ctx)
	if err != nil || result.NextNodeID != "ok" || book.scheduled[0].When.InMinutes != 4 {
		t.Fatalf("result = %+v, %v, scheduled = %+v", result, err, book.scheduled)
	}

	book = &followUpBook{}
	ctx = followUpNodeContext(workflow.NodeTypeActionScheduleMessage, map[string]interface{}{"text": "oi", "when_mode": "day", "days_from_today": "{{var.dias}}", "time": "{{var.hora}}"})
	ctx.State.Set("dias", float64(1))
	ctx.State.Set("hora", "17:00")
	result, err = NewScheduleMessageExecutor(book).Execute(ctx)
	if err != nil || result.NextNodeID != "ok" || *book.scheduled[0].When.DaysFromToday != 1 || book.scheduled[0].When.Time != "17:00" {
		t.Fatalf("result = %+v, %v, scheduled = %+v", result, err, book.scheduled)
	}
}

func TestScheduleMessageNodeRoutesEveryBadSchedulingValueToErroNeverToNow(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]interface{}
		code   string
		field  string
	}{
		{"unresolved amount", map[string]interface{}{"when_mode": "in", "delay_amount": "{{var.sumiu}}", "delay_unit": "hours"}, "follow_up_variable_missing", "delay_amount"},
		{"text amount", map[string]interface{}{"when_mode": "in", "delay_amount": "{{var.nome}}", "delay_unit": "hours"}, "follow_up_field_invalid", "delay_amount"},
		{"zero amount", map[string]interface{}{"when_mode": "in", "delay_amount": float64(0), "delay_unit": "hours"}, "follow_up_field_invalid", "delay_amount"},
		{"fraction amount", map[string]interface{}{"when_mode": "in", "delay_amount": 1.5, "delay_unit": "hours"}, "follow_up_field_invalid", "delay_amount"},
		{"missing amount", map[string]interface{}{"when_mode": "in", "delay_unit": "hours"}, "follow_up_field_invalid", "delay_amount"},
		{"unknown unit", map[string]interface{}{"when_mode": "in", "delay_amount": float64(4), "delay_unit": "weeks"}, "follow_up_field_invalid", "delay_unit"},
		{"missing unit", map[string]interface{}{"when_mode": "in", "delay_amount": float64(4)}, "follow_up_field_invalid", "delay_unit"},
		{"unresolved days", map[string]interface{}{"when_mode": "day", "days_from_today": "{{var.sumiu}}", "time": "10:00"}, "follow_up_variable_missing", "days_from_today"},
		{"text days", map[string]interface{}{"when_mode": "day", "days_from_today": "amanhã", "time": "10:00"}, "follow_up_field_invalid", "days_from_today"},
		{"missing days", map[string]interface{}{"when_mode": "day", "time": "10:00"}, "follow_up_field_invalid", "days_from_today"},
		{"bad time", map[string]interface{}{"when_mode": "day", "days_from_today": float64(1), "time": "25:00"}, "follow_up_field_invalid", "time"},
		{"bad date", map[string]interface{}{"when_mode": "date", "date": "{{var.nome}}", "time": "10:00"}, "follow_up_field_invalid", "date"},
		{"unknown mode", map[string]interface{}{"when_mode": "logo"}, "follow_up_field_invalid", "when_mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			book := &followUpBook{}
			tc.config["text"] = "oi"
			result, err := NewScheduleMessageExecutor(book).Execute(followUpNodeContext(workflow.NodeTypeActionScheduleMessage, tc.config))
			if err != nil {
				t.Fatal(err)
			}
			if len(book.scheduled) != 0 {
				t.Fatalf("scheduled = %+v", book.scheduled)
			}
			if result.NextNodeID != "bad" || result.Output["error_code"] != tc.code || result.Output["error_field"] != tc.field {
				t.Fatalf("result = %+v", result)
			}
			if label, _ := result.Output["error_field_label"].(string); label == "" || label == tc.field {
				t.Fatalf("label = %v", result.Output["error_field_label"])
			}
		})
	}
}
