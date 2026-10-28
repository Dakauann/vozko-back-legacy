package workflow_repository

import (
	"time"

	"gorm.io/gorm"

	"vozko/domain/workflow"
	"vozko/infra/database/schema"
)

type testValuesRepository struct {
	db *gorm.DB
}

func NewTestValuesRepository(db *gorm.DB) workflow.TestValuesRepository {
	return &testValuesRepository{db: db}
}

func (r *testValuesRepository) TestValues(workspaceID, workflowID, userID string) (map[string]string, error) {
	var rows []schema.WorkflowTestValue
	if err := r.db.Where("workspace_id = ? AND workflow_id = ? AND user_id = ?", workspaceID, workflowID, userID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Name] = row.Value
	}
	return out, nil
}

func (r *testValuesRepository) SaveTestValues(workspaceID, workflowID, userID string, values map[string]string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := deleteTestValues(tx, workspaceID, workflowID, userID); err != nil {
			return err
		}
		if len(values) == 0 {
			return nil
		}
		now := time.Now().UTC()
		rows := make([]schema.WorkflowTestValue, 0, len(values))
		for name, value := range values {
			rows = append(rows, schema.WorkflowTestValue{WorkflowID: workflowID, UserID: userID, Name: name, WorkspaceID: workspaceID, Value: value, UpdatedAt: now})
		}
		return tx.Create(&rows).Error
	})
}

func (r *testValuesRepository) ClearTestValues(workspaceID, workflowID, userID string) error {
	return deleteTestValues(r.db, workspaceID, workflowID, userID)
}

func deleteTestValues(db *gorm.DB, workspaceID, workflowID, userID string) error {
	return db.Where("workspace_id = ? AND workflow_id = ? AND user_id = ?", workspaceID, workflowID, userID).Delete(&schema.WorkflowTestValue{}).Error
}
