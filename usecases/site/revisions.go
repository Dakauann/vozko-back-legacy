package site_usecase

import (
	"context"
	"strings"

	"vozko/domain/site"
)

func (s *Service) ListRevisions(ctx context.Context, workspaceID, projectID string, limit, offset int) ([]Revision, int64, error) {
	if workspaceID == "" {
		return nil, 0, ErrWorkspaceRequired
	}
	if _, err := s.store.GetProject(ctx, workspaceID, projectID); err != nil {
		return nil, 0, err
	}
	return s.store.ListRevisions(ctx, workspaceID, projectID, min(max(limit, 1), MaxListLimit), max(offset, 0))
}

type SaveVersionInput struct {
	WorkspaceID     string
	ProjectID       string
	ExpectedVersion int64
	Author          Author
	Label           string
}

func (s *Service) SaveVersion(ctx context.Context, in SaveVersionInput) (*View, error) {
	if !in.Author.Valid() {
		return nil, ErrAuthorInvalid
	}
	if !validLabel(in.Label) {
		return nil, ErrLabelInvalid
	}
	base, err := s.editableAt(ctx, in.WorkspaceID, in.ProjectID, in.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	return s.commit(ctx, base, base.Files, revisionPlan{author: in.Author, kind: RevisionCheckpoint, label: strings.TrimSpace(in.Label)})
}

type RestoreInput struct {
	WorkspaceID     string
	ProjectID       string
	RevisionID      string
	ExpectedVersion int64
	Author          Author
}

func (s *Service) Restore(ctx context.Context, in RestoreInput) (*View, error) {
	if !in.Author.Valid() {
		return nil, ErrAuthorInvalid
	}
	base, err := s.editableAt(ctx, in.WorkspaceID, in.ProjectID, in.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	target, err := s.store.GetRevision(ctx, in.WorkspaceID, in.ProjectID, in.RevisionID)
	if err != nil {
		return nil, err
	}
	old, err := s.source(ctx, &base.Project, target)
	if err != nil {
		return nil, err
	}
	files := make(map[string]site.File, len(old))
	for path, file := range old {
		current, exists := base.Files[path]
		switch {
		case exists && current.Content == file.Content:
			files[path] = current
		case exists:
			files[path] = site.File{Content: file.Content, Rev: max(current.Rev, file.Rev) + 1}
		default:
			files[path] = site.File{Content: file.Content, Rev: file.Rev + 1}
		}
	}
	return s.commit(ctx, base, files, revisionPlan{author: in.Author, kind: RevisionRestore, label: target.Label})
}

func (s *Service) RevisionSource(ctx context.Context, workspaceID, projectID, revisionID string) (site.Project, error) {
	if workspaceID == "" {
		return site.Project{}, ErrWorkspaceRequired
	}
	p, err := s.store.GetProject(ctx, workspaceID, projectID)
	if err != nil {
		return site.Project{}, err
	}
	rev, err := s.store.GetRevision(ctx, workspaceID, projectID, revisionID)
	if err != nil {
		return site.Project{}, err
	}
	files, err := s.source(ctx, p, rev)
	if err != nil {
		return site.Project{}, err
	}
	return site.Project{Stack: p.Stack, Files: files}, nil
}
