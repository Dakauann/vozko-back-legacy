package automationneeds_repository

import (
	"context"

	"gorm.io/gorm"

	"vozko/domain/actor"
	"vozko/domain/conversation"
	automationneeds_usecase "vozko/usecases/automationneeds"
)

const heldSQL = `SELECT CAST(ia.entry_id AS text) AS entry_id, ia.entry_type
FROM inbox_assignments ia
WHERE ia.workspace_id = ? AND CAST(ia.assigned_user_id AS text) = ? AND ia.assignee_kind = ? AND CAST(ia.business_phone_id AS text) = ?
  AND NOT EXISTS (SELECT 1 FROM conversation_delegations d WHERE d.entry_id = CAST(ia.entry_id AS text) AND d.entry_type = ia.entry_type)
ORDER BY ia.entry_id
LIMIT ?`

type Held struct {
	db *gorm.DB
}

func NewHeld(db *gorm.DB) *Held {
	return &Held{db: db}
}

func (h *Held) HeldOn(ctx context.Context, workspaceID string, automation conversation.Automation, slotID string, limit int) ([]automationneeds_usecase.HeldConversation, error) {
	id, kind := actor.Split(automation.ActorID())
	var rows []automationneeds_usecase.HeldConversation
	err := h.db.WithContext(ctx).Raw(heldSQL, workspaceID, id, string(kind), slotID, limit).Scan(&rows).Error
	return rows, err
}
