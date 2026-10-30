package conversation

type InboxView struct {
	Status ConversationStatus
	Groups GroupScope
}

func (v InboxView) ExcludesFinished() bool {
	return v.Status == "" && !v.Groups.ListsFinished()
}

func (v InboxView) Includes(status ConversationStatus) bool {
	if v.Status != "" {
		return status == v.Status
	}
	return !v.ExcludesFinished() || status != ConversationStatusFinished
}
