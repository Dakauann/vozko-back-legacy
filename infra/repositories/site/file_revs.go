package site_repository

import (
	"context"
	"maps"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/infra/database/schema"
	site_usecase "vozko/usecases/site"
)

const FileRevRetention = 50

func insertFileRevs(tx *gorm.DB, workspaceID, projectID string, revs []site_usecase.FileRev, now time.Time) error {
	if len(revs) == 0 {
		return nil
	}
	records := make([]schema.SiteFileRev, 0, len(revs))
	paths := map[string]bool{}
	for _, r := range revs {
		records = append(records, schema.SiteFileRev{WorkspaceID: workspaceID, ProjectID: projectID, Path: r.Path, Rev: int64(r.Rev), SHA256: r.SHA256, CreatedAt: now})
		paths[r.Path] = true
	}
	err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}, {Name: "path"}, {Name: "rev"}},
		DoUpdates: clause.AssignmentColumns([]string{"sha256", "created_at"}),
	}).CreateInBatches(&records, blobBatchSize).Error
	if err != nil {
		return err
	}
	return tx.Exec(`DELETE FROM site_file_revs f USING (SELECT path, rev, row_number() OVER (PARTITION BY path ORDER BY rev DESC) AS n FROM site_file_revs WHERE project_id = ? AND path IN ?) ranked
		WHERE f.project_id = ? AND f.path = ranked.path AND f.rev = ranked.rev AND ranked.n > ?`,
		projectID, slices.Sorted(maps.Keys(paths)), projectID, FileRevRetention).Error
}

func (r *Repository) FileRevs(ctx context.Context, workspaceID, projectID string, wants map[string][]uint64) (map[string]map[uint64]string, error) {
	if !validID(workspaceID) || !validID(projectID) {
		return nil, site_usecase.ErrProjectNotFound
	}
	out := map[string]map[uint64]string{}
	var pairs [][]any
	for _, path := range slices.Sorted(maps.Keys(wants)) {
		for _, rev := range wants[path] {
			pairs = append(pairs, []any{path, int64(rev)})
		}
	}
	if len(pairs) == 0 {
		return out, nil
	}
	var records []schema.SiteFileRev
	err := r.db.WithContext(ctx).Select("path", "rev", "sha256").
		Where("workspace_id = ? AND project_id = ? AND (path, rev) IN ?", workspaceID, projectID, pairs).Find(&records).Error
	if err != nil {
		return nil, err
	}
	for _, rec := range records {
		if out[rec.Path] == nil {
			out[rec.Path] = map[uint64]string{}
		}
		out[rec.Path][uint64(rec.Rev)] = rec.SHA256
	}
	return out, nil
}
