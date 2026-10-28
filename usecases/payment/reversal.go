package payment_usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"vozko/domain/balance"
	billing "vozko/domain/billing"
	"vozko/domain/invoice"
	"vozko/domain/notification"
	"vozko/domain/payment"
	"vozko/domain/shared"
	"vozko/domain/workspace"
)

const (
	reversalLockTimeout = 30 * time.Second
	releasedAheadPrefix = "invoice-reversal-released:"
	releasedAheadTTL    = 180 * 24 * time.Hour
)

var errReversalsNotWired = errors.New("payment reversals need a ledger reader, a lock, an ops alerter and a release record")

type keyLocker interface {
	Acquire(ctx context.Context, key string) (func() (bool, error), error)
}

type releaseRecord interface {
	SetString(key, value string, ttl time.Duration) error
	Exists(key string) (bool, error)
}

type reversalDeps struct {
	ledger   balance.ReferenceNetReader
	locks    keyLocker
	alerts   billing.OpsAlerter
	releases releaseRecord
}

func (uc *handlePaymentWebhookUseCase) WithReversals(ledger balance.ReferenceNetReader, locks keyLocker, alerts billing.OpsAlerter, releases releaseRecord) *handlePaymentWebhookUseCase {
	if ledger != nil && locks != nil && alerts != nil && releases != nil {
		uc.reversals = &reversalDeps{ledger: ledger, locks: locks, alerts: alerts, releases: releases}
	}
	return uc
}

func releasedAheadKey(invoiceID string) string {
	return releasedAheadPrefix + invoiceID
}

type ReversalHooks interface {
	OnPaymentReversed(ctx context.Context, workspaceID string) error
	OnReversalReleased(ctx context.Context, workspaceID string) error
}

func (uc *handlePaymentWebhookUseCase) WithReversalHooks(hooks ReversalHooks) *handlePaymentWebhookUseCase {
	uc.reversalHooks = hooks
	return uc
}

func (uc *handlePaymentWebhookUseCase) afterReversal(workspaceID string, released bool) {
	if uc.reversalHooks == nil {
		return
	}
	hook, name := uc.reversalHooks.OnPaymentReversed, "cut"
	if released {
		hook, name = uc.reversalHooks.OnReversalReleased, "restore"
	}
	if err := hook(context.Background(), workspaceID); err != nil {
		log.Printf("[invoice-webhook] workspace %s: access %s after a payment reversal failed, the sites sweep retries it: %v", workspaceID, name, err)
	}
}

func (uc *handlePaymentWebhookUseCase) reverse(inv *invoice.Invoice, event *payment.WebhookEvent) error {
	if err := uc.takeBack(inv, event); err != nil {
		return err
	}
	if inv.Status == invoice.StatusReversed {
		uc.afterReversal(inv.WorkspaceID, false)
	}
	return nil
}

