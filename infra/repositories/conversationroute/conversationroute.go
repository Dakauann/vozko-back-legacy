package conversationroute

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/conversation"
	"vozko/infra/database/schema"
)

type repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) conversation.DepartmentRoutes {
	return &repository{db: db}
}

func (r *repository) RouteToDepartment(entryID, entryType, departmentID string) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "entry_id"}, {Name: "entry_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"department_id", "routed_at"}),
	}).Create(&schema.ConversationDepartmentRoute{
		EntryID:      entryID,
		EntryType:    entryType,
		DepartmentID: departmentID,
		RoutedAt:     time.Now().UTC(),
	}).Error
}

func (r *repository) RoutedDepartment(entryID, entryType string) (string, bool, error) {
	var record schema.ConversationDepartmentRoute
	err := r.db.Where("entry_id = ? AND entry_type = ?", entryID, entryType).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return record.DepartmentID, true, nil
}
