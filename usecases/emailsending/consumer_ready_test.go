package emailsending

import (
	"errors"
	"testing"
)

type startCounter struct{ starts int }

func (s *startCounter) Start() error {
	s.starts++
	return nil
}

func TestTheNotificationConsumerStartsOnlyOnceEveryHandlerIsBound(t *testing.T) {
	h := newHarness(t)
	runner := &startCounter{}
	consumer := &Consumer{runner: runner, events: h.m.Events}

	if err := consumer.Start(); !errors.Is(err, ErrNoUnsubscriber) || !errors.Is(err, ErrNoInboundHandler) {
		t.Fatalf("Start without handlers = %v, want both refusals", err)
	}
	h.m.Events.SetUnsubscriber(&recordingUnsubscriber{})
	if err := consumer.Start(); !errors.Is(err, ErrNoInboundHandler) || errors.Is(err, ErrNoUnsubscriber) {
		t.Fatalf("Start without the inbound handler = %v", err)
	}
	h.m.Events.SetInboundHandler(&recordingInbound{})
	if err := consumer.Start(); err != nil || runner.starts != 1 {
		t.Fatalf("Start = %v starts = %d, want one start once bound", err, runner.starts)
	}
}
