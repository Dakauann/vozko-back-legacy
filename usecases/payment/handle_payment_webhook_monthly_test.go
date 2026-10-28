package payment_usecase

import (
	"errors"
	"testing"
	"time"

	"vozko/domain/balance"
	"vozko/domain/invoice"
	"vozko/domain/payment"
)

type erroringCreditBalance struct{ err error }

func (u *erroringCreditBalance) Execute(balance.CreditBalanceInput) (*balance.Transaction, error) {
	return nil, u.err
}

type webhookConfirmMonthly struct {
	calls []string
	dues  []time.Time
	err   error
}

func (c *webhookConfirmMonthly) ExecuteForCycle(workspaceID string, billedDue time.Time) error {
	c.calls = append(c.calls, workspaceID)
	c.dues = append(c.dues, billedDue)
	return c.err
}

func TestHandleAsaasWebhook_MonthlyBillingCreditsOnlyPlanPortionAndExtends(t *testing.T) {
	repo := &webhookInvoiceRepo{byExternal: map[string]*invoice.Invoice{
		"pay-m": {
			ID:            "inv-m",
			ExternalID:    "pay-m",
			WorkspaceID:   "ws-1",
			Purpose:       invoice.PurposeMonthlyBilling,
			AmountBRL:     1399,
			AmountUSD:     233_166_667,
			CreditableUSD: 183_166_667,
			ExchangeRate:  6,
		},
	}}
	credit := &webhookCreditBalance{}
	confirm := &webhookConfirmMonthly{}
	uc := NewHandlePaymentWebhookUseCase(repo, nil, credit, &webhookDebitBalance{}, nil).
		WithMonthlyBilling(confirm)

	if err := uc.Execute(&payment.AsaasWebhookEvent{Event: "PAYMENT_RECEIVED", Payment: payment.AsaasWebhookPayment{ID: "pay-m"}}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(credit.calls) != 1 {
		t.Fatalf("expected one balance credit, got %d", len(credit.calls))
	}
	if credit.calls[0].Amount != 183_166_667 {
		t.Fatalf("credit Amount = %d, want 183_166_667 (plan portion only, NOT the full %d)", credit.calls[0].Amount, 233_166_667)
	}
	if credit.calls[0].WorkspaceID != "ws-1" {
		t.Fatalf("credit workspace = %q, want ws-1", credit.calls[0].WorkspaceID)
	}
	if len(confirm.calls) != 1 || confirm.calls[0] != "ws-1" {
		t.Fatalf("expected subscription extension for ws-1, got %v", confirm.calls)
	}
}

func TestHandleAsaasWebhook_MonthlyBillingCreditableFallsBackToAmount(t *testing.T) {
	repo := &webhookInvoiceRepo{byExternal: map[string]*invoice.Invoice{
		"pay-old": {
			ID: "inv-old", ExternalID: "pay-old", WorkspaceID: "ws-9",
			Purpose: invoice.PurposeMonthlyBilling, AmountBRL: 60, AmountUSD: 10_000_000, CreditableUSD: 0, ExchangeRate: 6,
		},
	}}
	credit := &webhookCreditBalance{}
	uc := NewHandlePaymentWebhookUseCase(repo, nil, credit, &webhookDebitBalance{}, nil)

	if err := uc.Execute(&payment.AsaasWebhookEvent{Event: "PAYMENT_RECEIVED", Payment: payment.AsaasWebhookPayment{ID: "pay-old"}}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(credit.calls) != 1 || credit.calls[0].Amount != 10_000_000 {
		t.Fatalf("expected fallback credit of the full AmountUSD, got %+v", credit.calls)
	}
}

func TestHandleAsaasWebhook_MonthlyBillingAlreadyPaidDoesNotReprocess(t *testing.T) {
	paid := &invoice.Invoice{
		ID: "inv-p", ExternalID: "pay-p", WorkspaceID: "ws-1",
		Purpose: invoice.PurposeMonthlyBilling, AmountUSD: 10_000_000, CreditableUSD: 10_000_000,
		Status: invoice.StatusPaid, ExchangeRate: 6,
	}
	repo := &webhookInvoiceRepo{byExternal: map[string]*invoice.Invoice{"pay-p": paid}}
	credit := &webhookCreditBalance{}
	confirm := &webhookConfirmMonthly{}
	uc := NewHandlePaymentWebhookUseCase(repo, nil, credit, &webhookDebitBalance{}, nil).
		WithMonthlyBilling(confirm)

	if err := uc.Execute(&payment.AsaasWebhookEvent{Event: "PAYMENT_RECEIVED", Payment: payment.AsaasWebhookPayment{ID: "pay-p"}}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(credit.calls) != 0 || len(confirm.calls) != 0 {
		t.Fatalf("an already-paid invoice must not re-credit or re-extend, credit=%d confirm=%d", len(credit.calls), len(confirm.calls))
	}
}

func TestHandleAsaasWebhook_MonthlyBillingOverdueMarksOverdue(t *testing.T) {
	repo := &webhookInvoiceRepo{byExternal: map[string]*invoice.Invoice{
		"pay-o": {ID: "inv-o", ExternalID: "pay-o", WorkspaceID: "ws-1", Purpose: invoice.PurposeMonthlyBilling, AmountUSD: 10_000_000, ExchangeRate: 6},
	}}
	credit := &webhookCreditBalance{}
	uc := NewHandlePaymentWebhookUseCase(repo, nil, credit, &webhookDebitBalance{}, nil)

	if err := uc.Execute(&payment.AsaasWebhookEvent{Event: "PAYMENT_OVERDUE", Payment: payment.AsaasWebhookPayment{ID: "pay-o"}}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(repo.statusUpdates) != 1 || repo.statusUpdates[0] != invoice.StatusOverdue {
		t.Fatalf("an overdue monthly invoice must be marked Overdue (drives the dunning window), got %v", repo.statusUpdates)
	}
	if len(credit.calls) != 0 {
		t.Fatalf("an overdue event must never credit saldo, got %d", len(credit.calls))
	}
}

func TestHandleAsaasWebhook_MonthlyBillingCreditFailureRollsBackToPending(t *testing.T) {
	repo := &webhookInvoiceRepo{byExternal: map[string]*invoice.Invoice{
		"pay-f": {ID: "inv-f", ExternalID: "pay-f", WorkspaceID: "ws-1", Purpose: invoice.PurposeMonthlyBilling,
			AmountUSD: 10_000_000, CreditableUSD: 10_000_000, ExchangeRate: 6},
	}}
	credit := &erroringCreditBalance{err: errors.New("ledger down")}
	confirm := &webhookConfirmMonthly{}
	uc := NewHandlePaymentWebhookUseCase(repo, nil, credit, &webhookDebitBalance{}, nil).
		WithMonthlyBilling(confirm)

	err := uc.Execute(&payment.AsaasWebhookEvent{Event: "PAYMENT_RECEIVED", Payment: payment.AsaasWebhookPayment{ID: "pay-f"}})
	if err == nil {
		t.Fatal("a saldo-credit failure must surface as an error so the webhook is retried")
	}
	if last := repo.statusUpdates[len(repo.statusUpdates)-1]; last != invoice.StatusPending {
		t.Fatalf("expected rollback to Pending after a failed credit, got %v", repo.statusUpdates)
	}
	if len(confirm.calls) != 0 {
		t.Fatalf("subscriptions must not be extended when the credit failed, got %v", confirm.calls)
	}
}

func TestHandleAsaasWebhook_AnAddonOnlyInvoiceCreditsNothingAndConfirmsItsOwnCycle(t *testing.T) {
	due := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	repo := &webhookInvoiceRepo{byExternal: map[string]*invoice.Invoice{
		"pay-a": {
			ID: "inv-a", ExternalID: "pay-a", WorkspaceID: "ws-2", Purpose: invoice.PurposeMonthlyBilling,
			AmountBRL: 10, AmountUSD: 2_000_000, CreditableUSD: 0, ExchangeRate: 5, DueDate: &due,
			LineItems: []invoice.InvoiceLineItem{{Kind: invoice.LineItemChannel, Label: "Domínios", AmountBRL: 10, Quantity: 2}},
		},
	}}
	credit := &webhookCreditBalance{}
	confirm := &webhookConfirmMonthly{}
	uc := NewHandlePaymentWebhookUseCase(repo, nil, credit, &webhookDebitBalance{}, nil).
		WithMonthlyBilling(confirm)

	if err := uc.Execute(&payment.AsaasWebhookEvent{Event: "PAYMENT_RECEIVED", Payment: payment.AsaasWebhookPayment{ID: "pay-a"}}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(credit.calls) != 0 {
		t.Fatalf("paying for add-ons never turns into wallet balance, got %+v", credit.calls)
	}
	if len(confirm.dues) != 1 || !confirm.dues[0].Equal(due) || confirm.calls[0] != "ws-2" {
		t.Fatalf("the payment confirms the cycle the invoice billed, got %v %v", confirm.calls, confirm.dues)
	}
}