func (uc *handlePaymentWebhookUseCase) takeBack(inv *invoice.Invoice, event *payment.WebhookEvent) error {
	if uc.reversals == nil {
		return errReversalsNotWired
	}
	if inv.Status == invoice.StatusRefunded {
		log.Printf("[invoice-webhook] %s for invoice %s: already refunded by the legacy path", event.Event, inv.ID)
		return nil
	}
	unlock, err := uc.lockInvoice(inv.ID)
	if err != nil {
		return err
	}
	defer unlock()

	if meaning, _ := payment.MeaningOf(event.Event); meaning == payment.MeaningReversalOpened {
		released, err := uc.reversals.releases.Exists(releasedAheadKey(inv.ID))
		if err != nil {
			return fmt.Errorf("read the early release of invoice %s: %w", inv.ID, err)
		}
		if released {
			uc.alert("Payment reversal opened after its release",
				fmt.Sprintf("invoice=%s workspace=%s event=%s: the dispute was already released, nothing is taken back", inv.ID, inv.WorkspaceID, event.Event))
			return nil
		}
	}

	kind := payment.ReversalKindForBillingType(inv.BillingType)
	credited, err := uc.reversals.ledger.NetCreditUnder(inv.WorkspaceID, inv.ID)
	if err != nil {
		return fmt.Errorf("read credit of invoice %s: %w", inv.ID, err)
	}
	if credited <= 0 {
		if err := uc.markReversed(inv); err != nil {
			return err
		}
		uc.alert("Reversal of an invoice that was never credited",
			fmt.Sprintf("invoice=%s kind=%s event=%s status=%s", inv.ID, kind, event.Event, inv.Status))
		return nil
	}

	reversedBRL := event.Payment.ReversedValue
	if event.Event == payment.EventPaymentPartiallyRefunded && reversedBRL <= 0 {
		uc.alert("Partial reversal without its amount",
			fmt.Sprintf("invoice=%s kind=%s: the provider sent no reversed value, so the whole credit was taken back; review by hand", inv.ID, kind))
	}
	target := invoice.ReversalTargetMicros(credited, inv.AmountBRL, reversedBRL)
	already, err := uc.reversedSoFar(inv)
	if err != nil {
		return err
	}
	if delta := target - already; delta > 0 {
		ref := invoice.ReversalReference(inv.ID)
		if _, err := uc.debitBalance.Execute(balance.DebitBalanceInput{
			WorkspaceID:        inv.WorkspaceID,
			Amount:             delta,
			ServiceType:        balance.ServiceTopUp,
			ReferenceID:        &ref,
			Description:        reversalDescription(kind, inv),
			IsRefund:           true,
			AllowNegative:      true,
			ExchangeRateMicros: invoiceRateMicros(inv),
		}); err != nil {
			return fmt.Errorf("take back %d micros for reversed invoice %s: %w", delta, inv.ID, err)
		}
	}
	if err := uc.markReversed(inv); err != nil {
		return err
	}

	reversedAmount := inv.AmountBRL * float64(target) / float64(credited)
	uc.alert("Payment "+string(kind)+" recorded", fmt.Sprintf("invoice=%s workspace=%s event=%s taken back=%d micros of %d", inv.ID, inv.WorkspaceID, event.Event, target, credited))
	uc.notifyReversal(inv, kind, reversedAmount, target)
	return nil
}

func (uc *handlePaymentWebhookUseCase) releaseReversal(inv *invoice.Invoice) error {
	if uc.reversals == nil {
		return errReversalsNotWired
	}
	unlock, err := uc.lockInvoice(inv.ID)
	if err != nil {
		return err
	}
	defer unlock()

	current, err := uc.invoiceRepo.GetByID(inv.ID)
	if err != nil {
		return fmt.Errorf("read invoice %s for its release: %w", inv.ID, err)
	}
	inv.Status = current.Status
	if inv.Status != invoice.StatusReversed {
		return uc.recordReleaseAhead(inv)
	}

	already, err := uc.reversedSoFar(inv)
	if err != nil {
		return err
	}
	if already <= 0 {
		uc.alert("Payment released for an invoice with nothing reversed",
			fmt.Sprintf("invoice=%s workspace=%s: nothing was taken back, so nothing is given back; review by hand", inv.ID, inv.WorkspaceID))
		return nil
	}
	ref := invoice.ReversalReference(inv.ID)
	if _, err := uc.creditBalance.Execute(balance.CreditBalanceInput{
		WorkspaceID:        inv.WorkspaceID,
		Amount:             already,
		ServiceType:        balance.ServiceTopUp,
		ReferenceID:        &ref,
		Description:        fmt.Sprintf("Contestação revertida a seu favor: R$ %.2f", inv.AmountBRL),
		IsRefund:           true,
		ExchangeRateMicros: invoiceRateMicros(inv),
	}); err != nil {
		return fmt.Errorf("give back %d micros for released invoice %s: %w", already, inv.ID, err)
	}
	if err := uc.invoiceRepo.UpdateStatus(inv.ID, invoice.StatusPaid); err != nil {
		return err
	}
	inv.Status = invoice.StatusPaid
	uc.afterReversal(inv.WorkspaceID, true)
	uc.alert("Payment reversal released", fmt.Sprintf("invoice=%s workspace=%s given back=%d micros", inv.ID, inv.WorkspaceID, already))
	uc.notifyRelease(inv)
	return nil
}

func (uc *handlePaymentWebhookUseCase) recordReleaseAhead(inv *invoice.Invoice) error {
	if err := uc.reversals.releases.SetString(releasedAheadKey(inv.ID), string(inv.Status), releasedAheadTTL); err != nil {
		return fmt.Errorf("record the early release of invoice %s: %w", inv.ID, err)
	}
	uc.alert("Payment reversal released before it opened",
		fmt.Sprintf("invoice=%s workspace=%s status=%s: a later opening of this reversal takes nothing back", inv.ID, inv.WorkspaceID, inv.Status))
	return nil
}

