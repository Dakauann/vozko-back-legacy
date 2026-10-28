package inbox_assignment_usecase

import (
	"errors"
	"testing"

	ia "vozko/domain/inbox_assignment"
)

func TestALeadWithoutAConversationIsHandedOutByTheSameRoulette(t *testing.T) {
	repo := newStatefulRepo()
	svc := strategyService(repo, defaultEligible("ana", "bob"), strategyConfig(ia.StrategyRoundRobin))
	first, err := svc.PickForLead("ws-1", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.PickForLead("ws-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || (first != "ana" && first != "bob") || (second != "ana" && second != "bob") {
		t.Fatalf("picks %q then %q, want both members in turn", first, second)
	}
	if len(repo.assignments) != 0 {
		t.Fatalf("a lead pick must not assign any conversation: %+v", repo.assignments)
	}
}

func TestALeadPickWithNobodyAvailableRefuses(t *testing.T) {
	svc := strategyService(newStatefulRepo(), defaultEligible(), strategyConfig(ia.StrategyRoundRobin))
	if _, err := svc.PickForLead("ws-1", ""); !errors.Is(err, ErrNoLeadCandidate) {
		t.Fatalf("err = %v", err)
	}
}
