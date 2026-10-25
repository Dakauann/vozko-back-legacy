package conversation

import (
	"slices"
	"strings"

	"vozko/domain/actor"
)

type Distribution int

const (
	DistributionUnknown Distribution = iota
	DistributionAutomatic
	DistributionManual
)

type MessageAttention struct {
	Owner        string
	Distribution Distribution
	Takers       []string
}

type MessageAttentionReader interface {
	MessageAttention(workspaceID, entryID, entryType, knownOwner string) MessageAttention
}

func (a MessageAttention) Alerts(viewerID string) bool {
	if viewerID == "" {
		return false
	}
	if a.Owner != "" {
		return a.Owner == viewerID && actor.KindOf(a.Owner) == actor.KindHuman
	}
	return a.Distribution == DistributionManual && slices.Contains(a.Takers, viewerID)
}

func AlertsAssignee(assignee, assignedBy string) bool {
	assignee = strings.TrimSpace(assignee)
	return actor.KindOf(assignee) == actor.KindHuman && assignee != strings.TrimSpace(assignedBy)
}
