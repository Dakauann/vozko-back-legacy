package inbox_assignment_usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/shared"
)

type scriptedVariablesCheck struct {
	refuse  map[string]bool
	checked []string
	pending []map[string]string
}

func (s *scriptedVariablesCheck) ConversationMissingWith(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType, target conversation.Automation, pending map[string]string) error {
	s.pending = append(s.pending, pending)
	if s.refuse[target.ID] && pending["turma"] != "" {
		s.checked = append(s.checked, entryID+"/"+target.ID)
		return nil
	}
	return s.ConversationMissing(ctx, workspaceID, entryID, entryType, target)
}

func (s *scriptedVariablesCheck) ConversationMissing(_ context.Context, _, entryID string, _ shared.EntryType, target conversation.Automation) error {
	s.checked = append(s.checked, entryID+"/"+target.ID)
	if s.refuse[target.ID] {
		return &automationneeds.Refusal{Cause: automationneeds.ErrConversationMissing, Needs: automationneeds.Needs{{Name: "turma"}}}
	}
	return nil
}

func TestDelegatingToAnAutomationTheConversationCannotFeedChangesNothing(t *testing.T) {
	f := newAIFixture(noAutomation)
	f.seed("entry-1", "bob")
	delegation, store, _ := newDelegation(f)
	check := &scriptedVariablesCheck{refuse: map[string]bool{"wf-9": true}}
	delegation.variables = check
	runs := &delegationRuns{}
	delegation.runs = runs
	_, err := delegation.Delegate(context.Background(), delegateInput(conversation.AutomationWorkflow, "wf-9"))
	if !errors.Is(err, automationneeds.ErrConversationMissing) {
		t.Fatalf("err = %v", err)
	}
	assert.Empty(t, store.saved)
	assert.Empty(t, runs.stopped, "running workflows are left alone when the delegation is refused")
	assert.Equal(t, "bob", f.owner("entry-1"))
	assert.Equal(t, []string{"entry-1/wf-9"}, check.checked)
}

func TestASystemDelegationIsRefusedTheSameWayAndKeepsTheOwner(t *testing.T) {
	f := newAIFixture(noAutomation)
	f.seed("entry-1", "bob")
	delegation, store, _ := newDelegation(f)
	delegation.variables = &scriptedVariablesCheck{refuse: map[string]bool{"agent-9": true}}
	err := delegation.DelegateAsSystem(context.Background(), SystemDelegation{WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: shared.EntryTypeWhatsApp, Automation: conversation.Automation{Kind: conversation.AutomationAgent, ID: "agent-9"}})
	if !errors.Is(err, automationneeds.ErrConversationMissing) {
		t.Fatalf("err = %v", err)
	}
	assert.Empty(t, store.saved)
	assert.Equal(t, "bob", f.owner("entry-1"))
}

func TestSwitchingAutomationBackOnChecksTheGoverningAutomationFirst(t *testing.T) {
	f := newAIFixture(agentGoverned)
	f.seed("entry-1", "bob")
	toggle, _ := newToggle(f)
	toggle.variables = &scriptedVariablesCheck{refuse: map[string]bool{"agent-1": true}}
	_, err := toggle.SetAutomation(context.Background(), toggleInput(pauseOn()))
	if !errors.Is(err, automationneeds.ErrConversationMissing) {
		t.Fatalf("err = %v", err)
	}
	assert.Equal(t, "bob", f.owner("entry-1"), "the person keeps the conversation")
}

func TestSwitchingAPausedConversationBackOnChecksTheAutomationItWillHandTo(t *testing.T) {
	paused := false
	profile := agentGoverned
	profile.AutomationEnabled = &paused
	f := newAIFixture(profile)
	f.seed("entry-1", "bob")
	toggle, _ := newToggle(f)
	check := &scriptedVariablesCheck{refuse: map[string]bool{"agent-1": true}}
	toggle.variables = check
	_, err := toggle.SetAutomation(context.Background(), toggleInput(pauseOn()))
	if !errors.Is(err, automationneeds.ErrConversationMissing) {
		t.Fatalf("err = %v", err)
	}
	assert.Equal(t, []string{"entry-1/agent-1"}, check.checked)
	assert.Empty(t, f.pauser.paused, "automation stays paused")
	assert.Equal(t, "bob", f.owner("entry-1"))
}

func TestSwitchingAutomationBackOnFailsClosedWhenTheConversationCannotBeRead(t *testing.T) {
	f := newAIFixture(agentGoverned)
	f.seed("entry-1", "bob")
	f.profiles.err = errors.New("db down")
	toggle, _ := newToggle(f)
	_, err := toggle.SetAutomation(context.Background(), toggleInput(pauseOn()))
	if !errors.Is(err, automationneeds.ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
	assert.Equal(t, "bob", f.owner("entry-1"))
}
