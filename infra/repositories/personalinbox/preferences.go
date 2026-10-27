package personalinbox_repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/personalinbox"
	"vozko/infra/database/schema"
)

func (r *Repository) FindPreferences(ctx context.Context, workspaceID, userID string) (personalinbox.Preferences, error) {
	var row schema.PersonalInboxPreference
	err := r.db.WithContext(ctx).Where("workspace_id = ? AND user_id = ?", workspaceID, userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return personalinbox.DefaultPreferences(), nil
	}
	if err != nil {
		return personalinbox.Preferences{}, err
	}
	return personalinbox.Preferences{HideGroups: row.HideGroups}, nil
}

func (r *Repository) SavePreferences(ctx context.Context, workspaceID, userID string, prefs personalinbox.Preferences) error {
	row := schema.PersonalInboxPreference{
		WorkspaceID: workspaceID,
		UserID:      userID,
		HideGroups:  prefs.HideGroups,
		UpdatedAt:   time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"hide_groups", "updated_at"}),
	}).Create(&row).Error
}
