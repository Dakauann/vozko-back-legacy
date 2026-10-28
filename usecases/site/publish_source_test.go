package site_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/site"
)

func (m *memoryStore) MarkPublished(_ context.Context, mark PublishMark) (PublishMarked, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fail(); err != nil {
		return PublishMarked{}, err
	}
	p, ok := m.projects[mark.ProjectID]
	if !ok || p.WorkspaceID != mark.WorkspaceID {
		return PublishMarked{}, ErrProjectNotFound
	}
	for _, b := range mark.Blobs {
		m.blobs[blobKey(mark.WorkspaceID, b.SHA256)] = b.Bytes
	}
	m.revisions[mark.Revision.ID] = mark.Revision
	current := p.Version == mark.SourceVersion && p.WorkingRevisionID == mark.SourceRevisionID
	if current {
		p.WorkingRevisionID = mark.Revision.ID
		m.projects[p.ID] = p
	}
	return PublishMarked{Project: &p, Current: current}, nil
}

func TestPublishSourceReadsTheWorkingRevisionWithoutCommitting(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Clínica")
	commits := len(h.store.commits)
	got, err := h.svc.PublishSource(context.Background(), wsA, v.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision.ID != v.Revision.ID || got.Project.Version != v.Project.Version || len(h.store.commits) != commits {
		t.Fatalf("publishing reads, never commits: %+v commits %d", got.Revision, len(h.store.commits)-commits)
	}
	if _, err := h.svc.PublishSource(context.Background(), "", v.Project.ID); !errors.Is(err, ErrWorkspaceRequired) {
		t.Fatalf("got %v", err)
	}
	archived := true
	if _, err := h.svc.Update(context.Background(), wsA, v.Project.ID, got.Project.Version, Change{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.PublishSource(context.Background(), wsA, v.Project.ID); !errors.Is(err, ErrProjectArchived) {
		t.Fatalf("an archived page is not published: %v", err)
	}
}

func TestThePublishMarkKeepsTheVersionAndMovesTheWorkingRevisionOnlyWhenUnchanged(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Clínica")
	marker, err := NewPublishMarker(h.store, h.clock.Now, func() string { return "44444444-4444-4444-8444-444444444444" })
	if err != nil {
		t.Fatal(err)
	}
	in := MarkPublishedInput{WorkspaceID: wsA, ProjectID: v.Project.ID, SourceRevisionID: v.Revision.ID, SourceVersion: v.Project.Version, Files: v.Files, Author: PersonAuthor(userA)}
	rev, marked, err := marker.Mark(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !marked.Current || marked.Project.Version != v.Project.Version || marked.Project.WorkingRevisionID != rev.ID {
		t.Fatalf("an unchanged page points at the publish mark without a new version: %+v", marked.Project)
	}
	if rev.Kind != RevisionPublish || rev.ParentID != v.Revision.ID || len(rev.Manifest) != len(v.Files) {
		t.Fatalf("mark %+v", rev)
	}
	edited, err := h.svc.SaveFiles(context.Background(), SaveFilesInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: v.Project.Version, Author: PersonAuthor(userA), Files: map[string]string{"pages/index.html": "<h1>Mudou</h1>"}})
	if err != nil {
		t.Fatalf("the editor saves on the version it had before publishing: %v", err)
	}
	stale := in
	stale.SourceVersion, stale.SourceRevisionID = v.Project.Version, rev.ID
	other, marked, err := marker.Mark(context.Background(), stale)
	if err != nil {
		t.Fatal(err)
	}
	if marked.Current || marked.Project.WorkingRevisionID != edited.Revision.ID || marked.Project.Version != edited.Project.Version || other.ID == "" {
		t.Fatalf("a page edited while publishing keeps the edit: %+v", marked.Project)
	}
}

func TestThePublishMarkRefusesWhatItCannotRecord(t *testing.T) {
	h := newHarness(t)
	marker, err := NewPublishMarker(h.store, h.clock.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string]MarkPublishedInput{
		"no author":    {WorkspaceID: wsA, ProjectID: "p", SourceRevisionID: "r", SourceVersion: 1, Files: map[string]site.File{"index.html": {Content: "x", Rev: 1}}},
		"no files":     {WorkspaceID: wsA, ProjectID: "p", SourceRevisionID: "r", SourceVersion: 1, Author: PersonAuthor(userA)},
		"no workspace": {ProjectID: "p", SourceRevisionID: "r", SourceVersion: 1, Author: PersonAuthor(userA), Files: map[string]site.File{"index.html": {Content: "x", Rev: 1}}},
		"no version":   {WorkspaceID: wsA, ProjectID: "p", SourceRevisionID: "r", Author: PersonAuthor(userA), Files: map[string]site.File{"index.html": {Content: "x", Rev: 1}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := marker.Mark(context.Background(), in); err == nil {
				t.Fatal("refused")
			}
		})
	}
	if _, err := NewPublishMarker(nil, nil, nil); !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("got %v", err)
	}
}
