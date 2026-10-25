package cron

import (
	"testing"
	"time"

	ttdomain "vozko/domain/tiktok"
)

func TestTheTokenRefreshRunsWellInsideTheRefreshLead(t *testing.T) {
	if ttdomain.TokenRefreshLead < 3*time.Hour {
		t.Fatalf("the hourly refresh needs a lead of several hours, got %v", ttdomain.TokenRefreshLead)
	}
	if period := catalogPeriod(t, "tiktok_token_refresh"); period != time.Hour {
		t.Fatalf("tiktok_token_refresh every %v, want hourly", period)
	}
}
