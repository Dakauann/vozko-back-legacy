package telegram

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/automationneeds"
	tgdomain "vozko/domain/telegram"
	"vozko/usecases/automationneeds/receptivetest"
)

func TestTheBotRefusesAWorkflowItsReceptiveCannotFeed(t *testing.T) {
	repo := &fakeAccounts{FindByIDFn: func(context.Context, string) (*tgdomain.Account, error) {
		return &tgdomain.Account{ID: "tg-1", WorkspaceID: "ws-1", BotUserID: 7, Mode: tgdomain.ModeBot, Status: tgdomain.StatusActive}, nil
	}}
	guard := receptivetest.Refuse(&automationneeds.Refusal{Cause: automationneeds.ErrReceptiveUnmet})
	workflowID := "wf-1"
	on := true
	_, err := NewUpdateAccountConfigUseCase(repo, guard).Execute(context.Background(), "ws-1", "tg-1", UpdateAccountConfigInput{WorkflowID: &workflowID, EnableWorkflow: &on})
	if !errors.Is(err, automationneeds.ErrReceptiveUnmet) {
		t.Fatalf("err = %v", err)
	}
	if len(repo.Updated) != 0 {
		t.Fatal("a refused bot must not be written")
	}
}
