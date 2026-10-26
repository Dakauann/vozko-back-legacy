package inbox_assignment_usecase

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aa "vozko/domain/ai_attendance"
	"vozko/domain/conversation"
	ce "vozko/domain/conversation_event"
	ia "vozko/domain/inbox_assignment"
)

type failingAssign struct {
	*statefulRepo
	err error
}

func (f failingAssign) Assign(*ia.InboxAssignment) error { return f.err }

func (f failingAssign) AssignIfUnassigned(*ia.InboxAssignment) (bool, error) { return false, f.err }

func anaStarts() ia.ConversationStart {
	return ia.ConversationStart{WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: "whatsapp", AccountID: "phone-1", StarterID: "ana"}
}

func startFixture(profile conversation.AutomationProfile) (*aiFixture, *recordingAlerts) {
	f := newAIFixture(profile, "ana", "bob")
	alerts := &recordingAlerts{}
	f.svc.assigneeAlerts = alerts
	return f, alerts
}

func TestHoldingAStartGivesTheStarterTheConversationAndStopsTheAutomationBeforeAnythingIsSent(t *testing.T) {
	f, _ := startFixture(agentGoverned)

	hold, err := f.svc.HoldForStarter(anaStarts())

	require.NoError(t, err)
	assert.Equal(t, "ana", f.owner("entry-1"))
	assert.Equal(t, []string{"whatsapp:entry-1"}, f.pauser.paused, "the number's agent must not answer the reply")
	assert.Empty(t, f.history.appended, "nothing is announced until the first message went out")
	assert.Empty(t, f.broadcast.owners)
	assert.True(t, hold.Ownership.Takes)
}

func TestTheReplyToAHeldStartStaysWithTheStarter(t *testing.T) {
	f, _ := startFixture(agentGoverned)
	_, err := f.svc.HoldForStarter(anaStarts())
	require.NoError(t, err)

	assert.Equal(t, "ana", f.svc.EnsureAssignment("entry-1", "whatsapp", "phone-1"), "the automation must not take it when the contact replies")
}

func TestConfirmingAStartRecordsTheStarterWithoutAlertingThem(t *testing.T) {
	f, alerts := startFixture(agentGoverned)
	hold, err := f.svc.HoldForStarter(anaStarts())
	require.NoError(t, err)

	f.svc.ConfirmStart(hold)

	h := f.lastHistory(t)
	assert.Equal(t, ia.TriggerOutreachSent, h.Trigger)
	assert.Equal(t, "ana", h.AssignedActorID)
	assert.Equal(t, "ana", h.AssignedByActorID)
	require.Len(t, f.events.logged, 1)
	assert.Equal(t, ce.EventAutoAssigned, f.events.logged[0].EventType)
	assert.Empty(t, alerts.alerted, "a self-start alerts nobody")
	assert.Equal(t, []string{"entry-1 from "}, f.broadcast.owners)
	assert.Empty(t, f.sessions.ended, "no automation held it, so no session ends")
}

func TestAStartTakesTheConversationFromTheNumbersAgentAndEndsItsSession(t *testing.T) {
	f, alerts := startFixture(agentGoverned)
	f.seed("entry-1", "ai:agent-1")

	hold, err := f.svc.HoldForStarter(anaStarts())
	require.NoError(t, err)
	f.svc.ConfirmStart(hold)

	assert.Equal(t, "ana", f.owner("entry-1"))
	assert.Equal(t, []string{"whatsapp:entry-1"}, f.pauser.paused)
	assert.Equal(t, []string{"ws-1|entry-1|" + string(aa.OutcomeHandedOff) + "|" + aa.EndReasonManualAssignment + "|ana"}, f.sessions.ended)
	assert.Equal(t, []string{"ana"}, f.sessions.by)
	assert.Equal(t, "ai:agent-1", f.lastHistory(t).PreviousActorID)
	assert.Equal(t, []string{"entry-1 from ai:agent-1"}, f.broadcast.owners)
	assert.Empty(t, alerts.alerted)
}

func TestAStartLeavesAColleagueTheirConversationAndTheirAutomationSwitch(t *testing.T) {
	f, alerts := startFixture(agentGoverned)
	f.seed("entry-1", "bob")

	hold, err := f.svc.HoldForStarter(anaStarts())
	require.NoError(t, err)
	f.svc.ConfirmStart(hold)

	assert.Equal(t, "bob", f.owner("entry-1"))
	assert.Empty(t, f.pauser.paused, "a person owns it, so the automation already stays out")
	assert.Empty(t, f.history.appended)
	assert.Empty(t, f.broadcast.owners)
	assert.Empty(t, alerts.alerted, "bob is not alerted for a colleague's start")
}

