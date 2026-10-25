package ws

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/conversation"
	inbox_assignment "vozko/domain/inbox_assignment"
	"vozko/domain/personalinbox"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	"vozko/domain/whatsapp_campaign"
	workspace_department "vozko/domain/workspace/workspace_department"
)

type quietHubHistory struct{ conversation.HistoryProvider }

func (quietHubHistory) GetUnreadCount(string, shared.EntryType) (int64, error) { return 0, nil }
func (quietHubHistory) GetEntryInfo(string, string) (conversation.EntryInfo, error) {
	return conversation.EntryInfo{}, nil
}
func (quietHubHistory) PresentMessage(string, string, *conversation.Message) {}
func (quietHubHistory) GetWindowStatusForEntry(string, string) conversation.WindowState {
	return conversation.WindowState{}
}

type quietHubHistoryReader struct{}

func (quietHubHistoryReader) ReadHistory(conversation.HistoryQuery) (conversation.HistoryPage, error) {
	return conversation.HistoryPage{}, nil
}

type quietHubMarker struct{}

func (quietHubMarker) MarkAsRead(string, shared.EntryType, []string, string) error { return nil }
func (quietHubMarker) SendTypingIndicator(string, shared.EntryType) error          { return nil }

type quietHubStages struct{ conversation.StageProvider }

func (quietHubStages) GetEntryStage(string, string, string) (*conversation.InboxEntryStage, error) {
	return nil, nil
}

type quietHubLabels struct{ conversation.LabelProvider }

func (quietHubLabels) GetEntryLabels(string, string, string) ([]conversation.InboxEntryLabel, error) {
	return nil, nil
}

type unusedHubTemplates struct{ conversation.TemplateSender }

type quietHubWorkspaces struct {
	conversation.CampaignWorkspaceResolver
}

func (quietHubWorkspaces) GetCampaignWorkspaceID(string, string) (string, error) { return "", nil }
func (quietHubWorkspaces) GetEntryWorkspaceID(string, string) (string, error)    { return "", nil }
func (quietHubWorkspaces) GetEntryCampaignID(string, string) (string, error)     { return "", nil }

type unusedHubMessages struct{ conversation.MessageRepository }

type unusedHubCampaigns struct{ whatsapp_campaign.Repository }

func (unusedHubCampaigns) FindByID(string) (*whatsapp_campaign.Campaign, error) {
	return nil, errors.New("no campaign in this test")
}

type quietHubDepartments struct{}

func (quietHubDepartments) ListMembers(string) ([]workspace_department.DepartmentMember, error) {
	return nil, nil
}

type quietHubPresence struct{}

func (quietHubPresence) Transition(string, string, string, string) error { return nil }

type quietHubMediaLimits struct{}

func (quietHubMediaLimits) MediaLimits(shared.EntryType) ([]conversation.MediaLimit, bool) {
	return nil, false
}

type quietHubAssigner struct{}

func (quietHubAssigner) AssignOnOpen(string, string, string, bool) (bool, error) { return false, nil }
func (quietHubAssigner) AssignManual(string, string, string, string, string, string, string) error {
	return nil
}

type ownerOnlyAttention struct{}

func (ownerOnlyAttention) MessageAttention(_, _, _, owner string) conversation.MessageAttention {
	return conversation.MessageAttention{Owner: owner}
}

type unusedHubPersonAssign struct {
	inbox_assignment.PersonAssignUseCase
}

type unusedHubStatus struct {
	conversation.ConversationStatusUpdater
}

type unusedHubPersonFinish struct {
	conversation.PersonFinishUseCase
}

type quietHubMarks struct{ personalinbox.UseCases }

func (quietHubMarks) PageMarks(context.Context, string, []shared.EntryRef) []personalinbox.Mark {
	return nil
}
func (quietHubMarks) MarkRead(context.Context, shared.Person, string, shared.EntryRef) {}

type quietHubTeamChat struct{}

func (quietHubTeamChat) Typing(context.Context, shared.Person, string, string) error { return nil }

type quietHubInbox struct{ conversation.InboxService }

func (quietHubInbox) SearchInbox(string, conversation.SearchInboxInput) ([]conversation.InboxEntry, int64, error) {
	return nil, 0, nil
}
func (quietHubInbox) GetInboxStageCounts(string, string, string) (map[string]int64, error) {
	return nil, nil
}
func (quietHubInbox) GetConversationStatusCounts(string, string, string) (map[string]int64, error) {
	return nil, nil
}
func (quietHubInbox) GetAvailableLabels(string) ([]conversation.InboxEntryLabel, error) {
	return nil, nil
}
func (quietHubInbox) BuildInboxEntry(entryID, entryType string) (*conversation.InboxEntry, error) {
	return &conversation.InboxEntry{EntryID: entryID, EntryType: entryType}, nil
}

