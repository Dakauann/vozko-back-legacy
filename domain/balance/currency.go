package balance

const LedgerCurrency = "USD"

const BillingCurrency = "BRL"

type ReferenceTotals struct {
	LedgerMicros  int64
	BillingMicros int64
	Transactions  int
	Debits        int
	MissingRate   bool

	GenerationBillingMicros int64
}

type ReferenceTotaler interface {
	TotalsUnder(workspaceID, referencePrefix string) (ReferenceTotals, error)
}