func TestAStartKeepsTheDelegationThePersonChose(t *testing.T) {
	profile := agentGoverned
	profile.Delegate = &conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "wf-1"}
	f, _ := startFixture(profile)
	f.seed("entry-1", "workflow:wf-1")

	hold, err := f.svc.HoldForStarter(anaStarts())
	require.NoError(t, err)
	f.svc.ConfirmStart(hold)

	assert.Equal(t, "workflow:wf-1", f.owner("entry-1"))
	assert.Empty(t, f.pauser.paused, "pausing would drop the delegation")
	assert.Empty(t, f.history.appended)
}

func TestAnAlreadyPausedConversationIsNotPausedAgain(t *testing.T) {
	paused := false
	f, _ := startFixture(conversation.AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true, AutomationEnabled: &paused})

	_, err := f.svc.HoldForStarter(anaStarts())

	require.NoError(t, err)
	assert.Equal(t, "ana", f.owner("entry-1"))
	assert.Empty(t, f.pauser.paused)
}

func TestAStartThatCannotReadTheAutomationFailsClosed(t *testing.T) {
	f, _ := startFixture(agentGoverned)
	f.profiles.err = errors.New("db down")

	_, err := f.svc.HoldForStarter(anaStarts())

	require.ErrorIs(t, err, ia.ErrStarterNotAssigned)
	assert.Equal(t, "", f.owner("entry-1"))
	assert.Empty(t, f.pauser.paused)
}

func TestAStartThatCannotReadTheOwnerFailsClosed(t *testing.T) {
	f, _ := startFixture(agentGoverned)
	f.svc.repo = failingFind{statefulRepo: f.repo, err: errors.New("db down")}

	_, err := f.svc.HoldForStarter(anaStarts())

	require.ErrorIs(t, err, ia.ErrStarterNotAssigned)
	assert.Empty(t, f.pauser.paused)
}

func TestAStartThatCannotAssignFailsClosed(t *testing.T) {
	f, _ := startFixture(agentGoverned)
	f.svc.repo = failingAssign{statefulRepo: f.repo, err: errors.New("db down")}

	_, err := f.svc.HoldForStarter(anaStarts())

	require.ErrorIs(t, err, ia.ErrStarterNotAssigned)
	assert.Empty(t, f.pauser.paused, "nothing may be paused for a conversation the starter did not get")
}

func TestAStartThatCannotPauseGivesTheConversationBack(t *testing.T) {
	f, _ := startFixture(agentGoverned)
	f.seed("entry-1", "ai:agent-1")
	f.pauser.err = errors.New("db down")

	_, err := f.svc.HoldForStarter(anaStarts())

	require.ErrorIs(t, err, ia.ErrStarterNotAssigned)
	assert.Equal(t, "ai:agent-1", f.owner("entry-1"))
	assert.Empty(t, f.history.appended)
}

func TestReleasingAStartPutsTheConversationBackAsItWas(t *testing.T) {
	f, _ := startFixture(agentGoverned)
	f.seed("entry-1", "ai:agent-1")
	hold, err := f.svc.HoldForStarter(anaStarts())
	require.NoError(t, err)

	require.NoError(t, f.svc.ReleaseStart(hold))

	assert.Equal(t, "ai:agent-1", f.owner("entry-1"))
	assert.Equal(t, []string{"whatsapp:entry-1", "resume:entry-1"}, f.pauser.paused)
	assert.Empty(t, f.history.appended)
	assert.Empty(t, f.sessions.ended)
}

func TestReleasingAStartOnAFreeConversationFreesItAgain(t *testing.T) {
	f, _ := startFixture(agentGoverned)
	hold, err := f.svc.HoldForStarter(anaStarts())
	require.NoError(t, err)

	require.NoError(t, f.svc.ReleaseStart(hold))

	assert.Equal(t, "", f.owner("entry-1"))
}

func TestClaimingForTheStarterHoldsAndConfirmsAtOnce(t *testing.T) {
	f, alerts := startFixture(agentGoverned)

	require.NoError(t, f.svc.ClaimForStarter(anaStarts()))

	assert.Equal(t, "ana", f.owner("entry-1"))
	assert.Equal(t, []string{"whatsapp:entry-1"}, f.pauser.paused)
	assert.Equal(t, ia.TriggerOutreachSent, f.lastHistory(t).Trigger)
	assert.Empty(t, alerts.alerted)
}

func TestClaimingForTheStarterFailsClosed(t *testing.T) {
	f, _ := startFixture(agentGoverned)
	f.pauser.err = errors.New("db down")

	require.ErrorIs(t, f.svc.ClaimForStarter(anaStarts()), ia.ErrStarterNotAssigned)
	assert.Equal(t, "", f.owner("entry-1"))
}

func TestAStartWithoutAPersonChangesNothing(t *testing.T) {
	for _, starter := range []string{"", "system", "ai:agent-1"} {
		f, _ := startFixture(agentGoverned)
		in := anaStarts()
		in.StarterID = starter

		require.NoError(t, f.svc.ClaimForStarter(in))

		assert.Equal(t, "", f.owner("entry-1"), starter)
		assert.Empty(t, f.pauser.paused, starter)
		assert.Equal(t, 0, f.profiles.calls, starter)
	}
}
