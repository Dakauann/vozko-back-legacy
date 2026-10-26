package automationgatetest

import (
	"context"
	"errors"

	"vozko/domain/conversation"
	ia "vozko/domain/inbox_assignment"
	"vozko/domain/shared"
	conversation_usecase "vozko/usecases/conversation"
)

var ErrUnreadable = errors.New("automationgatetest: the owner store is down")

type Owners struct {
	Owner string
	Err   error
}

func (o Owners) FindByEntry(workspaceID, entryID, entryType string) (*ia.InboxAssignment, error) {
	if o.Err != nil {
		return nil, o.Err
	}
	if o.Owner == "" {
		return nil, nil
	}
	return &ia.InboxAssignment{WorkspaceID: workspaceID, EntryID: entryID, EntryType: entryType, AssignedUserID: o.Owner}, nil
}

type Delegations struct {
	To  *conversation.Automation
	Err error
}

func (d Delegations) Find(context.Context, string, shared.EntryType) (*conversation.Delegation, error) {
	if d.Err != nil {
		return nil, d.Err
	}
	if d.To == nil {
		return nil, nil
	}
	return &conversation.Delegation{Automation: *d.To}, nil
}

func (d Delegations) FindMany(context.Context, []shared.EntryRef) (map[shared.EntryRef]conversation.Automation, error) {
	return nil, nil
}

func (Delegations) Save(context.Context, conversation.Delegation) error { return nil }

func (Delegations) Delete(context.Context, string, shared.EntryType) error { return nil }

type Profiles struct {
	Profile conversation.AutomationProfile
}

func (p Profiles) EntryAutomation(string, string) (conversation.AutomationProfile, error) {
	return p.Profile, nil
}

func Gate(owners Owners, delegations Delegations) *conversation_usecase.AutomationGate {
	return conversation_usecase.NewAutomationGate(conversation_usecase.AutomationGateDeps{
		Owners:      owners,
		Delegations: delegations,
		Profiles:    Profiles{},
	})
}

func Open() *conversation_usecase.AutomationGate { return Gate(Owners{}, Delegations{}) }

func OwnedBy(owner string) *conversation_usecase.AutomationGate {
	return Gate(Owners{Owner: owner}, Delegations{})
}

func DelegatedBy(owner string, to conversation.Automation) *conversation_usecase.AutomationGate {
	return Gate(Owners{Owner: owner}, Delegations{To: &to})
}

func Unreadable() *conversation_usecase.AutomationGate {
	return Gate(Owners{Err: ErrUnreadable}, Delegations{})
}

type Case struct {
	Name    string
	Gate    *conversation_usecase.AutomationGate
	Answers bool
}

func Cases(delegate conversation.Automation) []Case {
	return []Case{
		{Name: "unowned", Gate: Open(), Answers: true},
		{Name: "owned by the agent", Gate: OwnedBy("ai:agent-1"), Answers: true},
		{Name: "owned by a workflow", Gate: OwnedBy("workflow:wf-1"), Answers: true},
		{Name: "delegated by the person who owns it", Gate: DelegatedBy("user-1", delegate), Answers: true},
		{Name: "owned by a person", Gate: OwnedBy("user-1")},
		{Name: "owner unreadable", Gate: Unreadable()},
	}
}
