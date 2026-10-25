package whatsapp_campaign_usecase

import (
	"errors"
	"testing"

	"vozko/domain/campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
	"vozko/usecases/campaignqueue"
)

func withBalance(h *testHarness, micros int64) {
	h.cachedBalanceChecker.mu.Lock()
	h.cachedBalanceChecker.balanceMicros = micros
	h.cachedBalanceChecker.mu.Unlock()
}

func TestNoBalanceNeverReadsTheContact(t *testing.T) {
	h := newTestHarness()
	msg := seedRunningEntry(h, "lead-1")
	h.consumeTempl.cost = 100
	withBalance(h, 99)

	got := h.consumer.handle(msg)

	if got.Outcome != campaignqueue.OutcomeRetryLater || got.Delay != noBalanceRetryDelay {
		t.Fatalf("outcome = %+v, want a retry in %s", got, noBalanceRetryDelay)
	}
	if n := h.entryRepo.findByIDCount(); n != 0 {
		t.Fatalf("read the contact %d time(s) with no balance", n)
	}
	if len(h.screener.calls) != 0 || h.screener.claims != 0 {
		t.Fatalf("screened %d time(s) and claimed %d time(s) with no balance", len(h.screener.calls), h.screener.claims)
	}
	if n := len(h.consumeTempl.executeCalls); n != 0 {
		t.Fatalf("debited %d time(s) with no balance", n)
	}
	if inflight, _ := h.inflightReserver.GetInflight("ws-1"); inflight != 0 {
		t.Fatalf("left %d micros reserved", inflight)
	}
	if status := h.entryRepo.entries["entry-1"].Status; status != wce.SendStatusPending {
		t.Fatalf("entry status = %s, want pending", status)
	}
}

func TestBalanceThatCoversOneMessageKeepsTheFlow(t *testing.T) {
	h := newTestHarness()
	msg := seedRunningEntry(h, "lead-1")
	h.consumeTempl.cost = 100
	withBalance(h, 100)

	got := h.consumer.handle(msg)

	if got.Outcome != campaignqueue.OutcomeDone {
		t.Fatalf("outcome = %+v, want Done", got)
	}
	if n := h.entryRepo.findByIDCount(); n != 1 {
		t.Fatalf("read the contact %d time(s), want once", n)
	}
	if len(h.screener.calls) != 1 || h.screener.claims != 1 {
		t.Fatalf("screened %d time(s) and claimed %d time(s), want once each", len(h.screener.calls), h.screener.claims)
	}
	if n := len(h.consumeTempl.executeCalls); n != 1 || h.waClient.sendCalls.Load() != 1 {
		t.Fatalf("debits = %d sends = %d, want one each", n, h.waClient.sendCalls.Load())
	}
	if inflight, _ := h.inflightReserver.GetInflight("ws-1"); inflight != 0 {
		t.Fatalf("left %d micros reserved", inflight)
	}
	if status := h.entryRepo.entries["entry-1"].Status; status != wce.SendStatusSent {
		t.Fatalf("entry status = %s, want sent", status)
	}
}

func TestReservationContentionKeepsTheShortRetry(t *testing.T) {
	h := newTestHarness()
	msg := seedRunningEntry(h, "lead-1")
	h.consumeTempl.cost = 500
	withBalance(h, 10_000)
	h.shared.TryIncrBy("balance:inflight:ws-1", 9_900, 10_000)

	got := h.consumer.handle(msg)

	if got.Outcome != campaignqueue.OutcomeRetryLater || got.Delay != balanceRequeueDelay {
		t.Fatalf("outcome = %+v, want the reservation retry in %s", got, balanceRequeueDelay)
	}
	if n := h.entryRepo.findByIDCount(); n != 1 {
		t.Fatalf("read the contact %d time(s), want once: only Reserve may refuse here", n)
	}
}

func TestSkipStatusesWaitForBalanceAndThenApply(t *testing.T) {
	h := newTestHarness()
	msg := seedRunningEntry(h, "lead-1")
	h.screener.skip["lead-1"] = campaign.SkipOptedOut
	h.consumeTempl.cost = 100
	withBalance(h, 0)

	if got := h.consumer.handle(msg); got.Outcome != campaignqueue.OutcomeRetryLater {
		t.Fatalf("outcome without balance = %+v, want a retry", got)
	}
	if entry := h.entryRepo.entries["entry-1"]; entry.Status != wce.SendStatusPending || entry.ErrorCode != 0 {
		t.Fatalf("entry = (%s, %d) without balance, want it untouched", entry.Status, entry.ErrorCode)
	}

	withBalance(h, 100)

	if got := h.consumer.handle(msg); got.Outcome != campaignqueue.OutcomeDrop {
		t.Fatalf("outcome with balance = %+v, want Drop", got)
	}
	entry := h.entryRepo.entries["entry-1"]
	if entry.Status != wce.SendStatusFailed || entry.ErrorCode != 920002 || entry.ErrorMessage != string(campaign.SkipOptedOut) {
		t.Fatalf("entry = (%s, %d, %q), want the opted out skip", entry.Status, entry.ErrorCode, entry.ErrorMessage)
	}
	if n := len(h.consumeTempl.executeCalls); n != 0 || h.waClient.sendCalls.Load() != 0 {
		t.Fatalf("debits = %d sends = %d for an opted out lead", n, h.waClient.sendCalls.Load())
	}
}

func TestUnreadableBalanceHoldsTheMessageWithoutReadingTheContact(t *testing.T) {
	h := newTestHarness()
	msg := seedRunningEntry(h, "lead-1")
	h.cachedBalanceChecker.mu.Lock()
	h.cachedBalanceChecker.getBalanceErr = errors.New("redis down")
	h.cachedBalanceChecker.mu.Unlock()

	got := h.consumer.handle(msg)

	if got.Outcome != campaignqueue.OutcomeRequeue {
		t.Fatalf("outcome = %+v, want Requeue (fail-closed)", got)
	}
	if n := h.entryRepo.findByIDCount(); n != 0 {
		t.Fatalf("read the contact %d time(s) while the balance is unreadable", n)
	}
	if n := len(h.consumeTempl.executeCalls); n != 0 {
		t.Fatalf("debited %d time(s) while the balance is unreadable", n)
	}
}
