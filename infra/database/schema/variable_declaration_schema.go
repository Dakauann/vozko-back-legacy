package schema

import "time"

type ConversationVariableDeclaration struct {
	WorkspaceID string    `gorm:"primaryKey;type:uuid"`
	Name        string    `gorm:"primaryKey;size:64"`
	Label       string    `gorm:"size:60;not null"`
	Description string    `gorm:"type:text;not null;default:''"`
	Kind        string    `gorm:"size:32;not null"`
	Spec        string    `gorm:"type:jsonb;not null;default:'{}'"`
	UpdatedBy   string    `gorm:"type:text;not null;default:''"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

func (ConversationVariableDeclaration) TableName() string {
	return "conversation_variable_declarations"
}
