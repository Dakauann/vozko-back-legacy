package inbox_assignment_usecase

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vozko/domain/conversation"
	ia "vozko/domain/inbox_assignment"
	wsc "vozko/domain/workspace_config"
)

type recordingAlerts struct {
	alerted []string
}

func (r *recordingAlerts) AlertAssignee(workspaceID, entryID, _, userID string) {
	r.alerted = append(r.alerted, workspaceID+"|"+entryID+"|"+userID)
}

type failingFind struct {
	*statefulRepo
	err error
}

func (f failingFind) FindByEntry(string, string, string) (*ia.InboxAssignment, error) {
	return nil, f.err
}

func claimConfig(skipAdmins bool) *rouletteConfig {
	return &rouletteConfig{cfg: &wsc.WorkspaceConfig{RouletteStrategy: ia.StrategyClaim, SkipAdminAssignment: skipAdmins}}
}

func attentionService(repo ia.Repository, eligible *mockEligible, resolver *mockResolver, cfg WorkspaceConfigProvider) *AssignmentService {
	return newService(repo, eligible, resolver, cfg)
}

func TestAMessageOnAnOwnedConversationNamesItsOwner(t *testing.T) {
	repo := newStatefulRepo()
	_ = repo.Assign(&ia.InboxAssignment{WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: "whatsapp", AssignedUserID: "ana"})
	svc := attentionService(repo, defaultEligible("ana", "bob"), defaultResolver("ws-1", ""), claimConfig(false))

	got := svc.MessageAttention("ws-1", "entry-1", "whatsapp", "")

	assert.Equal(t, "ana", got.Owner)
	assert.True(t, got.Alerts("ana"))
	assert.False(t, got.Alerts("bob"))
}

func TestAMessageOnAnAgentHeldConversationAlertsNobody(t *testing.T) {
	repo := newStatefulRepo()
	_ = repo.Assign(&ia.InboxAssignment{WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: "whatsapp", AssignedUserID: "ai:agent-1"})
	svc := attentionService(repo, defaultEligible("ana"), defaultResolver("ws-1", ""), claimConfig(false))

	got := svc.MessageAttention("ws-1", "entry-1", "whatsapp", "")

	assert.False(t, got.Alerts("ana"))
}

func TestAnUnreadableOwnerFallsBackToTheOwnerTheEntryNamed(t *testing.T) {
	repo := failingFind{statefulRepo: newStatefulRepo(), err: errors.New("db down")}
	svc := attentionService(repo, defaultEligible("ana", "bob"), defaultResolver("ws-1", ""), claimConfig(false))

	got := svc.MessageAttention("ws-1", "entry-1", "whatsapp", "ana")

	assert.Equal(t, conversation.MessageAttention{Owner: "ana", Distribution: conversation.DistributionUnknown}, got)
	assert.False(t, svc.MessageAttention("ws-1", "entry-1", "whatsapp", "").Alerts("bob"))
}

func TestAnUnownedConversationTheRouletteDealsAlertsNobody(t *testing.T) {
	cfg := &rouletteConfig{cfg: &wsc.WorkspaceConfig{RouletteStrategy: ia.StrategyRoundRobin}}
	svc := attentionService(newStatefulRepo(), defaultEligible("ana"), defaultResolver("ws-1", ""), cfg)

	got := svc.MessageAttention("ws-1", "entry-1", "whatsapp", "")

	assert.Equal(t, conversation.DistributionAutomatic, got.Distribution)
	assert.False(t, got.Alerts("ana"))
}

func TestAWorkspaceWithoutSettingsDealsByTheDefaultRoulette(t *testing.T) {
	svc := attentionService(newStatefulRepo(), defaultEligible("ana"), defaultResolver("ws-1", ""), &rouletteConfig{})

	assert.Equal(t, conversation.DistributionAutomatic, svc.MessageAttention("ws-1", "entry-1", "whatsapp", "").Distribution)
}

