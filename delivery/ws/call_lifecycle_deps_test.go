package ws

import (
	"context"
	"log"
	"time"

	cdr "vozko/domain/calls/cdr"
	callsession_domain "vozko/domain/callsession"
	"vozko/domain/messaging"
	callsession_usecase "vozko/usecases/callsession"
)

type swappableCDRStart struct{ inner cdr.StartCallUseCase }

func (s *swappableCDRStart) Execute(in cdr.StartCallInput) (*cdr.Call, error) {
	if s.inner == nil {
		return nil, nil
	}
	return s.inner.Execute(in)
}

type quietCDRAnswered struct{}

func (quietCDRAnswered) Execute(string, time.Time) error { return nil }

type quietCDRComplete struct{}

func (quietCDRComplete) Execute(cdr.CompleteCallInput) error { return nil }

type quietCallListItems struct{}

func (quietCallListItems) CheckItemDial(context.Context, callsession_domain.CallListItemDial) error {
	return nil
}
func (quietCallListItems) StampLastCall(context.Context, callsession_domain.CallListItemStamp) error {
	return nil
}

func lifecycleFor(admission callsession_domain.CallAdmissionCoordinator, pub messaging.MessageQueuePub, logger *log.Logger, starts *swappableCDRStart) (*callsession_usecase.OutboundCallLifecycleRunner, error) {
	if starts == nil {
		starts = &swappableCDRStart{}
	}
	return callsession_usecase.NewOutboundCallLifecycleRunner(callsession_usecase.OutboundCallLifecycleDeps{
		Admission:     admission,
		BillingPub:    pub,
		CDRStart:      starts,
		CDRAnswered:   quietCDRAnswered{},
		CDRComplete:   quietCDRComplete{},
		CallListItems: quietCallListItems{},
		Logger:        logger,
	})
}
