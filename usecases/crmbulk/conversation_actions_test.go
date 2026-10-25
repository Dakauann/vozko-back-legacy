package crmbulk_usecase

import (
	"context"
	"reflect"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/workspace"
)

type finishCall struct {
	by          shared.Person
	workspaceID string
	entry       shared.EntryRef
	outcome     string
}

type fakeFinisher struct {
	calls   []finishCall
	failIDs map[string]error
}

func (f *fakeFinisher) Finish(_ context.Context, by shared.Person, workspaceID string, entry shared.EntryRef, outcome string) error {
	f.calls = append(f.calls, finishCall{by, workspaceID, entry, outcome})
	return f.failIDs[entry.EntryID]
}

type readCall struct {
	entryID   string
	entryType shared.EntryType
	ids       []string
	userID    string
}

type fakeReadMarker struct {
	calls []readCall
}

func (f *fakeReadMarker) MarkAsRead(entryID string, entryType shared.EntryType, messageIDs []string, userID string) error {
	f.calls = append(f.calls, readCall{entryID, entryType, messageIDs, userID})
	return nil
}

type personalCall struct {
	kind  string
	by    shared.Person
	entry shared.EntryRef
}

type fakePersonal struct {
	calls []personalCall
}

func (f *fakePersonal) MarkRead(_ context.Context, by shared.Person, _ string, entry shared.EntryRef) {
	f.calls = append(f.calls, personalCall{"read", by, entry})
}

type conversationFixture struct {
	finish   *fakeFinisher
	read     *fakeReadMarker
	personal *fakePersonal
	bc       *mockBroadcaster
	authz    *mockAuthorizer
	svc      *Service
}

func newConversationFixture() conversationFixture {
	f := conversationFixture{
		finish:   &fakeFinisher{failIDs: map[string]error{}},
		read:     &fakeReadMarker{},
		personal: &fakePersonal{},
		bc:       &mockBroadcaster{},
		authz: &mockAuthorizer{allowPerm: map[string]bool{
			"conversations:send": true,
			"conversations:read": true,
		}},
	}
	f.svc = newTestService(Deps{Stages: &mockStageAssigner{}, LabelAdds: &mockLabelAssigner{}, LabelRemovals: &mockLabelRemover{}, Assigner: &mockEntryAssigner{}, Authz: f.authz, Broadcaster: f.bc, Conversations: ConversationActions{Finish: f.finish, Read: f.read, Personal: f.personal}})
	return f
}

func TestConversationActionsNeedTheSamePermissionAsTheirSingleCommand(t *testing.T) {
	cases := map[string]string{
		ActionClose:    "conversations:send",
		ActionMarkRead: "conversations:read",
	}
	for action, want := range cases {
		required, ok := actionRequirements[action]
		if !ok || string(required.Resource)+":"+string(required.Action) != want {
			t.Errorf("%s requires %+v, want %s", action, required, want)
		}
		if !workspace.ValidActionForResource(required.Resource, required.Action) {
			t.Errorf("%s requires an unregistered permission", action)
		}
	}
}

func TestBulkCloseFinishesEachConversationAsTheActorAndReportsFailuresPerEntry(t *testing.T) {
	f := newConversationFixture()
	f.finish.failIDs["e2"] = conversation.ErrOutcomeRequired

	res := run(f.svc, BulkInput{
		WorkspaceID: "ws-1", ActorID: "actor-1", Action: ActionClose, OutcomeCode: "sold",
		Selection: byIDs, Targets: targets("e1", "e2", "e3"),
	})

	if res.err != nil || res.Succeeded != 2 || len(res.Failed) != 1 || res.Failed[0].ID != "e2" {
		t.Fatalf("result = %+v", res)
	}
	if res.Failed[0].Error != conversation.ErrOutcomeRequired.Error() {
		t.Errorf("failure = %q, want the outcome refusal", res.Failed[0].Error)
	}
	want := finishCall{shared.Person{UserID: "actor-1"}, "ws-1", shared.EntryRef{EntryID: "e1", EntryType: shared.EntryTypeWhatsApp}, "sold"}
	if f.finish.calls[0] != want {
		t.Errorf("finish = %+v, want %+v", f.finish.calls[0], want)
	}
	if len(f.bc.entry)+len(f.bc.stage)+len(f.bc.label) != 0 {
		t.Error("the status service announces the close; bulk must not broadcast it again")
	}
}

func TestBulkMarkReadUsesTheReadMarkerAndClearsThePersonalUnreadMark(t *testing.T) {
	f := newConversationFixture()

	res := run(f.svc, BulkInput{WorkspaceID: "ws-1", ActorID: "actor-1", Action: ActionMarkRead, Selection: byIDs, Targets: targets("e1", "e2")})

	if res.err != nil || res.Succeeded != 2 {
		t.Fatalf("result = %+v", res)
	}
	if len(f.read.calls) != 2 || !reflect.DeepEqual(f.read.calls[0], readCall{entryID: "e1", entryType: shared.EntryTypeWhatsApp, userID: "actor-1"}) {
		t.Fatalf("read marker calls = %+v", f.read.calls)
	}
	if len(f.personal.calls) != 2 || f.personal.calls[0].kind != "read" {
		t.Fatalf("personal calls = %+v, want the unread mark cleared per entry", f.personal.calls)
	}
	if len(f.bc.entry) != 2 {
		t.Errorf("rows must refresh their unread count, got %v", f.bc.entry)
	}
}

func TestBulkConversationActionsSkipEntriesOutsideTheActorsScope(t *testing.T) {
	f := newConversationFixture()
	f.authz.denyEntry = map[string]bool{"e2": true}

	res := run(f.svc, BulkInput{WorkspaceID: "ws-1", ActorID: "actor-1", Action: ActionClose, Selection: byIDs, Targets: targets("e1", "e2")})

	if res.Succeeded != 1 || len(res.Failed) != 1 || res.Failed[0].Error != ErrForbiddenEntry.Error() {
		t.Fatalf("result = %+v", res)
	}
	if len(f.finish.calls) != 1 {
		t.Fatalf("an entry outside the scope must not reach the finisher: %+v", f.finish.calls)
	}
}
