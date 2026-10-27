package campaignstamp

import (
	"strings"
	"testing"
)

func TestOutcomeMomentReadsTheFirstOutcomeStampThenTheDecisionMoment(t *testing.T) {
	got := OutcomeMoment("e")
	want := "COALESCE(LEAST(e.sent_at, e.failed_at), CASE WHEN e.status IN ('NOT_ELIGIBLE_POSSIBLE_SPAM', 'SKIPPED_NOT_ON_WHATSAPP')" +
		" OR (e.status = 'FAILED' AND e.error_code IN (920003, 920001, 920002, 920004, 920007, 920006, 920008)) THEN e.created_at END)"
	if got != want {
		t.Fatalf("OutcomeMoment(e) =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "?") {
		t.Fatal("GORM binds every question mark in Raw SQL, the expression must carry none")
	}
}
