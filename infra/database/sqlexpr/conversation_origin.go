package sqlexpr

import (
	"strings"

	"vozko/domain/conversation"
)

const ConversationOriginTable = "conversation_origins"

func ConversationOrigin(alias string) string {
	var b strings.Builder
	b.WriteString("CASE " + column(alias, "sender_kind"))
	for _, origin := range conversation.Origins() {
		for _, kind := range conversation.OriginSenders(origin) {
			b.WriteString(" WHEN " + Literal(string(kind)) + " THEN " + Literal(string(origin)))
		}
	}
	b.WriteString(" ELSE " + Literal(string(conversation.OriginUnknown)) + " END")
	return b.String()
}

func OpensConversation(alias string) string {
	return column(alias, "sender_kind") + " NOT IN (" + LiteralList(stringsOf(conversation.NonOpeningSenderKinds())) + ") AND " + NotSeedPlaceholder(alias)
}

func ConversationOriginIs(entryID, entryType string) string {
	return conversationOriginExists(entryID, entryType, "co_f.origin = ?")
}

func ConversationOriginIn(entryID, entryType string) string {
	return conversationOriginExists(entryID, entryType, "co_f.origin = ANY(?)")
}

func ConversationOriginJoin(alias, entryID, entryType string) string {
	return "LEFT JOIN " + ConversationOriginTable + " " + alias + " ON " + conversationOriginKey(alias, entryID, entryType)
}

func conversationOriginExists(entryID, entryType, match string) string {
	return "EXISTS (SELECT 1 FROM " + ConversationOriginTable + " co_f WHERE " + conversationOriginKey("co_f", entryID, entryType) + " AND " + match + ")"
}

func conversationOriginKey(alias, entryID, entryType string) string {
	return alias + ".entry_id = " + entryID + " AND " + alias + ".entry_type = " + entryType
}
