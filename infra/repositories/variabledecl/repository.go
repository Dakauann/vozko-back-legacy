package variabledecl_repository

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domain "vozko/domain/variabledecl"
	"vozko/infra/database/schema"
)

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type spec struct {
	Example  string            `json:"example,omitempty"`
	Options  []string          `json:"options,omitempty"`
	Settings map[string]string `json:"settings,omitempty"`
}

func (r *Repository) List(ctx context.Context, workspaceID string) ([]domain.Declaration, error) {
	var rows []schema.ConversationVariableDeclaration
	if err := r.db.WithContext(ctx).Where("workspace_id = ?", workspaceID).Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Declaration, 0, len(rows))
	for _, row := range rows {
		var s spec
		if err := json.Unmarshal([]byte(row.Spec), &s); err != nil {
			return nil, fmt.Errorf("declaration %s of %s: %w", row.Name, workspaceID, err)
		}
		out = append(out, domain.Declaration{
			Name: row.Name, Label: row.Label, Description: row.Description, Example: s.Example,
			Type:      domain.Type{Kind: domain.Kind(row.Kind), Options: s.Options, Settings: s.Settings},
			UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

func (r *Repository) Save(ctx context.Context, workspaceID string, d domain.Declaration) error {
	raw, err := json.Marshal(spec{Example: d.Example, Options: d.Type.Options, Settings: d.Type.Settings})
	if err != nil {
		return err
	}
	row := schema.ConversationVariableDeclaration{
		WorkspaceID: workspaceID, Name: d.Name, Label: d.Label, Description: d.Description,
		Kind: string(d.Type.Kind), Spec: string(raw), UpdatedBy: d.UpdatedBy, CreatedAt: d.UpdatedAt, UpdatedAt: d.UpdatedAt,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"label", "description", "kind", "spec", "updated_by", "updated_at"}),
	}).Create(&row).Error
}

func (r *Repository) Delete(ctx context.Context, workspaceID, name string) (bool, error) {
	res := r.db.WithContext(ctx).Where("workspace_id = ? AND name = ?", workspaceID, name).Delete(&schema.ConversationVariableDeclaration{})
	return res.RowsAffected > 0, res.Error
}
