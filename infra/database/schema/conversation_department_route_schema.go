package schema

import "time"

const ConversationDepartmentRoutesTable = "conversation_department_routes"

type ConversationDepartmentRoute struct {
	EntryID      string    `gorm:"type:uuid;primaryKey"`
	EntryType    string    `gorm:"type:varchar(30);primaryKey"`
	DepartmentID string    `gorm:"type:uuid;not null"`
	RoutedAt     time.Time `gorm:"type:timestamptz;not null"`
}

func (ConversationDepartmentRoute) TableName() string { return ConversationDepartmentRoutesTable }
