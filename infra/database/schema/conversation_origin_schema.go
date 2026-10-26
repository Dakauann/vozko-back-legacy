package schema

import "time"

type ConversationOrigin struct {
	EntryID        string    `gorm:"type:uuid;primaryKey"`
	EntryType      string    `gorm:"type:varchar(20);primaryKey"`
	Origin         string    `gorm:"type:varchar(16);not null"`
	FirstMessageID string    `gorm:"type:text;not null"`
	FirstMessageAt time.Time `gorm:"type:timestamptz;not null"`
}

func (ConversationOrigin) TableName() string {
	return "conversation_origins"
}
