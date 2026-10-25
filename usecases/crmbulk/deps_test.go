package crmbulk_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/shared/deps"
)

func completeDeps() Deps {
	return Deps{
		Stages:        &mockStageAssigner{},
		LabelAdds:     &mockLabelAssigner{},
		LabelRemovals: &mockLabelRemover{},
		Assigner:      &mockEntryAssigner{},
		Authz:         allowAll(),
		Broadcaster:   &mockBroadcaster{},
		Targets:       &pagedResolver{},
		CountGate:     openGate{},
		Conversations: ConversationActions{Finish: &fakeFinisher{failIDs: map[string]error{}}, Read: &fakeReadMarker{}, Personal: &fakePersonal{}},
	}
}

func newTestService(d Deps) *Service {
	full := completeDeps()
	if d.Stages != nil {
		full.Stages = d.Stages
	}
	if d.LabelAdds != nil {
		full.LabelAdds = d.LabelAdds
	}
	if d.LabelRemovals != nil {
		full.LabelRemovals = d.LabelRemovals
	}
	if d.Assigner != nil {
		full.Assigner = d.Assigner
	}
	if d.Authz != nil {
		full.Authz = d.Authz
	}
	if d.Broadcaster != nil {
		full.Broadcaster = d.Broadcaster
	}
	if d.Targets != nil {
		full.Targets = d.Targets
	}
	if d.CountGate != nil {
		full.CountGate = d.CountGate
	}
	if d.Conversations.Finish != nil {
		full.Conversations.Finish = d.Conversations.Finish
	}
	if d.Conversations.Read != nil {
		full.Conversations.Read = d.Conversations.Read
	}
	if d.Conversations.Personal != nil {
		full.Conversations.Personal = d.Conversations.Personal
	}
	svc, err := NewService(full)
	if err != nil {
		panic(err)
	}
	return svc
}

func TestNewServiceRefusesEveryMissingDependency(t *testing.T) {
	var typedNilAuthz *mockAuthorizer
	cases := map[string]func(*Deps){
		"stage assigner":        func(d *Deps) { d.Stages = nil },
		"label assigner":        func(d *Deps) { d.LabelAdds = nil },
		"label remover":         func(d *Deps) { d.LabelRemovals = nil },
		"entry assigner":        func(d *Deps) { d.Assigner = nil },
		"authorizer":            func(d *Deps) { d.Authz = typedNilAuthz },
		"broadcaster":           func(d *Deps) { d.Broadcaster = nil },
		"selection resolver":    func(d *Deps) { d.Targets = nil },
		"count gate":            func(d *Deps) { d.CountGate = nil },
		"conversation finisher": func(d *Deps) { d.Conversations.Finish = nil },
		"read marker":           func(d *Deps) { d.Conversations.Read = nil },
		"personal inbox marks":  func(d *Deps) { d.Conversations.Personal = nil },
	}
	for name, drop := range cases {
		t.Run(name, func(t *testing.T) {
			d := completeDeps()
			drop(&d)
			svc, err := NewService(d)
			var missing *deps.MissingError
			if svc != nil || !errors.As(err, &missing) || len(missing.Missing) != 1 || missing.Missing[0] != name {
				t.Fatalf("svc = %v err = %v, want the %s reported missing", svc, err, name)
			}
		})
	}
}

func TestBulkCloseFinishesEveryTargetWithTheWiredFinisher(t *testing.T) {
	finish := &fakeFinisher{failIDs: map[string]error{}}
	svc := newTestService(Deps{
		Authz:         &mockAuthorizer{allowPerm: map[string]bool{"conversations:send": true}},
		Conversations: ConversationActions{Finish: finish},
	})
	res, err := svc.BulkApply(context.Background(), BulkInput{WorkspaceID: "ws-1", ActorID: "a", Action: ActionClose, Selection: byIDs, Targets: targets("e1", "e2")})
	if err != nil || res.Succeeded != 2 || len(finish.calls) != 2 {
		t.Fatalf("res = %+v err = %v finished = %d", res, err, len(finish.calls))
	}
}
