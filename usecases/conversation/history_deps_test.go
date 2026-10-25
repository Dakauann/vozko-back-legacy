package conversation_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/agent"
	"vozko/domain/conversation"
	"vozko/domain/dealautomation"
	ia "vozko/domain/inbox_assignment"
	"vozko/domain/livedecision"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	"vozko/domain/workflow"
	livedecisions_usecase "vozko/usecases/livedecisions"
	tools_usecase "vozko/usecases/tools"
)

type historyNoMedia struct {
	conversation.ConversationMediaRepository
}

func (historyNoMedia) ListByIDs([]string) ([]*conversation.ConversationMedia, error) { return nil, nil }

type historyNoAssignments struct{ ia.Repository }

func (historyNoAssignments) FindByEntries(string, []string) ([]*ia.InboxAssignment, error) {
	return nil, nil
}

type historyNoDelegations struct {
	conversation.DelegationRepository
}

func (historyNoDelegations) FindMany(context.Context, []shared.EntryRef) (map[shared.EntryRef]conversation.Automation, error) {
	return nil, nil
}

type historyNoWorkspaces struct{}

func (historyNoWorkspaces) GetEntryWorkspaceID(string, string) (string, error) { return "", nil }

type historyNoRuns struct{}

func (historyNoRuns) FindActiveByEntries([]string) (map[string]*workflow.WorkflowRun, error) {
	return nil, nil
}

type historyNoWorkflows struct{ WorkflowLookup }

func (historyNoWorkflows) FindByIDs([]string) ([]*workflow.Workflow, error) { return nil, nil }

func historyWith(s HistoryProviderService) *HistoryProviderService {
	if s.mediaRepo == nil {
		s.mediaRepo = historyNoMedia{}
	}
	if s.assignmentRepo == nil {
		s.assignmentRepo = historyNoAssignments{}
	}
	if s.delegations == nil {
		s.delegations = historyNoDelegations{}
	}
	if s.channelAdapters == nil {
		s.channelAdapters = conversation.NewAdapterRegistry()
	}
	if s.entryWorkspaces == nil {
		s.entryWorkspaces = historyNoWorkspaces{}
	}
	if s.workflowRunRepo == nil {
		s.workflowRunRepo = historyNoRuns{}
	}
	if s.workflowRepo == nil {
		s.workflowRepo = historyNoWorkflows{}
	}
	return &s
}

func TestTheHistoryProviderRefusesEveryMissingDependency(t *testing.T) {
	provider, err := NewHistoryProviderService(HistoryDeps{})
	var missing *deps.MissingError
	if provider != nil || !errors.As(err, &missing) || len(missing.Missing) != 13 {
		t.Fatalf("provider = %v err = %v, want every dependency refused", provider, err)
	}
}

func senderWith(s *MessageSenderService) *MessageSenderService {
	if s.channelAdapters == nil {
		s.channelAdapters = conversation.NewAdapterRegistry()
	}
	return s
}

func markerDefaults(s *MessageMarkerService) *MessageMarkerService {
	if s.channelAdapters == nil {
		s.channelAdapters = conversation.NewAdapterRegistry()
	}
	return s
}

type inboxNoSchedules struct{}

func (inboxNoSchedules) AwaitingAnalysis([]string, string) (map[string]bool, error) { return nil, nil }

type inboxUnusedTemplates struct{ conversation.TemplateSender }

type inboxNoLiveReads struct{}

func (inboxNoLiveReads) LiveReadViews(string, []string, string) (map[string]*livedecision.LiveReadView, error) {
	return nil, nil
}

func inboxSources() InboxSources {
	return InboxSources{Schedules: inboxNoSchedules{}, Templates: inboxUnusedTemplates{}, LiveReads: inboxNoLiveReads{}}
}

type debounceNoQueue struct{}

func (debounceNoQueue) EnqueueSubject(context.Context, *AnalysisSubject) error { return nil }

type debounceDefaultWindows struct{}

func (debounceDefaultWindows) ConfiguredDebounceWindows(context.Context) (map[string]time.Duration, error) {
	return nil, nil
}

type debounceNoDealPipeline struct{}

func (debounceNoDealPipeline) PipelineFor(string, dealautomation.Channel, string) (string, error) {
	return "", nil
}

type debounceUnusedDealDesk struct {
	tools_usecase.OpportunityManager
}

type debounceOpenCascade struct{}

func (debounceOpenCascade) StageNeedsReview(context.Context, livedecisions_usecase.Trigger) bool {
	return true
}

func (debounceOpenCascade) QuietGate(_ context.Context, _ livedecisions_usecase.Trigger, wantMemory, wantDeals bool, _, _ string) livedecision.QuietGate {
	return livedecision.QuietGate{NeedsMemory: wantMemory, NeedsDeals: wantDeals}
}

type debounceNoAgents struct{}

func (debounceNoAgents) FindByID(string) (*agent.Agent, error) { return nil, nil }

func debounceWith(j *analysisDebounceJob) *analysisDebounceJob {
	if j.analysisQueue == nil {
		j.analysisQueue = debounceNoQueue{}
	}
	if j.debouncePolicy == nil {
		j.debouncePolicy = debounceDefaultWindows{}
	}
	if j.dealSettings == nil {
		j.dealSettings = debounceNoDealPipeline{}
	}
	if j.dealDesk == nil {
		j.dealDesk = debounceUnusedDealDesk{}
	}
	if j.cascade == nil {
		j.cascade = debounceOpenCascade{}
	}
	if j.profileAgents == nil {
		j.profileAgents = debounceNoAgents{}
	}
	return j
}
