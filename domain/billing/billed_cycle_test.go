package billing

import (
	"testing"
	"time"
)

func brtAt(month time.Month, day, hour int) time.Time {
	return time.Date(2026, month, day, hour, 0, 0, 0, LocationBRT())
}

func TestAPeriodEndingOnOrBeforeTheDueDateIsBilledByThatInvoice(t *testing.T) {
	due := brtAt(time.March, 23, 0)
	if !DueOn(due, due) || !DueOn(brtAt(time.February, 23, 0), due) {
		t.Fatal("a period ending on the due date or earlier is billed by the invoice due that day")
	}
	if DueOn(brtAt(time.April, 23, 0), due) {
		t.Fatal("a period already paid past the due date is not billed again")
	}
}

func TestTheLatestIssuedDueIsTheCycleWhoseInvoiceWentOut(t *testing.T) {
	cases := map[string]struct {
		at   time.Time
		want time.Time
	}{
		"before the emit day pays the previous cycle": {brtAt(time.March, 10, 12), brtAt(time.February, 23, 0)},
		"on the emit day pays this month":             {brtAt(time.March, 18, 9), brtAt(time.March, 23, 0)},
		"after the due day still pays this month":     {brtAt(time.March, 26, 9), brtAt(time.March, 23, 0)},
		"january looks back to december":              {time.Date(2026, time.January, 5, 9, 0, 0, 0, LocationBRT()), time.Date(2025, time.December, 23, 0, 0, 0, 0, LocationBRT())},
	}
	for name, tc := range cases {
		if got := LatestIssuedDue(tc.at); !got.Equal(tc.want) {
			t.Errorf("%s: got %s, want %s", name, got, tc.want)
		}
	}
}

func TestAccessLastsUntilTheCutoffOfTheInvoiceThatRenewsThePeriod(t *testing.T) {
	end := brtAt(time.March, 23, 0)
	want := CancelCutoff(end, LocationBRT(), DefaultCancelDeadlineDay)
	if got := AccessEnd(end); !got.Equal(want) {
		t.Fatalf("access for a period ending Mar 23 runs to the Mar 27 cutoff, got %s", got)
	}
	late := brtAt(time.March, 28, 0)
	if got := AccessEnd(late); !got.Equal(late) {
		t.Fatalf("a period ending after the cutoff day keeps its own end, got %s", got)
	}
	if !AccessEnd(time.Time{}).Before(end) {
		t.Fatal("a missing period end grants no access")
	}
}