func (uc *handlePaymentWebhookUseCase) lockInvoice(invoiceID string) (func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), reversalLockTimeout)
	defer cancel()
	release, err := uc.reversals.locks.Acquire(ctx, "invoice-reversal:"+invoiceID)
	if err != nil {
		return nil, fmt.Errorf("lock invoice %s for its reversal: %w", invoiceID, err)
	}
	return func() {
		if _, err := release(); err != nil {
			log.Printf("[invoice-webhook] release reversal lock of invoice %s: %v", invoiceID, err)
		}
	}, nil
}

func (uc *handlePaymentWebhookUseCase) reversedSoFar(inv *invoice.Invoice) (int64, error) {
	net, err := uc.reversals.ledger.NetCreditUnder(inv.WorkspaceID, invoice.ReversalReference(inv.ID))
	if err != nil {
		return 0, fmt.Errorf("read reversals of invoice %s: %w", inv.ID, err)
	}
	return -net, nil
}

func (uc *handlePaymentWebhookUseCase) markReversed(inv *invoice.Invoice) error {
	if inv.Status == invoice.StatusReversed {
		return nil
	}
	if err := uc.invoiceRepo.UpdateStatus(inv.ID, invoice.StatusReversed); err != nil {
		return fmt.Errorf("mark invoice %s reversed: %w", inv.ID, err)
	}
	inv.Status = invoice.StatusReversed
	return nil
}

func (uc *handlePaymentWebhookUseCase) alert(subject, detail string) {
	if uc.reversals == nil {
		log.Printf("[invoice-webhook] ALERT %s: %s", subject, detail)
		return
	}
	if err := uc.reversals.alerts.Alert(context.Background(), subject, detail); err != nil {
		log.Printf("[invoice-webhook] ALERT %s: %s (alerter failed: %v)", subject, detail, err)
	}
}

var reversalTitles = map[payment.ReversalKind]string{
	payment.ReversalChargeback:       "Contestação de pagamento no cartão",
	payment.ReversalPixReturn:        "Devolução de PIX pelo banco do pagador",
	payment.ReversalUnexpectedRefund: "Estorno de pagamento registrado",
}

func reversalDescription(kind payment.ReversalKind, inv *invoice.Invoice) string {
	return fmt.Sprintf("%s: R$ %.2f", reversalTitles[kind], inv.AmountBRL)
}

func (uc *handlePaymentWebhookUseCase) notifyReversal(inv *invoice.Invoice, kind payment.ReversalKind, reversedBRL float64, targetMicros int64) {
	if uc.notifications == nil {
		return
	}
	_, err := uc.notifications.Publish(context.Background(), notification.Draft{
		WorkspaceID: inv.WorkspaceID,
		Category:    notification.CategoryBilling,
		Severity:    notification.SeverityWarning,
		Title:       reversalTitles[kind],
		Body: "O banco do pagador devolveu " + shared.FormatBRL(reversedBRL) + " de um pagamento. " +
			"Esse valor saiu do saldo da carteira, que pode ficar negativo até a próxima recarga.",
		ActionURL:   balancePath,
		ActionLabel: "Ver saldo",
		Audience:    notification.Holding(workspace.ResourceBalance, workspace.ActionRead),
		Email:       true,
		DedupKey:    fmt.Sprintf("payment_reversal:%s:%d", inv.ID, targetMicros),
		Source:      notification.SourcePaymentReversal,
	})
	if err != nil {
		log.Printf("[invoice-webhook] reversal notification for invoice %s failed: %v", inv.ID, err)
	}
}

func (uc *handlePaymentWebhookUseCase) notifyRelease(inv *invoice.Invoice) {
	if uc.notifications == nil {
		return
	}
	_, err := uc.notifications.Publish(context.Background(), notification.Draft{
		WorkspaceID: inv.WorkspaceID,
		Category:    notification.CategoryBilling,
		Severity:    notification.SeveritySuccess,
		Title:       "Contestação encerrada a seu favor",
		Body:        "O valor de " + shared.FormatBRL(inv.AmountBRL) + " voltou para o saldo da carteira.",
		ActionURL:   balancePath,
		ActionLabel: "Ver saldo",
		Audience:    notification.Holding(workspace.ResourceBalance, workspace.ActionRead),
		Email:       true,
		DedupKey:    "payment_reversal_released:" + inv.ID,
		Source:      notification.SourcePaymentReversal,
	})
	if err != nil {
		log.Printf("[invoice-webhook] release notification for invoice %s failed: %v", inv.ID, err)
	}
}
