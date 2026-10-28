package site_usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"vozko/domain/site"
)

func (s *Service) PublishSource(ctx context.Context, workspaceID, projectID string) (*View, error) {
	v, err := s.working(ctx, workspaceID, projectID)
	if err != nil {
		return nil, err
	}
	if err := editable(v.Project); err != nil {
		return nil, err
	}
	return v, nil
}

type PublishMark struct {
	WorkspaceID      string
	ProjectID        string
	SourceRevisionID string
	SourceVersion    int64
	Revision         Revision
	Blobs            []Blob
}

type PublishMarked struct {
	Project *Project
	Current bool
}

type PublishMarkStore interface {
	MarkPublished(ctx context.Context, mark PublishMark) (PublishMarked, error)
}

type MarkPublishedInput struct {
	WorkspaceID      string
	ProjectID        string
	SourceRevisionID string
	SourceVersion    int64
	Files            map[string]site.File
	Author           Author
}

type PublishMarker struct {
	store PublishMarkStore
	now   func() time.Time
	newID func() string
}

func NewPublishMarker(store PublishMarkStore, now func() time.Time, newID func() string) (*PublishMarker, error) {
	if store == nil {
		return nil, ErrMissingDependency
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if newID == nil {
		newID = uuid.NewString
	}
	return &PublishMarker{store: store, now: now, newID: newID}, nil
}

func (m *PublishMarker) Mark(ctx context.Context, in MarkPublishedInput) (Revision, PublishMarked, error) {
	switch {
	case !in.Author.Valid():
		return Revision{}, PublishMarked{}, ErrAuthorInvalid
	case in.WorkspaceID == "":
		return Revision{}, PublishMarked{}, ErrWorkspaceRequired
	case in.ProjectID == "" || in.SourceRevisionID == "" || in.SourceVersion < 1 || len(in.Files) == 0:
		return Revision{}, PublishMarked{}, ErrPublicationInvalid
	}
	now := m.now()
	manifest, blobs := encodeFiles(in.Files)
	rev := Revision{
		ID: m.newID(), WorkspaceID: in.WorkspaceID, ProjectID: in.ProjectID, ParentID: in.SourceRevisionID,
		Manifest: manifest, Author: in.Author, Kind: RevisionPublish, CreatedAt: now, UpdatedAt: now,
	}
	marked, err := m.store.MarkPublished(ctx, PublishMark{
		WorkspaceID: in.WorkspaceID, ProjectID: in.ProjectID, SourceRevisionID: in.SourceRevisionID, SourceVersion: in.SourceVersion, Revision: rev, Blobs: blobs,
	})
	if err != nil {
		return Revision{}, PublishMarked{}, err
	}
	return rev, marked, nil
}