func TestAnUnownedConversationInTheManualQueueAlertsTheDepartmentsTakers(t *testing.T) {
	eligible := deptEligible([]string{"ana", "admin"}, "ws-1:dept-sales", []string{"carla"})
	svc := attentionService(newStatefulRepo(), eligible, defaultResolver("ws-1", "dept-sales"), claimConfig(true))

	got := svc.MessageAttention("ws-1", "entry-1", "whatsapp", "")

	assert.Equal(t, conversation.DistributionManual, got.Distribution)
	assert.True(t, got.Alerts("carla"))
	assert.False(t, got.Alerts("admin"), "an admin outside the department cannot take it")
	require.NotEmpty(t, eligible.departmentCalls)
	assert.Equal(t, eligibleDeptCall{workspaceID: "ws-1", departmentID: "dept-sales", skipAdmins: true}, eligible.departmentCalls[0],
		"who can take it follows the same rule as the roulette, admins setting included")
}

func TestAnUnownedConversationWithoutADepartmentAlertsTheWorkspaceTakers(t *testing.T) {
	eligible := defaultEligible("ana", "bob")
	svc := attentionService(newStatefulRepo(), eligible, defaultResolver("ws-1", ""), claimConfig(false))

	got := svc.MessageAttention("ws-1", "entry-1", "whatsapp", "")

	assert.True(t, got.Alerts("ana"))
	assert.True(t, got.Alerts("bob"))
	assert.False(t, got.Alerts("zoe"))
}

func TestAnUnownedConversationAnAutomationWillTakeAlertsNobody(t *testing.T) {
	svc := attentionService(newStatefulRepo(), defaultEligible("ana"), defaultResolver("ws-1", ""), claimConfig(false))
	svc.automationProfiles = &stubAutomationProfiles{profile: agentGoverned}

	got := svc.MessageAttention("ws-1", "entry-1", "whatsapp", "")

	assert.Equal(t, conversation.DistributionAutomatic, got.Distribution)
	assert.False(t, got.Alerts("ana"))
}

func TestUnreadableInputsAlertOnlyTheOwner(t *testing.T) {
	cases := map[string]func() *AssignmentService{
		"settings": func() *AssignmentService {
			return attentionService(newStatefulRepo(), defaultEligible("ana"), defaultResolver("ws-1", ""), &rouletteConfig{err: errors.New("db down")})
		},
		"department": func() *AssignmentService {
			return attentionService(newStatefulRepo(), defaultEligible("ana"), &mockResolver{workspaceID: "ws-1", departmentErr: errors.New("db down")}, claimConfig(false))
		},
		"governance": func() *AssignmentService {
			svc := attentionService(newStatefulRepo(), defaultEligible("ana"), defaultResolver("ws-1", ""), claimConfig(false))
			svc.automationProfiles = &stubAutomationProfiles{err: errors.New("db down")}
			return svc
		},
		"workspace": func() *AssignmentService {
			return attentionService(newStatefulRepo(), defaultEligible("ana"), defaultResolver("", ""), claimConfig(false))
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			workspaceID := "ws-1"
			if name == "workspace" {
				workspaceID = ""
			}
			got := build().MessageAttention(workspaceID, "entry-1", "whatsapp", "")

			assert.Equal(t, conversation.DistributionUnknown, got.Distribution)
			assert.False(t, got.Alerts("ana"))
		})
	}
}

func TestTheRouletteAlertsThePersonItPicksOnce(t *testing.T) {
	svc := newService(newStatefulRepo(), defaultEligible("ana"), defaultResolver("ws-1", ""), defaultConfig())
	alerts := &recordingAlerts{}
	svc.assigneeAlerts = alerts

	svc.EnsureAssignment("entry-1", "whatsapp", "phone-1")
	svc.EnsureAssignment("entry-1", "whatsapp", "phone-1")

	assert.Equal(t, []string{"ws-1|entry-1|ana"}, alerts.alerted)
}

