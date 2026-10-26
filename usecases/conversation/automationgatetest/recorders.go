package automationgatetest

import (
	"context"
	"sync"

	"vozko/domain/conversation"
	"vozko/domain/workflow"
)

type Replies struct {
	mu       sync.Mutex
	requests []conversation.AIReplyRequest
}

func (r *Replies) Reply(_ context.Context, req conversation.AIReplyRequest) (*conversation.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
	return nil, nil
}

func (r *Replies) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

type Triggers struct {
	mu     sync.Mutex
	events []workflow.TriggerEvent
}

func (t *Triggers) Evaluate(event workflow.TriggerEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}

func (t *Triggers) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.events)
}
