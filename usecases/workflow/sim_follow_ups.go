package workflow_usecase

import (
	"context"
	"fmt"
	"time"

	sm "vozko/domain/scheduled_message"
	"vozko/domain/workflow"
)

const simulatedFollowUpID = "simulado"

type simFollowUps struct {
	effects *simEffects
}

func (f simFollowUps) Schedule(_ context.Context, in sm.FollowUpInput) (*sm.FollowUpScheduled, error) {
	now := time.Now().UTC()
	at, err := in.When.Resolve(now, time.UTC)
	if err != nil {
		return nil, err
	}
	description := "Agendaria a mensagem " + describeFollowUpWhen(in.When)
	if in.CancelOnReply {
		description += ", cancelada se o contato responder antes"
	}
	f.effects.change(workflow.SimulatedChange{
		Kind: workflow.SimulatedFollowUp, Action: "schedule", Value: in.Text, Description: description,
	})
	message := &sm.ScheduledMessage{ID: simulatedFollowUpID, Kind: sm.KindText, Text: in.Text, ScheduledAt: at, CancelOnReply: in.CancelOnReply}
	return &sm.FollowUpScheduled{Message: message, Local: at}, nil
}

func (f simFollowUps) Cancel(_ context.Context, in sm.FollowUpCancel) ([]*sm.ScheduledMessage, error) {
	description := "Cancelaria as mensagens pendentes que este fluxo agendou nesta conversa"
	if in.ID != "" {
		description = "Cancelaria a mensagem agendada " + in.ID
	}
	f.effects.change(workflow.SimulatedChange{
		Kind: workflow.SimulatedFollowUp, Action: "cancel", TargetID: in.ID, Description: description,
	})
	return nil, nil
}

func describeFollowUpWhen(w sm.FollowUpWhen) string {
	switch {
	case w.InMinutes > 0:
		return fmt.Sprintf("para daqui a %d minutos", w.InMinutes)
	case w.Date != "":
		return fmt.Sprintf("para %s às %s", w.Date, w.Time)
	case w.DaysFromToday != nil && *w.DaysFromToday > 0:
		return fmt.Sprintf("para daqui a %d dia(s) às %s", *w.DaysFromToday, w.Time)
	}
	return "para hoje às " + w.Time
}

var _ sm.FollowUpUseCase = simFollowUps{}
