package container

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"vozko/domain/media"
	"vozko/domain/sitepublish"
	site_usecase "vozko/usecases/site"
)

func (m *e2eSiteStore) MarkPublished(_ context.Context, mark site_usecase.PublishMark) (site_usecase.PublishMarked, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[mark.ProjectID]
	if !ok || p.WorkspaceID != mark.WorkspaceID {
		return site_usecase.PublishMarked{}, site_usecase.ErrProjectNotFound
	}
	for _, b := range mark.Blobs {
		m.blobs[b.SHA256] = b.Bytes
	}
	m.revisions[mark.Revision.ID] = mark.Revision
	current := p.Version == mark.SourceVersion && p.WorkingRevisionID == mark.SourceRevisionID
	if current {
		p.WorkingRevisionID = mark.Revision.ID
		m.projects[p.ID] = p
	}
	return site_usecase.PublishMarked{Project: &p, Current: current}, nil
}

func (m *e2eDeployments) RecordPublished(_ context.Context, project, id, revisionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.rows[id]
	if !ok || rec.ProjectID != project {
		return sitepublish.ErrDeploymentNotFound
	}
	rec.RevisionID = revisionID
	m.rows[id] = rec
	return nil
}

func (m *e2eDeployments) LiveProjects(context.Context, string, int) ([]sitepublish.LiveProject, error) {
	return nil, nil
}

func (m *e2eHostnames) CreateCustom(ctx context.Context, rec sitepublish.HostnameRecord, allowed int) error {
	used, _ := m.CountCustom(ctx, rec.WorkspaceID)
	if used >= allowed {
		return sitepublish.ErrDomainQuota
	}
	return m.Create(ctx, rec)
}

func (m *e2eHostnames) ListCustomByWorkspace(_ context.Context, workspaceID string) ([]sitepublish.HostnameRecord, error) {
	out := m.sorted(func(r sitepublish.HostnameRecord) bool {
		return r.WorkspaceID == workspaceID && r.Kind == sitepublish.HostnameCustom
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *e2eHostnames) CustomWorkspaces(context.Context, string, int) ([]string, error) {
	return nil, nil
}

type e2eLocks struct {
	mu   sync.Mutex
	held map[string]bool
}

func (l *e2eLocks) SetNX(key, _ string, _ time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held[key] {
		return false, nil
	}
	l.held[key] = true
	return true, nil
}

func (l *e2eLocks) Del(keys ...string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		delete(l.held, k)
	}
	return nil
}

func (l *e2eLibrary) GetMediaForWorkspace(workspaceID, mediaID string) (*media.Media, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	row, ok := l.rows[mediaID]
	if !ok || row.WorkspaceID != workspaceID {
		return nil, media.ErrMediaNotFound
	}
	return &row, nil
}

func (l *e2eLibrary) KeyFromURL(url string) (string, bool) {
	return strings.CutPrefix(url, "https://cdn.vozko.test/")
}

func (l *e2eLibrary) DeleteFile(context.Context, string) error {
	return nil
}

type e2eCounters struct{}

func (e2eCounters) IncrWithTTL(string, time.Duration) (int64, error) {
	return 1, nil
}

func (e2eCounters) Del(...string) error {
	return nil
}

func (m *e2eHostnames) ListPage(context.Context, sitepublish.HostnamePage) ([]sitepublish.HostnameRecord, error) {
	return nil, nil
}
