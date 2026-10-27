package instagram

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/automationneeds"
	igdomain "vozko/domain/instagram"
	"vozko/usecases/automationneeds/receptivetest"
)

func TestTheAccountRefusesAnAgentItsReceptiveCannotFeed(t *testing.T) {
	repo := &fakeAccountRepo{FindByIDFn: func(context.Context, string) (*igdomain.Account, error) {
		return &igdomain.Account{ID: "ig-1", WorkspaceID: "ws-1", IGUserID: "u-1", Status: igdomain.StatusConnected}, nil
	}}
	guard := receptivetest.Refuse(&automationneeds.Refusal{Cause: automationneeds.ErrReceptiveUnmet})
	agentID := "agent-1"
	on := true
	_, err := NewUpdateAccountConfigUseCase(repo, guard).Execute(context.Background(), UpdateAccountConfigInput{ID: "ig-1", WorkspaceID: "ws-1", AgentID: &agentID, EnableAgentResponses: &on})
	if !errors.Is(err, automationneeds.ErrReceptiveUnmet) {
		t.Fatalf("err = %v", err)
	}
	ok := receptivetest.Allow()
	if _, err := NewUpdateAccountConfigUseCase(repo, ok).Execute(context.Background(), UpdateAccountConfigInput{ID: "ig-1", WorkspaceID: "ws-1"}); err != nil {
		t.Fatal(err)
	}
	if len(ok.Settled) != 1 || ok.Settled[0] != "instagram:ig-1" {
		t.Fatalf("settled = %v", ok.Settled)
	}
}
