package container

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"vozko/domain/media"
	"vozko/domain/shared"
	"vozko/domain/site"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
	"vozko/usecases/siterender"
)

type e2eDesigns struct {
	mu   sync.Mutex
	rows map[string]site_usecase.DesignSystemRecord
}

func (d *e2eDesigns) create(rec *site_usecase.DesignSystemRecord) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rows == nil {
		d.rows = map[string]site_usecase.DesignSystemRecord{}
	}
	rec.Version = 1
	d.rows[rec.ID] = *rec
}

func (d *e2eDesigns) get(workspaceID, id string) (*site_usecase.DesignSystemRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rec, ok := d.rows[id]
	if !ok || rec.WorkspaceID != workspaceID {
		return nil, site_usecase.ErrDesignSystemNotFound
	}
	return &rec, nil
}

func (d *e2eDesigns) list(workspaceID string) []site_usecase.DesignSystemRecord {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []site_usecase.DesignSystemRecord
	for _, rec := range d.rows {
		if rec.WorkspaceID == workspaceID {
			out = append(out, rec)
		}
	}
	return out
}

func (d *e2eDesigns) update(rec *site_usecase.DesignSystemRecord, expected int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	current, ok := d.rows[rec.ID]
	if !ok {
		return site_usecase.ErrDesignSystemNotFound
	}
	if current.Version != expected {
		return shared.ErrVersionConflict
	}
	rec.Version = expected + 1
	d.rows[rec.ID] = *rec
	return nil
}

func (m *e2eSiteStore) SetThumbnail(_ context.Context, workspaceID, projectID, mediaID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[projectID]
	if !ok || p.WorkspaceID != workspaceID {
		return "", site_usecase.ErrProjectNotFound
	}
	previous := p.ThumbnailMediaID
	p.ThumbnailMediaID = mediaID
	m.projects[projectID] = p
	return previous, nil
}

func (b *replayBuilder) Extract(_ context.Context, req sitebuild.ExtractRequest) (sitebuild.ExtractResult, error) {
	b.mu.Lock()
	b.extracts = append(b.extracts, req)
	b.mu.Unlock()
	at := strings.LastIndex(req.Node, "#")
	offset, err := strconv.Atoi(req.Node[at+1:])
	file, ok := req.Project.Files[req.Node[:max(at, 0)]]
	if at < 0 || err != nil || !ok || offset >= len(file.Content) || file.Content[offset] != '<' {
		return sitebuild.ExtractResult{Failure: &sitebuild.ExecuteFailure{Code: site.CodeNodeStale, Field: "node"}}, nil
	}
	return sitebuild.ExtractResult{Source: strings.SplitN(file.Content[offset:], "\n", 2)[0]}, nil
}

func (r *replayRenderer) OpenGraph(_ context.Context, url string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.socials = append(r.socials, url)
	return []byte("og:" + url), nil
}

func (r *replayRenderer) Screenshot(_ context.Context, url string, viewport siterender.Viewport) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shots = append(r.shots, url)
	r.sizes = append(r.sizes, viewport)
	return []byte("thumb:" + url), nil
}

type e2eLibrary struct {
	mu      sync.Mutex
	rows    map[string]media.Media
	deleted []string
}

func (l *e2eLibrary) UploadMedia(workspaceID string, data []byte, name string, kind media.MediaType, description string) (media.Media, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.rows == nil {
		l.rows = map[string]media.Media{}
	}
	id := "media-" + strconv.Itoa(len(l.rows)+1)
	row := media.Media{ID: id, WorkspaceID: workspaceID, URL: "https://cdn.vozko.test/" + name, Type: kind, Description: description, CreatedAt: time.Now()}
	l.rows[id] = row
	return row, nil
}

func (l *e2eLibrary) DeleteMedia(mediaID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.deleted = append(l.deleted, mediaID)
	delete(l.rows, mediaID)
	return nil
}

func (l *e2eLibrary) Locate(_ context.Context, workspaceID string, ids []string) (map[string]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := map[string]string{}
	for _, id := range ids {
		if row, ok := l.rows[id]; ok && row.WorkspaceID == workspaceID {
			out[id] = row.URL
		}
	}
	return out, nil
}

func e2eJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := jpeg.Encode(&out, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

type e2eThrottle struct {
	mu   sync.Mutex
	keys map[string]bool
}

func (t *e2eThrottle) SetNX(key, _ string, _ time.Duration) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.keys == nil {
		t.keys = map[string]bool{}
	}
	if t.keys[key] {
		return false, nil
	}
	t.keys[key] = true
	return true, nil
}

func (t *e2eThrottle) GetString(string) (string, error) {
	return "", nil
}
