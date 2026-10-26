package servicemessage

import (
	"errors"
	"testing"

	"vozko/domain/conversation"
)

func TestTheReportedPriceIsTheOneTheChargeUses(t *testing.T) {
	pricer := &fakePricer{price: 45_000, cost: 30_000}
	b, err := NewBilling(Deps{Pricer: pricer, Ledger: newFakeLedger(), BalanceChecker: &fakeChecker{}, Unattributed: &fakeUnattributed{}})
	if err != nil {
		t.Fatal(err)
	}
	price, err := b.ServiceMessagePrice("ws1")
	if err != nil || price.PriceMicros != 45_000 {
		t.Fatalf("price = %+v err = %v, want the 45000 the charge would debit", price, err)
	}
	if pricer.category != conversation.MetaPricingCategoryService {
		t.Fatalf("priced category %q, want the service line the charge prices", pricer.category)
	}
}

func TestAPriceThatCannotBeResolvedIsReportedAsAnError(t *testing.T) {
	b, _ := NewBilling(Deps{Pricer: &fakePricer{err: errors.New("plan unavailable")}, Ledger: newFakeLedger(), BalanceChecker: &fakeChecker{}, Unattributed: &fakeUnattributed{}})
	if _, err := b.ServiceMessagePrice("ws1"); err == nil {
		t.Fatalf("an unresolvable price came back as a value")
	}
}

func TestReadingThePriceChargesNothing(t *testing.T) {
	ledger := newFakeLedger()
	b, _ := NewBilling(Deps{Pricer: &fakePricer{price: 45_000}, Ledger: ledger, BalanceChecker: &fakeChecker{}, Unattributed: &fakeUnattributed{}})
	_, _ = b.ServiceMessagePrice("ws1")
	if len(ledger.debits) != 0 {
		t.Fatalf("reading the price wrote %d ledger entries", len(ledger.debits))
	}
}