func quietHubCommands() HubCommands {
	return HubCommands{
		Assigner:      quietHubAssigner{},
		Attention:     ownerOnlyAttention{},
		PersonAssign:  unusedHubPersonAssign{},
		Status:        unusedHubStatus{},
		PersonFinish:  unusedHubPersonFinish{},
		PersonalMarks: quietHubMarks{},
		TeamChat:      quietHubTeamChat{},
		Inbox:         quietHubInbox{},
	}
}

func fillHubDeps(d *ConversationHubDeps) {
	if d.Metrics == nil {
		d.Metrics = noopWSMetricsRecorder{}
	}
	if d.ReplicaID == "" {
		d.ReplicaID = "test-replica"
	}
	if d.History == nil {
		d.History = quietHubHistory{}
	}
	if d.HistoryReader == nil {
		d.HistoryReader = quietHubHistoryReader{}
	}
	if d.Marker == nil {
		d.Marker = quietHubMarker{}
	}
	if d.Stages == nil {
		d.Stages = quietHubStages{}
	}
	if d.Labels == nil {
		d.Labels = quietHubLabels{}
	}
	if d.Templates == nil {
		d.Templates = unusedHubTemplates{}
	}
	if d.Workspaces == nil {
		d.Workspaces = quietHubWorkspaces{}
	}
	if d.Messages == nil {
		d.Messages = unusedHubMessages{}
	}
	if d.Campaigns == nil {
		d.Campaigns = unusedHubCampaigns{}
	}
	if d.Departments == nil {
		d.Departments = quietHubDepartments{}
	}
	if d.Presence == nil {
		d.Presence = quietHubPresence{}
	}
	if d.MediaLimits == nil {
		d.MediaLimits = quietHubMediaLimits{}
	}
}

func testHub(d ConversationHubDeps) *ConversationHub {
	fillHubDeps(&d)
	hub, err := NewConversationHub(d)
	if err != nil {
		panic(err)
	}
	if err := hub.BindCommands(quietHubCommands()); err != nil {
		panic(err)
	}
	return hub
}

func TestTheHubRefusesToBindIncompleteCommandsOrToBindTwice(t *testing.T) {
	d := ConversationHubDeps{Authorizer: multiOpenAuthorizer{}}
	fillHubDeps(&d)
	hub, err := NewConversationHub(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := hub.CommandsBound(); !errors.Is(err, ErrHubCommandsUnbound) {
		t.Fatalf("CommandsBound before binding = %v", err)
	}
	if err := hub.BindCommands(HubCommands{}); !errors.Is(err, deps.ErrMissing) {
		t.Fatalf("BindCommands(empty) = %v, want missing commands refused", err)
	}
	if err := hub.BindCommands(quietHubCommands()); err != nil {
		t.Fatal(err)
	}
	if err := hub.BindCommands(quietHubCommands()); !errors.Is(err, ErrHubCommandsBound) {
		t.Fatalf("second BindCommands = %v, want ErrHubCommandsBound", err)
	}
	if err := hub.CommandsBound(); err != nil {
		t.Fatalf("CommandsBound after binding = %v", err)
	}
}

func TestNewConversationHubRefusesMissingMetrics(t *testing.T) {
	var typedNil *noopWSMetricsRecorder
	for name, d := range map[string]ConversationHubDeps{
		"nil":       {Authorizer: multiOpenAuthorizer{}, ReplicaID: "r"},
		"typed nil": {Authorizer: multiOpenAuthorizer{}, ReplicaID: "r", Metrics: typedNil},
	} {
		t.Run(name, func(t *testing.T) {
			hub, err := NewConversationHub(d)
			if hub != nil || !errors.Is(err, deps.ErrMissing) {
				t.Fatalf("hub = %v err = %v, want the missing metrics refused", hub, err)
			}
		})
	}
}

func TestNewConversationHubRefusesAMissingAuthorizer(t *testing.T) {
	hub, err := NewConversationHub(ConversationHubDeps{Metrics: noopWSMetricsRecorder{}, ReplicaID: "r"})
	var missing *deps.MissingError
	if hub != nil || !errors.As(err, &missing) || missing.Missing[0] != "authorizer" {
		t.Fatalf("hub = %v err = %v, want the authorizer refused", hub, err)
	}
}

func TestTheCallSocketRefusesAMissingAuthorizer(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a call socket without an authorizer must fail boot")
		}
	}()
	NewCallSessionWSHandler(&fakeStartUseCase{}, &fakeEndUseCase{}, nil, nil, nil, noopWSMetricsRecorder{})
}

type historyInbox struct {
	quietHubInbox
	history interface {
		GetInboxEntry(entryID, entryType string) (*conversation.InboxEntry, error)
	}
}

func (i historyInbox) BuildInboxEntry(entryID, entryType string) (*conversation.InboxEntry, error) {
	return i.history.GetInboxEntry(entryID, entryType)
}
