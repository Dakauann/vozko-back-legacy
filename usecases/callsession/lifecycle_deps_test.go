package callsession_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	cdr "vozko/domain/calls/cdr"
	"vozko/domain/callsession"
	"vozko/domain/shared/deps"
)

type quietCDRStart struct{}

func (quietCDRStart) Execute(cdr.StartCallInput) (*cdr.Call, error) { return nil, nil }

type quietCDRAnswered struct{}

func (quietCDRAnswered) Execute(string, time.Time) error { return nil }

type quietCDRComplete struct{}

func (quietCDRComplete) Execute(cdr.CompleteCallInput) error { return nil }

type quietCallListItems struct{}

func (quietCallListItems) CheckItemDial(context.Context, callsession.CallListItemDial) error {
	return nil
}
func (quietCallListItems) StampLastCall(context.Context, callsession.CallListItemStamp) error {
	return nil
}

func lifecycleDeps(d OutboundCallLifecycleDeps) OutboundCallLifecycleDeps {
	if d.CDRStart == nil {
		d.CDRStart = quietCDRStart{}
	}
	if d.CDRAnswered == nil {
		d.CDRAnswered = quietCDRAnswered{}
	}
	if d.CDRComplete == nil {
		d.CDRComplete = quietCDRComplete{}
	}
	if d.CallListItems == nil {
		d.CallListItems = quietCallListItems{}
	}
	return d
}

func TestTheLifecycleRefusesMissingCallRecordSteps(t *testing.T) {
	_, err := NewOutboundCallLifecycleRunner(OutboundCallLifecycleDeps{BillingPub: &fakeBillingPub{}})
	var missing *deps.MissingError
	if !errors.As(err, &missing) || len(missing.Missing) != 4 {
		t.Fatalf("err = %v, want the four call record steps refused", err)
	}
}
