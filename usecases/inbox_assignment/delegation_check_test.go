package inbox_assignment_usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

func systemDelegationTo(kind conversation.AutomationKind, id string) SystemDelegation {
	return SystemDelegation{WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: shared.EntryTypeWhatsApp, Automation: conversation.Automation{Kind: kind, ID: id}}
}

func TestCheckingADelegationAnswersWithoutTouchingTheConversation(t *testing.T) {
	f := newAIFixture(noAutomation)
	f.seed("entry-1", "bob")
	delegation, store, _ := newDelegation(f)
	runs := &delegationRuns{}
	delegation.runs = runs

	require.NoError(t, delegation.CheckDelegation(context.Background(), systemDelegationTo(conversation.AutomationAgent, "agent-9")))

	assert.Empty(t, store.saved)
	assert.Empty(t, runs.stopped)
	assert.Equal(t, "bob", f.owner("entry-1"))
}

func TestCheckingADelegationRefusesWhatTheDelegationItselfRefuses(t *testing.T) {
	f := newAIFixture(noAutomation)
	f.seed("entry-1", "bob")
	delegation, _, _ := newDelegation(f)
	delegation.variables = &scriptedVariablesCheck{refuse: map[string]bool{"wf-9": true}}

	assert.ErrorIs(t, delegation.CheckDelegation(context.Background(), systemDelegationTo(conversation.AutomationAgent, "off")), conversation.ErrAutomationUnusable)
	assert.ErrorIs(t, delegation.CheckDelegation(context.Background(), systemDelegationTo(conversation.AutomationWorkflow, "ivr")), conversation.ErrAutomationUnusable)
	refusal, unmet := automationneeds.RefusalOf(delegation.CheckDelegation(context.Background(), systemDelegationTo(conversation.AutomationWorkflow, "wf-9")))
	require.True(t, unmet)
	assert.Equal(t, []string{"turma"}, refusal.Needs.Names())

	delegation.variables = nil
	assert.ErrorIs(t, delegation.CheckDelegation(context.Background(), systemDelegationTo(conversation.AutomationAgent, "agent-9")), automationneeds.ErrUnreadable)
}

func TestAnUnusableWorkflowSaysWhyItCannotTakeTheConversation(t *testing.T) {
	f := newAIFixture(noAutomation)
	delegation, _, _ := newDelegation(f)

	err := delegation.directory.Usable("ws-1", conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "draft"})

	assert.ErrorIs(t, err, conversation.ErrAutomationUnusable)
	assert.ErrorIs(t, err, workflow.ErrWorkflowNotPublished)
}

func TestCheckingADelegationCountsTheValuesAboutToBeStored(t *testing.T) {
	f := newAIFixture(noAutomation)
	f.seed("entry-1", "bob")
	delegation, _, _ := newDelegation(f)
	check := &scriptedVariablesCheck{refuse: map[string]bool{"wf-9": true}}
	delegation.variables = check
	in := systemDelegationTo(conversation.AutomationWorkflow, "wf-9")
	in.Pending = map[string]string{"turma": "Noite"}

	require.NoError(t, delegation.CheckDelegation(context.Background(), in))
	assert.Equal(t, []map[string]string{{"turma": "Noite"}}, check.pending)
}
