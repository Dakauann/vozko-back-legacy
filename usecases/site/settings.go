package site_usecase

import (
	"context"
	"maps"
	"reflect"
	"slices"

	"vozko/domain/shared"
	"vozko/domain/site"
)

func (s *Service) GetSettings(ctx context.Context, workspaceID, projectID string) (*StoredSettings, error) {
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	if _, err := s.store.GetProject(ctx, workspaceID, projectID); err != nil {
		return nil, err
	}
	return s.store.GetSettings(ctx, workspaceID, projectID)
}

func (s *Service) PutSettings(ctx context.Context, workspaceID, projectID string, expected int64, raw []byte) (*StoredSettings, error) {
	if err := shared.RequireVersion(expected); err != nil {
		return nil, err
	}
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	p, err := s.store.GetProject(ctx, workspaceID, projectID)
	if err != nil {
		return nil, err
	}
	if err := editable(*p); err != nil {
		return nil, err
	}
	current, err := s.store.GetSettings(ctx, workspaceID, projectID)
	if err != nil {
		return nil, err
	}
	if err := shared.ExpectVersion(current.Version, &expected); err != nil {
		return nil, err
	}
	settings, err := site.DecodeSettings(raw)
	if err != nil {
		return nil, err
	}
	ids := slices.Sorted(maps.Keys(settings.Forms))
	for _, id := range ids {
		if err := s.references.Check(ctx, workspaceID, id, settings.Forms[id]); err != nil {
			return nil, err
		}
		if before, kept := current.Settings.Forms[id]; kept && reflect.DeepEqual(before, settings.Forms[id]) {
			continue
		}
		if err := settings.Forms[id].CheckDealOwner(id); err != nil {
			return nil, err
		}
	}
	if ref := settings.Site.WebchatWidget; ref != "" && ref != current.Settings.Site.WebchatWidget {
		if err := s.references.CheckWebchat(ctx, workspaceID, ref); err != nil {
			return nil, err
		}
	}
	keyed := slices.Sorted(maps.Keys(settings.Bookings))
	for _, id := range ids {
		if _, booking := settings.Bookings[id]; !booking {
			keyed = append(keyed, id)
		}
	}
	slices.Sort(keyed)
	if err := s.bind(ctx, workspaceID, projectID, keyed, current.Settings.Forms, settings.Forms); err != nil {
		return nil, err
	}
	next := &StoredSettings{ProjectID: projectID, WorkspaceID: workspaceID, Settings: settings, UpdatedAt: s.now()}
	if err := s.store.SaveSettings(ctx, next, expected); err != nil {
		return nil, err
	}
	return next, nil
}

func (s *Service) bind(ctx context.Context, workspaceID, projectID string, ids []string, before, forms map[string]site.FormSettings) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := s.bindings.KeysFor(ctx, workspaceID, projectID, ids); err != nil {
		return err
	}
	for _, id := range ids {
		workflowID := forms[id].WorkflowID
		if workflowID == "" || workflowID == before[id].WorkflowID {
			continue
		}
		bound, err := s.bindings.WorkflowBound(ctx, workspaceID, projectID, id, workflowID)
		if err != nil {
			return err
		}
		if !bound {
			return &FormReferenceError{Form: id, Err: ErrFormWorkflowNotBound}
		}
	}
	return nil
}
