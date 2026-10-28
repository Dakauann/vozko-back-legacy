package siteform_repository

import (
	"context"

	"vozko/usecases/siteforms"
)

const selectWorkspaceFormsSQL = `SELECT k.project_id, p.name AS project_name, k.form_id, k.form_key, EXISTS (SELECT 1 FROM site_form_manifests m WHERE m.form_key = k.form_key AND m.active) AS live FROM site_form_keys k JOIN site_projects p ON p.id = k.project_id AND p.workspace_id = k.workspace_id WHERE k.workspace_id = ? ORDER BY p.name, k.form_id`

func (s *FormStore) WorkspaceForms(ctx context.Context, workspaceID string) ([]siteforms.PageForm, error) {
	var rows []struct {
		ProjectID   string
		ProjectName string
		FormID      string
		FormKey     string
		Live        bool
	}
	if err := s.db.WithContext(ctx).Raw(selectWorkspaceFormsSQL, workspaceID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]siteforms.PageForm, 0, len(rows))
	for _, r := range rows {
		out = append(out, siteforms.PageForm{ProjectID: r.ProjectID, ProjectName: r.ProjectName, FormID: r.FormID, FormKey: r.FormKey, Live: r.Live})
	}
	return out, nil
}
