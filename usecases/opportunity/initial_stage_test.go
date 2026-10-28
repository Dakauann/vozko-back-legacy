package opportunity_usecase

import (
	"errors"
	"testing"
)

func TestTheInitialStageOfADealFunnelIsItsOpeningStage(t *testing.T) {
	svc := newAutomationService(newFakeOppRepo())
	got, err := svc.InitialStage("ws1", "pipe1")
	if err != nil || got != "stage-new" {
		t.Fatalf("InitialStage = %q, %v", got, err)
	}
	if _, err := svc.InitialStage("ws1", "pipe-conv"); !errors.Is(err, ErrNotOpportunityPipeline) {
		t.Fatalf("a conversation funnel = %v", err)
	}
}
