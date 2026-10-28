package receptivetest

import (
	"context"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
)

type Guard struct {
	Refusal  error
	Admitted []conversation.ChannelAutomation
	Settled  []string
}

func Allow() *Guard { return &Guard{} }

func Refuse(err error) *Guard { return &Guard{Refusal: err} }

func (g *Guard) Admit(_ string, _ domain.SlotKind, config conversation.ChannelAutomation) error {
	g.Admitted = append(g.Admitted, config)
	return g.Refusal
}

func (g *Guard) Saved(_ context.Context, _ string, kind domain.SlotKind, slotID string, _ conversation.ChannelAutomation) {
	g.Settled = append(g.Settled, string(kind)+":"+slotID)
}

var _ domain.ReceptiveGuard = (*Guard)(nil)
