package ai

const (
	ContextBudgetCapTokens  = 200_000
	ContextReserveMinTokens = 16_000
	PruneProtectTokens      = 40_000
	PruneMinimumTokens      = 20_000
	contextReserveDivisor   = 10
)

func (m ModelInfo) ContextCeiling() (int, bool) {
	window := int(m.ContextLength)
	if window <= 0 {
		return 0, false
	}
	reserve := window / contextReserveDivisor
	if window >= 2*ContextReserveMinTokens {
		reserve = max(reserve, ContextReserveMinTokens)
	}
	return window - reserve, true
}

func (m ModelInfo) ContextBudget() (int, bool) {
	ceiling, known := m.ContextCeiling()
	if !known {
		return 0, false
	}
	return min(ceiling, ContextBudgetCapTokens), true
}
