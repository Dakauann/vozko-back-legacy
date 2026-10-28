package payment_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/invoice"
	"vozko/domain/payment"
)

type recordedHooks struct {
	calls []string
	err   error
}

func (h *recordedHooks) OnPaymentReversed(_ context.Context, workspaceID string) error {
	h.calls = append(h.calls, "reversed:"+workspaceID)
	return h.err
}

func (h *recordedHooks) OnReversalReleased(_ context.Context, workspaceID string) error {
	h.calls = append(h.calls, "released:"+workspaceID)
	return h.err
}

func TestAChargebackCutsAccessAtOnceAndAWonDisputeRestoresIt(t *testing.T) {
	inv := paidPixTopUp()
	inv.BillingType = "CREDIT_CARD"
	rig := newReversalRig(inv)
	hooks := &recordedHooks{}
	rig.uc.WithReversalHooks(hooks)
	rig.credit(inv, 10_000_000)

	if err := rig.send(t, payment.EventPaymentChargebackRequested, 0); err != nil {
		t.Fatal(err)
	}
	if len(hooks.calls) != 1 || hooks.calls[0] != "reversed:ws-1" {
		t.Fatalf("calls = %v, want the cut right after the chargeback", hooks.calls)
	}
	if err := rig.send(t, payment.EventPaymentChargebackReversed, 0); err != nil {
		t.Fatal(err)
	}
	if len(hooks.calls) != 2 || hooks.calls[1] != "released:ws-1" {
		t.Fatalf("calls = %v, want the restore after the dispute was won", hooks.calls)
	}
}

func TestAPixMedOfANeverCreditedInvoiceStillCutsAccess(t *testing.T) {
	inv := paidPixTopUp()
	rig := newReversalRig(inv)
	hooks := &recordedHooks{err: errors.New("sites down")}
	rig.uc.WithReversalHooks(hooks)
	if err := rig.send(t, payment.EventPaymentRefunded, 0); err != nil {
		t.Fatalf("a failing hook never blocks the money step, got %v", err)
	}
	if len(hooks.calls) != 1 || hooks.calls[0] != "reversed:ws-1" {
		t.Fatalf("calls = %v", hooks.calls)
	}
}

func TestAReleaseArrivingBeforeItsOpeningNeverLeavesTheInvoiceReversed(t *testing.T) {
	inv := paidPixTopUp()
	inv.BillingType = "CREDIT_CARD"
	rig := newReversalRig(inv)
	hooks := &recordedHooks{}
	rig.uc.WithReversalHooks(hooks)
	rig.credit(inv, 10_000_000)

	for _, event := range []string{payment.EventPaymentChargebackReversed, payment.EventPaymentChargebackRequested} {
		if err := rig.send(t, event, 0); err != nil {
			t.Fatalf("%s: %v", event, err)
		}
	}
	if inv.Status != invoice.StatusPaid || rig.reversed(inv) != 0 || len(rig.book.debits) != 0 || len(rig.book.credits) != 0 {
		t.Fatalf("status = %s reversed = %d debits = %d credits = %d; the pair settles the same in either order",
			inv.Status, rig.reversed(inv), len(rig.book.debits), len(rig.book.credits))
	}
	if len(hooks.calls) != 0 {
		t.Fatalf("calls = %v; access is never cut for a dispute already won", hooks.calls)
	}
	if !rig.alerted("before") {
		t.Fatalf("ops hear about the out of order pair, alerts = %v", rig.alerts.subjects)
	}
}

func TestTheReversalPairSettlesTheSameInEitherOrder(t *testing.T) {
	for _, order := range [][]string{
		{payment.EventPaymentChargebackRequested, payment.EventPaymentChargebackReversed},
		{payment.EventPaymentChargebackReversed, payment.EventPaymentChargebackRequested},
	} {
		inv := paidPixTopUp()
		inv.BillingType = "CREDIT_CARD"
		rig := newReversalRig(inv)
		rig.credit(inv, 10_000_000)
		for _, event := range order {
			if err := rig.send(t, event, 0); err != nil {
				t.Fatalf("%v: %v", order, err)
			}
		}
		if inv.Status != invoice.StatusPaid || rig.reversed(inv) != 0 {
			t.Fatalf("%v: status = %s reversed = %d", order, inv.Status, rig.reversed(inv))
		}
	}
}

func TestALostDisputeStillTakesTheMoneyBackAfterAnEarlyRelease(t *testing.T) {
	inv := paidPixTopUp()
	inv.BillingType = "CREDIT_CARD"
	rig := newReversalRig(inv)
	rig.credit(inv, 10_000_000)
	for _, event := range []string{payment.EventPaymentChargebackReversed, payment.EventPaymentChargeback} {
		if err := rig.send(t, event, 0); err != nil {
			t.Fatalf("%s: %v", event, err)
		}
	}
	if inv.Status != invoice.StatusReversed || rig.reversed(inv) != 10_000_000 {
		t.Fatalf("status = %s reversed = %d; a final loss is always applied and handled by hand", inv.Status, rig.reversed(inv))
	}
}

func TestAnUnreadableReleaseRecordIsRetried(t *testing.T) {
	inv := paidPixTopUp()
	inv.BillingType = "CREDIT_CARD"
	rig := newReversalRig(inv)
	rig.credit(inv, 10_000_000)
	rig.releases.err = errors.New("redis down")

	if err := rig.send(t, payment.EventPaymentChargebackReversed, 0); err == nil {
		t.Fatal("a release that cannot be recorded is retried")
	}
	if err := rig.send(t, payment.EventPaymentChargebackRequested, 0); err == nil {
		t.Fatal("an opening that cannot check for an earlier release is retried")
	}
	if inv.Status != invoice.StatusPaid || len(rig.book.debits) != 0 {
		t.Fatalf("status = %s debits = %d; nothing moves while the record is unreadable", inv.Status, len(rig.book.debits))
	}
}

func TestAnEarlyReleaseIsRecordedLongEnoughForALateOpening(t *testing.T) {
	inv := paidPixTopUp()
	rig := newReversalRig(inv)
	if err := rig.send(t, payment.EventPaymentChargebackReversed, 0); err != nil {
		t.Fatal(err)
	}
	ttl, ok := rig.releases.marks[releasedAheadKey(inv.ID)]
	if !ok || ttl < 90*24*time.Hour {
		t.Fatalf("marks = %v", rig.releases.marks)
	}
}

func TestReversalsNeedTheReleaseRecord(t *testing.T) {
	inv := paidPixTopUp()
	rig := newReversalRig(inv)
	rig.uc.reversals = nil
	rig.uc.WithReversals(rig.book, rig.locks, rig.alerts, nil)
	if err := rig.send(t, payment.EventPaymentChargebackRequested, 0); err == nil {
		t.Fatal("without the release record the pair could not settle in either order")
	}
}
