package carriercampaign

import (
	"encoding/json"
	"strings"
	"testing"

	"vozko/domain/campaign/scheduledstart"
	"vozko/domain/carriercampaign"
	ccuc "vozko/usecases/carriercampaign"
)

func TestTheFailedScheduleReasonIsShownOnTheCampaignAndTheList(t *testing.T) {
	failed := &carriercampaign.Campaign{ID: "c1", ScheduleFailure: scheduledstart.FailureStartFailed}
	detail, _ := json.Marshal(campaignDTO(failed))
	row, _ := json.Marshal(summaryDTO(ccuc.CampaignSummary{Campaign: *failed}))
	for name, raw := range map[string][]byte{"detail": detail, "list": row} {
		if !strings.Contains(string(raw), `"schedule_failure":"schedule_start_failed"`) {
			t.Fatalf("%s: %s", name, raw)
		}
	}
	clean, _ := json.Marshal(campaignDTO(&carriercampaign.Campaign{ID: "c2"}))
	if strings.Contains(string(clean), "schedule_failure") {
		t.Fatalf("%s", clean)
	}
}
