package copilot_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/readiness"
	"vozko/domain/shared/deps"
	"vozko/usecases/agentloop"
)

type quietState struct{}

func (quietState) Snapshot(context.Context, readiness.Person) (*readiness.Snapshot, error) {
	return nil, nil
}

func completeDeps() Deps {
	messages := &fakeMessages{}
	return Deps{
		Engine:   agentloop.Engine{AI: &scriptAI{}},
		Registry: NewRegistry(),
		Access:   &fakeAccess{},
		Funds:    openFunds{},
		Threads:  &fakeThreads{thread: testThread()},
		Messages: messages,
		Files:    workspaceFiles{},
		State:    quietState{},
		NewID:    func() string { return "act-1" },
		Screens:  newMailbox(),
		Queue:    &fakeQueue{messages: messages},
		Adopter:  &fakeAdopter{adopted: map[string]string{}},
	}
}

func testService(d Deps) *Service {
	full := completeDeps()
	if d.Engine.AI != nil {
		full.Engine = d.Engine
	}
	if d.Registry != nil {
		full.Registry = d.Registry
	}
	if d.Access != nil {
		full.Access = d.Access
	}
	if d.Funds != nil {
		full.Funds = d.Funds
	}
	if d.Threads != nil {
		full.Threads = d.Threads
	}
	if d.Messages != nil {
		full.Messages = d.Messages
		if ms, ok := d.Messages.(*fakeMessages); ok && d.Queue == nil {
			full.Queue = &fakeQueue{messages: ms}
		}
	}
	if d.Files != nil {
		full.Files = d.Files
	}
	if d.State != nil {
		full.State = d.State
	}
	if d.NewID != nil {
		full.NewID = d.NewID
	}
	if d.Screens != nil {
		full.Screens = d.Screens
	}
	if d.Queue != nil {
		full.Queue = d.Queue
	}
	if d.Adopter != nil {
		full.Adopter = d.Adopter
	}
	svc, err := NewService(full)
	if err != nil {
		panic(err)
	}
	return svc
}

func TestNewServiceRefusesEveryMissingDependency(t *testing.T) {
	cases := map[string]func(*Deps){
		"tool registry":   func(d *Deps) { d.Registry = nil },
		"access checker":  func(d *Deps) { d.Access = nil },
		"funds checker":   func(d *Deps) { d.Funds = nil },
		"threads":         func(d *Deps) { d.Threads = nil },
		"messages":        func(d *Deps) { d.Messages = nil },
		"attachments":     func(d *Deps) { d.Files = nil },
		"workspace state": func(d *Deps) { d.State = nil },
		"id generator":    func(d *Deps) { d.NewID = nil },
		"screen mailbox":  func(d *Deps) { d.Screens = nil },
		"message queue":   func(d *Deps) { d.Queue = nil },
		"project adopter": func(d *Deps) { d.Adopter = nil },
	}
	for name, drop := range cases {
		t.Run(name, func(t *testing.T) {
			d := completeDeps()
			drop(&d)
			svc, err := NewService(d)
			var missing *deps.MissingError
			if svc != nil || !errors.As(err, &missing) || len(missing.Missing) != 1 || missing.Missing[0] != name {
				t.Fatalf("svc = %v err = %v, want %s reported missing", svc, err, name)
			}
		})
	}
}
