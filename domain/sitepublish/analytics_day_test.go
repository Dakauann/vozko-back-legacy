package sitepublish

import (
	"testing"
	"time"
)

func TestAnalyticsDaysAreSaoPauloDays(t *testing.T) {
	late := time.Date(2026, 10, 29, 2, 59, 0, 0, time.UTC)
	if got := AnalyticsDay(late); !got.Equal(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %s", got)
	}
	if got := AnalyticsDayStart(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)); !got.Equal(time.Date(2026, 10, 28, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %s", got)
	}
}
