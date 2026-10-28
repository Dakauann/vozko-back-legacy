package site_repository

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/infra/database/schema"
	site_usecase "vozko/usecases/site"
)

func designRecord(d site_usecase.DesignSystemRecord) (schema.SiteDesignSystem, error) {
	tokens, err := json.Marshal(nonNilTokens(d.System.Tokens))
	if err != nil {
		return schema.SiteDesignSystem{}, err
	}
	fonts, err := json.Marshal(nonNil(d.System.Fonts))
	if err != nil {
		return schema.SiteDesignSystem{}, err
	}
	snippets, err := json.Marshal(nonNil(d.System.Snippets))
	if err != nil {
		return schema.SiteDesignSystem{}, err
	}
	return schema.SiteDesignSystem{
		ID: d.ID, WorkspaceID: d.WorkspaceID, Name: d.System.Name, Tokens: tokens, Fonts: fonts, LogoMediaID: d.System.LogoMediaID,
		Voice: d.System.Voice, Snippets: snippets, IsDefault: d.IsDefault, Version: d.Version, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}, nil
}

func nonNilTokens(tokens map[string]string) map[string]string {
	if tokens == nil {
		return map[string]string{}
	}
	return tokens
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func toDesign(r schema.SiteDesignSystem) (site_usecase.DesignSystemRecord, error) {
	system := site.DesignSystem{Name: r.Name, LogoMediaID: r.LogoMediaID, Voice: r.Voice}
	for _, part := range []struct {
		raw  []byte
		into any
	}{{r.Tokens, &system.Tokens}, {r.Fonts, &system.Fonts}, {r.Snippets, &system.Snippets}} {
		if err := json.Unmarshal(part.raw, part.into); err != nil {
			return site_usecase.DesignSystemRecord{}, err
		}
	}
	if len(system.Tokens) == 0 {
		system.Tokens = nil
	}
	if len(system.Fonts) == 0 {
		system.Fonts = nil
	}
	if len(system.Snippets) == 0 {
		system.Snippets = nil
	}
	return site_usecase.DesignSystemRecord{
		ID: r.ID, WorkspaceID: r.WorkspaceID, System: system, IsDefault: r.IsDefault, Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, nil
}

func clearDefault(tx *gorm.DB, workspaceID, except string) error {
	return tx.Exec(`UPDATE site_design_systems SET is_default = false WHERE workspace_id = ? AND is_default AND id <> ?`, workspaceID, except).Error
}

func (r *Repository) CreateDesignSystem(ctx context.Context, d *site_usecase.DesignSystemRecord) error {
	if !validID(d.WorkspaceID) {
		return site_usecase.ErrWorkspaceRequired
	}
	record, err := designRecord(*d)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if d.IsDefault {
			if err := clearDefault(tx, d.WorkspaceID, d.ID); err != nil {
				return err
			}
		}
		return tx.Create(&record).Error
	})
}

func (r *Repository) GetDesignSystem(ctx context.Context, workspaceID, id string) (*site_usecase.DesignSystemRecord, error) {
	if !validID(workspaceID) || !validID(id) {
		return nil, site_usecase.ErrDesignSystemNotFound
	}
	var records []schema.SiteDesignSystem
	if err := r.db.WithContext(ctx).Where("workspace_id = ? AND id = ?", workspaceID, id).Limit(1).Find(&records).Error; err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, site_usecase.ErrDesignSystemNotFound
	}
	out, err := toDesign(records[0])
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *Repository) ListDesignSystems(ctx context.Context, workspaceID string) ([]site_usecase.DesignSystemRecord, error) {
	if !validID(workspaceID) {
		return nil, site_usecase.ErrWorkspaceRequired
	}
	var records []schema.SiteDesignSystem
	if err := r.db.WithContext(ctx).Where("workspace_id = ?", workspaceID).Order("updated_at DESC, id DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	out := make([]site_usecase.DesignSystemRecord, 0, len(records))
	for _, rec := range records {
		d, err := toDesign(rec)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (r *Repository) UpdateDesignSystem(ctx context.Context, d *site_usecase.DesignSystemRecord, expected int64) error {
	if !validID(d.WorkspaceID) || !validID(d.ID) {
		return site_usecase.ErrDesignSystemNotFound
	}
	record, err := designRecord(*d)
	if err != nil {
		return err
	}
	var saved []schema.SiteDesignSystem
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if d.IsDefault {
			if err := clearDefault(tx, d.WorkspaceID, d.ID); err != nil {
				return err
			}
		}
		return tx.Raw(`UPDATE site_design_systems SET name = ?, tokens = ?, fonts = ?, logo_media_id = ?, voice = ?, snippets = ?, is_default = ?,
			version = version + 1, updated_at = ? WHERE id = ? AND workspace_id = ? AND version = ? RETURNING *`,
			record.Name, record.Tokens, record.Fonts, record.LogoMediaID, record.Voice, record.Snippets, record.IsDefault,
			record.UpdatedAt, d.ID, d.WorkspaceID, expected).Scan(&saved).Error
	})
	if err != nil {
		return err
	}
	if len(saved) == 0 {
		if _, err := r.GetDesignSystem(ctx, d.WorkspaceID, d.ID); err != nil {
			return err
		}
		return shared.ErrVersionConflict
	}
	d.Version, d.UpdatedAt = saved[0].Version, saved[0].UpdatedAt
	return nil
}

func (r *Repository) DeleteDesignSystem(ctx context.Context, workspaceID, id string) error {
	if !validID(workspaceID) || !validID(id) {
		return site_usecase.ErrDesignSystemNotFound
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`DELETE FROM site_design_systems WHERE id = ? AND workspace_id = ?`, id, workspaceID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return site_usecase.ErrDesignSystemNotFound
		}
		return tx.Exec(`UPDATE site_projects SET design_system_id = NULL WHERE workspace_id = ? AND design_system_id = ?`, workspaceID, id).Error
	})
}
