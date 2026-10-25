package billing

import (
	"testing"
	"time"
)

func TestInvoicesAreEmittedFromTheEmitDayUntilTheDueDay(t *testing.T) {
	brt := LocationBRT()
	cases := map[int]bool{1: false, 17: false, 18: true, 20: true, 22: true, 23: false, 28: false}
	for day, want := range cases {
		at := time.Date(2026, 3, day, 12, 0, 0, 0, brt)
		if got := InEmitWindow(at); got != want {
			t.Errorf("day %d: InEmitWindow = %v, want %v", day, got, want)
		}
	}
}

func TestTheEmitWindowIsJudgedInBillingTime(t *testing.T) {
	lateOn17thBRT := time.Date(2026, 3, 18, 2, 0, 0, 0, time.UTC)
	if InEmitWindow(lateOn17thBRT) {
		t.Fatal("02:00 UTC on the 18th is still the 17th in Brasilia, before the window opens")
	}
}
