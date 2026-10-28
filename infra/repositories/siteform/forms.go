package siteform_repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/site"
	"vozko/domain/siteform"
	"vozko/domain/strictjson"
	"vozko/infra/database/schema"
	"vozko/usecases/siteforms"
)

const (
	insertKeySQL          = `INSERT INTO site_form_keys (form_key, workspace_id, project_id, form_id, created_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`
	selectKeysSQL         = `SELECT form_id, form_key FROM site_form_keys WHERE workspace_id = ? AND project_id = ? AND form_id IN ?`
	retireManifestsSQL    = `UPDATE site_form_manifests SET active = false WHERE project_id = ? AND active`
	retireEmbedsSQL       = `UPDATE site_widget_embeds SET active = false WHERE project_id = ? AND active`
	insertEmbedSQL        = `INSERT INTO site_widget_embeds (deployment_id, widget_id, workspace_id, project_id, active, created_at) VALUES (?, ?, ?, ?, true, ?) ON CONFLICT (deployment_id, widget_id) DO UPDATE SET active = true`
	selectActiveFormSQL   = `SELECT workspace_id, project_id, deployment_id, form_key, form_id, manifest, settings, success_url FROM site_form_manifests WHERE form_key = ? AND active LIMIT 1`
	selectEmbedProjectSQL = `SELECT DISTINCT project_id FROM site_widget_embeds WHERE workspace_id = ? AND widget_id = ? AND active ORDER BY project_id`
)

var ErrStoredFormInvalid = errors.New("siteform repository: a stored form could not be read")

type FormStore struct {
	db  *gorm.DB
	now func() time.Time
}

func NewFormStore(db *gorm.DB) *FormStore {
	return &FormStore{db: db, now: func() time.Time { return time.Now().UTC() }}
}

func (s *FormStore) EnsureKeys(ctx context.Context, workspaceID, projectID string, proposed map[string]string) (map[string]string, error) {
	out := map[string]string{}
	if len(proposed) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(proposed))
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for formID, key := range proposed {
			ids = append(ids, formID)
			if err := tx.Exec(insertKeySQL, key, workspaceID, projectID, formID, s.now()).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		FormID  string
		FormKey string
	}
	if err := s.db.WithContext(ctx).Raw(selectKeysSQL, workspaceID, projectID, ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.FormID] = r.FormKey
	}
	return out, nil
}

func (s *FormStore) Activate(ctx context.Context, a siteforms.Activation) error {
	now := s.now()
	rows := make([]schema.SiteFormManifest, 0, len(a.Forms))
	for _, form := range a.Forms {
		manifest, err := json.Marshal(form.Manifest)
		if err != nil {
			return err
		}
		settings, err := json.Marshal(form.Settings)
		if err != nil {
			return err
		}
		rows = append(rows, schema.SiteFormManifest{
			ID: uuid.NewString(), WorkspaceID: a.WorkspaceID, ProjectID: a.ProjectID, DeploymentID: a.DeploymentID,
			FormKey: form.FormKey, FormID: form.FormID, Manifest: manifest, Settings: settings, SuccessURL: form.SuccessURL, Active: true, CreatedAt: now,
		})
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(retireManifestsSQL, a.ProjectID).Error; err != nil {
			return err
		}
		if err := tx.Exec(retireEmbedsSQL, a.ProjectID).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		for _, widget := range a.Widgets {
			if err := tx.Exec(insertEmbedSQL, a.DeploymentID, widget, a.WorkspaceID, a.ProjectID, now).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *FormStore) Deactivate(ctx context.Context, projectID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(retireManifestsSQL, projectID).Error; err != nil {
			return err
		}
		return tx.Exec(retireEmbedsSQL, projectID).Error
	})
}

func (s *FormStore) ActiveForm(ctx context.Context, formKey string) (*siteforms.ActiveForm, error) {
	var rows []schema.SiteFormManifest
	if err := s.db.WithContext(ctx).Raw(selectActiveFormSQL, formKey).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]
	manifest, err := siteform.DecodeManifest(row.Manifest)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStoredFormInvalid, err)
	}
	var settings site.FormSettings
	if err := strictjson.Decode(row.Settings, &settings); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStoredFormInvalid, err)
	}
	return &siteforms.ActiveForm{
		WorkspaceID: row.WorkspaceID, ProjectID: row.ProjectID, DeploymentID: row.DeploymentID, FormKey: row.FormKey,
		FormID: row.FormID, Manifest: manifest, Settings: settings, SuccessURL: row.SuccessURL,
	}, nil
}

func (s *FormStore) ProjectsEmbedding(ctx context.Context, workspaceID, widgetID string) ([]string, error) {
	var projects []string
	if err := s.db.WithContext(ctx).Raw(selectEmbedProjectSQL, workspaceID, widgetID).Scan(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}
