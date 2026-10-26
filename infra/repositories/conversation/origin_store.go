package conversation_repository

import "vozko/infra/database/sqlexpr"

func recordOriginsSQL(entries string) string {
	return `INSERT INTO ` + sqlexpr.ConversationOriginTable + ` (entry_id, entry_type, origin, first_message_id, first_message_at)
		SELECT fm.entry_id, fm.entry_type, ` + sqlexpr.ConversationOrigin("fm") + `, fm.id, fm.created_at
		FROM (` + entries + `) e
		CROSS JOIN LATERAL (
			SELECT cm.entry_id, cm.entry_type, cm.sender_kind, cm.id, cm.created_at
			FROM conversation_messages cm
			WHERE cm.entry_id = e.entry_id AND cm.entry_type = e.entry_type
			  AND ` + sqlexpr.OpensConversation("cm") + `
			ORDER BY cm.created_at, cm.id
			LIMIT 1
		) fm
		ON CONFLICT (entry_id, entry_type) DO UPDATE SET
			origin = EXCLUDED.origin,
			first_message_id = EXCLUDED.first_message_id,
			first_message_at = EXCLUDED.first_message_at
		WHERE (EXCLUDED.first_message_at, EXCLUDED.first_message_id) < (` + sqlexpr.ConversationOriginTable + `.first_message_at, ` + sqlexpr.ConversationOriginTable + `.first_message_id)
		   OR (EXCLUDED.first_message_id = ` + sqlexpr.ConversationOriginTable + `.first_message_id AND EXCLUDED.origin <> ` + sqlexpr.ConversationOriginTable + `.origin)`
}

func oneEntrySQL() string {
	return `SELECT ?::uuid AS entry_id, ?::text AS entry_type`
}

func unsettledEntrySQL() string {
	return oneEntrySQL() + ` WHERE NOT EXISTS (SELECT 1 FROM ` + sqlexpr.ConversationOriginTable + ` settled
		WHERE settled.entry_id = ?::uuid AND settled.entry_type = ? AND settled.first_message_at < ?)`
}
