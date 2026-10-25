package inbox_assignment_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	aa "vozko/domain/ai_attendance"
	"vozko/domain/conversation"
	ce "vozko/domain/conversation_event"
	"vozko/domain/crm_telemetry"
	ia "vozko/domain/inbox_assignment"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	wd "vozko/domain/workspace/workspace_department"
	wsc "vozko/domain/workspace_config"
)

type quietTelemetry struct{}

func (quietTelemetry) Publish(crm_telemetry.Kind, any) error { return nil }

type historyTelemetry struct{ store ia.IntervalWriter }

func (t historyTelemetry) Publish(kind crm_telemetry.Kind, payload any) error {
	p, ok := payload.(crm_telemetry.AssignmentHistoryPayload)
	if kind != crm_telemetry.KindAssignmentHistory || !ok {
		return nil
	}
	return ia.RecordInterval(t.store, p.Interval())
}

type quietEvents struct{}

func (quietEvents) Log(*ce.ConversationEvent) {}

type noOutreach struct{}

func (noOutreach) LastOutreachBy(string, string) (string, error) { return "", nil }

type unusedRoutes struct{ conversation.DepartmentRoutes }

type unusedRevoker struct {
	conversation.EntryAccessRevoker
}

type noPortfolio struct{}

func (noPortfolio) HolderOfConversation(string, string, string) (string, error) { return "", nil }

type ungoverned struct{}

func (ungoverned) EntryAutomation(string, string) (conversation.AutomationProfile, error) {
	return conversation.AutomationProfile{}, nil
}

type quietEntries struct{}

func (quietEntries) BroadcastEntryUpdate(string, string, *conversation.Message) {}
func (quietEntries) AnnounceOwnerChange(string, string, string, string)         {}

type quietAlerts struct{}

func (quietAlerts) AlertAssignee(string, string, string, string) {}

type quietPauser struct{}

func (quietPauser) SetAutomation(context.Context, string, shared.EntryType, *bool) error { return nil }

type quietSessions struct{}

func (quietSessions) End(aa.EndRequest) {}

type unusedDepartments struct{ DepartmentLookup }

type noAccounts struct{}

func (noAccounts) EntryAccountID(string, string) (string, error) { return "", nil }

type unusedReceivers struct{ ConversationReceivers }

type noSettings struct{}

func (noSettings) GetByWorkspaceID(context.Context, string) (*wsc.WorkspaceConfig, error) {
	return nil, nil
}

type emptyRoster struct{}

func (emptyRoster) ListRouletteMembers(string, string, bool) ([]string, error) { return nil, nil }

type nobodySeen struct{}

func (nobodySeen) LastSeen(string, []string) (map[string]time.Time, error) {
	return map[string]time.Time{}, nil
}

type noWorkload struct{}

func (noWorkload) OpenConversations(string, []string) (map[string]int, error) {
	return map[string]int{}, nil
}

type noHandouts struct{}

func (noHandouts) LastHandouts(string, []string) (map[string]time.Time, error) {
	return map[string]time.Time{}, nil
}

func candidatesFor(online conversation.EligibleUserProvider) *CandidateResolver {
	r, err := NewCandidateResolver(CandidateDeps{Online: online, Roster: emptyRoster{}, Presence: nobodySeen{}, Workload: noWorkload{}, Handouts: noHandouts{}})
	if err != nil {
		panic(err)
	}
	return r
}

func newService(repo ia.Repository, eligible conversation.EligibleUserProvider, resolver conversation.CampaignWorkspaceResolver, cfg WorkspaceConfigProvider) *AssignmentService {
	if cfg == nil {
		cfg = noSettings{}
	}
	svc, err := NewAssignmentService(AssignmentDeps{
		Assignments:       repo,
		Candidates:        candidatesFor(eligible),
		WorkspaceResolver: resolver,
		WorkspaceConfig:   cfg,
		Telemetry:         quietTelemetry{},
		Events:            quietEvents{},
		Outreach:          noOutreach{},
		Routes:            unusedRoutes{},
		AccessRevoker:     unusedRevoker{},
		Portfolio:         noPortfolio{},
		Automation:        ungoverned{},
		Entries:           quietEntries{},
		Alerts:            quietAlerts{},
		Pauser:            quietPauser{},
		AISessions:        quietSessions{},
		Departments:       unusedDepartments{},
		Accounts:          noAccounts{},
		Receivers:         unusedReceivers{},
	})
	if err != nil {
		panic(err)
	}
	return svc
}

func TestTheAssignmentServiceRefusesEveryMissingDependency(t *testing.T) {
	svc, err := NewAssignmentService(AssignmentDeps{})
	var missing *deps.MissingError
	if svc != nil || !errors.As(err, &missing) || len(missing.Missing) != 18 {
		t.Fatalf("svc = %v err = %v, want every dependency refused", svc, err)
	}
}

func TestTheCandidateResolverRefusesEveryMissingReader(t *testing.T) {
	r, err := NewCandidateResolver(CandidateDeps{})
	var missing *deps.MissingError
	if r != nil || !errors.As(err, &missing) || len(missing.Missing) != 5 {
		t.Fatalf("r = %v err = %v, want every reader refused", r, err)
	}
}

func TestTheRescueJobRefusesEveryMissingDependency(t *testing.T) {
	job, err := NewRescueJob(RescueDeps{})
	var missing *deps.MissingError
	if job != nil || !errors.As(err, &missing) || len(missing.Missing) != 6 {
		t.Fatalf("job = %v err = %v, want every dependency refused", job, err)
	}
}

type noDepartmentHours struct{}

func (noDepartmentHours) ListWorkingHours([]string) ([]wd.DepartmentSchedule, error) { return nil, nil }

func rescueJob(cfg RescueConfigReader, history RescueHistoryReader, attention ia.EntryAttentionReader, status RescueStatusReader, svc *AssignmentService) *RescueJob {
	job, err := NewRescueJob(RescueDeps{Config: cfg, History: history, Attention: attention, Status: status, Assign: svc, Departments: noDepartmentHours{}})
	if err != nil {
		panic(err)
	}
	return job
}
