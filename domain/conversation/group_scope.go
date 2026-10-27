package conversation

type GroupScope string

const (
	GroupScopeClients GroupScope = ""
	GroupScopeGroups  GroupScope = "groups"
	GroupScopeNone    GroupScope = "none"
)

const GroupsStatusCountKey = "groups"

func InboxGroupScope(wantsGroups, hidesGroups bool) GroupScope {
	switch {
	case !wantsGroups:
		return GroupScopeClients
	case hidesGroups:
		return GroupScopeNone
	default:
		return GroupScopeGroups
	}
}

func (s GroupScope) Lists(isGroup bool) bool {
	switch s {
	case GroupScopeGroups:
		return isGroup
	case GroupScopeNone:
		return false
	default:
		return !isGroup
	}
}

func (s GroupScope) ListsFinished() bool {
	return s == GroupScopeGroups
}

type EntryReach int

const (
	EntryUnseen EntryReach = iota
	EntryListed
	EntryElsewhere
)

func ReachOf(isGroup bool, scope GroupScope, hidesGroups bool) EntryReach {
	switch {
	case isGroup && hidesGroups:
		return EntryUnseen
	case scope.Lists(isGroup):
		return EntryListed
	default:
		return EntryElsewhere
	}
}

type StatusTally struct {
	Status string
	Group  bool
	Count  int64
}

func FoldStatusCounts(tallies []StatusTally) map[string]int64 {
	out := make(map[string]int64, len(tallies))
	for _, tally := range tallies {
		if tally.Group {
			out[GroupsStatusCountKey] += tally.Count
			continue
		}
		out[tally.Status] += tally.Count
	}
	return out
}
