package conversation_repository

import "vozko/domain/conversation"

const UnofficialGroupColumn = "is_group"

const groupScopeMatchesNothing = "1 = 0"

func groupScopeCondition(scope conversation.GroupScope, column string) string {
	switch {
	case scope == conversation.GroupScopeNone:
		return groupScopeMatchesNothing
	case scope == conversation.GroupScopeGroups && column == "":
		return groupScopeMatchesNothing
	case scope == conversation.GroupScopeGroups:
		return column + " IS TRUE"
	case column == "":
		return ""
	default:
		return column + " IS NOT TRUE"
	}
}
