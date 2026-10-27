package scheduled_message_usecase

import (
	"context"
	"sync"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

type fakeGate struct {
	mu     sync.Mutex
	refuse bool
	err    error
	calls  int
}

func (g *fakeGate) MayAct(string, string, shared.EntryType) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if g.err != nil {
		return false, g.err
	}
	return !g.refuse, nil
}

func (g *fakeGate) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

type fakeZones struct {
	loc *time.Location
	err error
}

func (z fakeZones) Location(context.Context, string, string) (*time.Location, error) {
	return z.loc, z.err
}

type fakeAutomationSend struct {
	mu    sync.Mutex
	calls []conversation.AutomationSendInput
	err   error
}

func (s *fakeAutomationSend) Execute(_ context.Context, in conversation.AutomationSendInput) (*conversation.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, in)
	if s.err != nil {
		return nil, s.err
	}
	return &conversation.Message{ID: "msg-auto", EntryID: in.EntryID, EntryType: shared.EntryType(in.EntryType)}, nil
}

func (s *fakeAutomationSend) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

type fakeNames map[string]string

func (n fakeNames) Names(actorIDs ...string) map[string]string {
	out := map[string]string{}
	for _, id := range actorIDs {
		if name, ok := n[id]; ok {
			out[id] = name
		}
	}
	return out
}
