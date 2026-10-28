package billing

import "time"

type ConfirmMonthlyBillingUseCase interface {
	Execute(workspaceID string) error
}

type ConfirmBilledCycleUseCase interface {
	ExecuteForCycle(workspaceID string, billedDue time.Time) error
}

type EmitMonthlyInvoicesUseCase interface {
	Execute() (int, error)
}

type CancelSweepUseCase interface {
	Execute() (int, error)
}
