package billing_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/invoice"
	workspace_addon "vozko/domain/workspace/workspace_addon"
)

type recordingAccessHooks struct {
	cut, restored []string
	cutErr        error
	restoreErr    error
}

func (h *recordingAccessHooks) OnAccessCut(_ context.Context, workspaceID string) error {
	h.cut = append(h.cut, workspaceID)
	return h.cutErr
}

func (h *recordingAccessHooks) OnAccessRestored(_ context.Context, workspaceID string) error {
	h.restored = append(h.restored, workspaceID)
	return h.restoreErr
}

func TestSweepCutsWorkspaceAccessThroughTheHooks(t *testing.T) {
	inv := &fakeInvoiceRepo{unpaid: []invoice.Invoice{unpaidMonthly("inv-1", "ws-1")}}
	subs := &fakeSubs{latest: planSub("ws-1", time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC))}
	hooks := &recordingAccessHooks{}
	uc := sweepFixture(inv, subs, &fakeAddons{}, &fakeEntitlementHandler{}, &fakeAlerter{}, 28).WithAccessHooks(hooks)

	if n, err := uc.Execute(); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(hooks.cut) != 1 || hooks.cut[0] != "ws-1" || inv.statuses["inv-1"] != invoice.StatusExpired {
		t.Fatalf("cut=%v status=%q", hooks.cut, inv.statuses["inv-1"])
	}
}

func TestSweepRetriesWhenTheAccessCutFails(t *testing.T) {
	inv := &fakeInvoiceRepo{unpaid: []invoice.Invoice{unpaidMonthly("inv-1", "ws-1")}}
	subs := &fakeSubs{latest: planSub("ws-1", time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC))}
	hooks := &recordingAccessHooks{cutErr: errors.New("kv down")}
	alerter := &fakeAlerter{}
	uc := sweepFixture(inv, subs, &fakeAddons{}, &fakeEntitlementHandler{}, alerter, 28).WithAccessHooks(hooks)

	if n, err := uc.Execute(); err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if inv.statuses["inv-1"] == invoice.StatusExpired || alerter.calls != 1 {
		t.Fatalf("the invoice must stay unpaid for the retry and alert once, status=%q alerts=%d", inv.statuses["inv-1"], alerter.calls)
	}
}

func TestLatePaymentRestoresWorkspaceAccess(t *testing.T) {
	anchor := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)
	subs := &fakeSubs{latest: expiredPlanSub("ws-1", anchor)}
	addons := &fakeAddons{byWS: map[string][]*workspace_addon.AddonSubscription{"ws-1": {expiredAddonSub(anchor)}}}
	hooks := &recordingAccessHooks{}

	if err := confirmFixture(subs, addons).WithAccessHooks(hooks).Execute("ws-1"); err != nil {
		t.Fatal(err)
	}
	if len(hooks.restored) != 1 || hooks.restored[0] != "ws-1" {
		t.Fatalf("restored=%v", hooks.restored)
	}
}

func TestOnTimePaymentLeavesAccessAlone(t *testing.T) {
	anchor := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)
	subs := &fakeSubs{latest: planSub("ws-1", anchor)}
	hooks := &recordingAccessHooks{}
	if err := confirmFixture(subs, &fakeAddons{}).WithAccessHooks(hooks).Execute("ws-1"); err != nil {
		t.Fatal(err)
	}
	if len(hooks.restored) != 0 {
		t.Fatalf("restored=%v", hooks.restored)
	}
}

func TestRestoreFailureDoesNotFailThePayment(t *testing.T) {
	anchor := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)
	subs := &fakeSubs{latest: expiredPlanSub("ws-1", anchor)}
	hooks := &recordingAccessHooks{restoreErr: errors.New("kv down")}
	if err := confirmFixture(subs, &fakeAddons{}).WithAccessHooks(hooks).Execute("ws-1"); err != nil {
		t.Fatalf("a paid invoice must stay confirmed, got %v", err)
	}
}
