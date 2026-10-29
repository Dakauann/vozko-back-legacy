package tutorial_repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/tutorial"
	"vozko/infra/database/schema"
)

type Progress struct {
	db *gorm.DB
}

func NewProgress(db *gorm.DB) *Progress {
	return &Progress{db: db}
}

func progressOf(row schema.TutorialProgress) tutorial.Progress {
	return tutorial.Progress{TutorialID: row.TutorialID, Status: tutorial.Status(row.Status), Step: row.Step, UpdatedAt: row.UpdatedAt.UTC()}
}

func (r *Progress) List(ctx context.Context, userID string) ([]tutorial.Progress, error) {
	var rows []schema.TutorialProgress
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("tutorial_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]tutorial.Progress, len(rows))
	for i, row := range rows {
		out[i] = progressOf(row)
	}
	return out, nil
}

func (r *Progress) Find(ctx context.Context, userID, tutorialID string) (tutorial.Progress, bool, error) {
	var row schema.TutorialProgress
	err := r.db.WithContext(ctx).Where("user_id = ? AND tutorial_id = ?", userID, tutorialID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tutorial.Progress{}, false, nil
	}
	if err != nil {
		return tutorial.Progress{}, false, err
	}
	return progressOf(row), true, nil
}

func (r *Progress) Save(ctx context.Context, userID string, progress tutorial.Progress) error {
	row := schema.TutorialProgress{UserID: userID, TutorialID: progress.TutorialID, Status: string(progress.Status), Step: progress.Step, UpdatedAt: progress.UpdatedAt}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "tutorial_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "step", "updated_at"}),
	}).Create(&row).Error
}

func (r *Progress) Delete(ctx context.Context, userID, tutorialID string) error {
	return r.db.WithContext(ctx).Where("user_id = ? AND tutorial_id = ?", userID, tutorialID).Delete(&schema.TutorialProgress{}).Error
}