func TestAManualAssignAlertsTheNewOwnerOnce(t *testing.T) {
	svc := newService(newStatefulRepo(), defaultEligible("ana", "bob"), defaultResolver("ws-1", ""), defaultConfig())
	alerts := &recordingAlerts{}
	svc.assigneeAlerts = alerts

	require.NoError(t, svc.AssignManual("entry-1", "whatsapp", "phone-1", "ws-1", "bob", "ana", ia.TriggerManual))
	require.NoError(t, svc.AssignManual("entry-1", "whatsapp", "phone-1", "ws-1", "bob", "ana", ia.TriggerManual))

	assert.Equal(t, []string{"ws-1|entry-1|bob"}, alerts.alerted)
}

func TestATransferToAColleagueAlertsTheColleagueOnce(t *testing.T) {
	f := newAIFixture(conversation.AutomationProfile{}, "ana", "bob")
	alerts := &recordingAlerts{}
	f.svc.assigneeAlerts = alerts
	f.seed("entry-1", "ana")

	require.NoError(t, f.svc.AssignManual("entry-1", "whatsapp", "phone-1", "ws-1", "bob", "ana", ia.TriggerManual))

	assert.Equal(t, []string{"ws-1|entry-1|bob"}, alerts.alerted)
}

func TestATransferToADepartmentAlertsWhoItsRouletteDrawsOnce(t *testing.T) {
	f := newAIFixture(workflowGoverned, "ana")
	alerts := &recordingAlerts{}
	f.svc.assigneeAlerts = alerts
	f.eligible.departmentUsers["ws-1:dept-sales"] = []string{"carla", "davi"}
	f.seed("entry-1", "workflow:wf-1")

	_, err := f.svc.HandOffToRoulette(departmentHandOff("dept-sales"))

	require.NoError(t, err)
	assert.Equal(t, []string{"ws-1|entry-1|carla"}, alerts.alerted)
}

func TestAnAgentHandOffAlertsThePersonOnce(t *testing.T) {
	f := newAIFixture(agentGoverned, "ana")
	alerts := &recordingAlerts{}
	f.svc.assigneeAlerts = alerts
	f.seed("entry-1", "ai:agent-1")

	require.NoError(t, f.svc.HandOffToHuman("ws-1", "entry-1", "whatsapp", "bob"))

	assert.Equal(t, []string{"ws-1|entry-1|bob"}, alerts.alerted)
}

func TestTakingAConversationYourselfRaisesNoAlert(t *testing.T) {
	f := newAIFixture(agentGoverned, "ana")
	alerts := &recordingAlerts{}
	f.svc.assigneeAlerts = alerts

	require.NoError(t, f.svc.Reassign("entry-1", "whatsapp", "phone-1", "ws-1", "ana"))
	claimed, err := f.svc.ClaimIfUnassigned("entry-2", "whatsapp", "phone-1", "ws-1", "bob", ia.TriggerOpen)
	require.NoError(t, err)
	require.True(t, claimed)
	f.seed("entry-3", "ai:agent-1")
	owner, err := f.svc.TakeOverFromAutomation("entry-3", "whatsapp", "ana")
	require.NoError(t, err)
	require.Equal(t, "ana", owner)

	assert.Empty(t, alerts.alerted)
}

func TestAConversationGivenToAnAutomationRaisesNoAlert(t *testing.T) {
	f := newAIFixture(agentGoverned, "ana")
	alerts := &recordingAlerts{}
	f.svc.assigneeAlerts = alerts

	f.svc.EnsureAssignment("entry-1", "whatsapp", "phone-1")
	require.NoError(t, f.svc.UnassignSystem("entry-1", "whatsapp", "ws-1", ia.TriggerAutomationReleased))

	assert.Empty(t, alerts.alerted)
}
