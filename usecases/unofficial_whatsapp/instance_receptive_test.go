package unofficial_whatsapp

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/automationneeds"
	uw "vozko/domain/unofficial_whatsapp"
	"vozko/usecases/automationneeds/receptivetest"
)

func TestTheNumberRefusesAnAgentItsReceptiveCannotFeed(t *testing.T) {
	instances := newFakeInstanceRepo(&uw.Instance{ID: "inst-a", WorkspaceID: "ws-1", ServerID: "srv-1", Status: uw.StatusConnected, SendDelayMinMS: uw.MinSendDelayMS})
	guard := receptivetest.Refuse(&automationneeds.Refusal{Cause: automationneeds.ErrReceptiveUnmet})
	agentID := "agent-1"
	agentRef := &agentID
	on := true
	_, err := NewUpdateInstanceConfigUseCase(instances, guard).Execute(context.Background(), UpdateInstanceConfigInput{
		InstanceID: "inst-a", WorkspaceID: "ws-1", AgentID: &agentRef, EnableAgentResponses: &on,
	})
	if !errors.Is(err, automationneeds.ErrReceptiveUnmet) {
		t.Fatalf("err = %v", err)
	}
	if len(guard.Admitted) != 1 || !guard.Admitted[0].EnableAgentResponses {
		t.Fatalf("admitted = %+v", guard.Admitted)
	}
}

func TestASavedNumberSettlesItsNotice(t *testing.T) {
	instances := newFakeInstanceRepo(&uw.Instance{ID: "inst-a", WorkspaceID: "ws-1", ServerID: "srv-1", Status: uw.StatusConnected, SendDelayMinMS: uw.MinSendDelayMS})
	guard := receptivetest.Allow()
	name := "Loja"
	if _, err := NewUpdateInstanceConfigUseCase(instances, guard).Execute(context.Background(), UpdateInstanceConfigInput{InstanceID: "inst-a", WorkspaceID: "ws-1", DisplayName: &name}); err != nil {
		t.Fatal(err)
	}
	if len(guard.Settled) != 1 || guard.Settled[0] != "unofficial_whatsapp:inst-a" {
		t.Fatalf("settled = %v", guard.Settled)
	}
}
