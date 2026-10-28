package sitethumbnail

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"strings"
	"sync"
	"testing"
	"time"

	"vozko/domain/media"
	"vozko/usecases/siterender"
)

type fakeRenderer struct {
	calls []string
	sizes []siterender.Viewport
	err   error
}

func (f *fakeRenderer) Screenshot(_ context.Context, url string, viewport siterender.Viewport) ([]byte, error) {
	f.calls = append(f.calls, url)
	f.sizes = append(f.sizes, viewport)
	if f.err != nil {
		return nil, f.err
	}
	return []byte("jpeg"), nil
}

type upload struct {
	workspaceID string
	name        string
	kind        media.MediaType
}

type fakeUploader struct {
	uploads []upload
	err     error
	n       int
}

func (f *fakeUploader) UploadMedia(workspaceID string, data []byte, name string, kind media.MediaType, _ string) (media.Media, error) {
	if f.err != nil {
		return media.Media{}, f.err
	}
	f.n++
	f.uploads = append(f.uploads, upload{workspaceID: workspaceID, name: name, kind: kind})
	id := "media-" + string(rune('0'+f.n))
	return media.Media{ID: id, WorkspaceID: workspaceID, URL: "https://cdn.vozko.test/" + id + ".jpg", Type: kind}, nil
}

type fakeProjects struct {
	current map[string]string
	err     error
}

func (f *fakeProjects) SetThumbnail(_ context.Context, workspaceID, projectID, mediaID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	previous := f.current[workspaceID+"/"+projectID]
	f.current[workspaceID+"/"+projectID] = mediaID
	return previous, nil
}

type fakeRemover struct {
	uploader *fakeUploader
	deleted  []string
	err      error
}

func (f *fakeRemover) DeleteMedia(mediaID string) error {
	f.deleted = append(f.deleted, mediaID)
	return f.err
}

func (f *fakeRemover) GetMediaForWorkspace(workspaceID, mediaID string) (*media.Media, error) {
	for i := range f.uploader.uploads {
		id := "media-" + string(rune('1'+i))
		if id == mediaID && f.uploader.uploads[i].workspaceID == workspaceID {
			return &media.Media{ID: id, WorkspaceID: workspaceID, URL: "https://cdn.vozko.test/" + id + ".jpg"}, nil
		}
	}
	return nil, media.ErrMediaNotFound
}

type fakeFiles struct {
	deleted []string
	err     error
}

func (f *fakeFiles) KeyFromURL(url string) (string, bool) {
	return strings.CutPrefix(url, "https://cdn.vozko.test/")
}

func (f *fakeFiles) DeleteFile(_ context.Context, key string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, key)
	return nil
}

type fakeThrottle struct {
	mu     sync.Mutex
	keys   map[string]time.Duration
	values map[string]string
	err    error
}

func (f *fakeThrottle) SetNX(key, value string, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	if _, ok := f.keys[key]; ok {
		return false, nil
	}
	f.keys[key], f.values[key] = ttl, value
	return true, nil
}

func (f *fakeThrottle) GetString(key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	return f.values[key], nil
}

type world struct {
	now      time.Time
	svc      *Service
	renderer *fakeRenderer
	uploader *fakeUploader
	projects *fakeProjects
	remover  *fakeRemover
	files    *fakeFiles
	throttle *fakeThrottle
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{
		renderer: &fakeRenderer{}, uploader: &fakeUploader{}, projects: &fakeProjects{current: map[string]string{}},
		files: &fakeFiles{}, throttle: &fakeThrottle{keys: map[string]time.Duration{}, values: map[string]string{}},
		now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC),
	}
	w.remover = &fakeRemover{uploader: w.uploader}
	svc, err := New(Deps{
		Renderer: w.renderer, Uploader: w.uploader, Projects: w.projects, Media: w.remover, Files: w.files, Throttle: w.throttle,
		Spawn: func(fn func()) { fn() }, NewID: func() string { return "f1" }, Now: func() time.Time { return w.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	w.svc = svc
	return w
}

func TestNewRefusesMissingDependencies(t *testing.T) {
	if _, err := New(Deps{}); err == nil {
		t.Fatal("expected a refusal")
	}
}

func TestAPublishCapturesADesktopThumbnailIntoTheLibrary(t *testing.T) {
	w := newWorld(t)
	w.svc.Published(context.Background(), "ws1", "p1", "https://tok.previa.test/")
	if len(w.renderer.calls) != 1 || w.renderer.calls[0] != "https://tok.previa.test/" || w.renderer.sizes[0] != siterender.Desktop {
		t.Fatalf("renderer %+v %+v", w.renderer.calls, w.renderer.sizes)
	}
	if len(w.uploader.uploads) != 1 || w.uploader.uploads[0] != (upload{workspaceID: "ws1", name: "images/ws1/f1.jpg", kind: media.MediaTypeProductImage}) {
		t.Fatalf("uploads %+v", w.uploader.uploads)
	}
	if w.uploader.uploads[0].kind.Private() {
		t.Fatal("a thumbnail is never private")
	}
	if w.projects.current["ws1/p1"] != "media-1" || len(w.remover.deleted) != 0 {
		t.Fatalf("projects %+v deleted %+v", w.projects.current, w.remover.deleted)
	}
	w.svc.Published(context.Background(), "ws1", "p1", "https://tok.previa.test/")
	if w.projects.current["ws1/p1"] != "media-2" || len(w.remover.deleted) != 1 || w.remover.deleted[0] != "media-1" {
		t.Fatalf("a new thumbnail replaces the previous library entry: %+v %+v", w.projects.current, w.remover.deleted)
	}
	if len(w.files.deleted) != 1 || w.files.deleted[0] != "media-1.jpg" {
		t.Fatalf("the previous stored object is deleted too: %+v", w.files.deleted)
	}
	w.files.err = errors.New("r2 down")
	w.svc.Published(context.Background(), "ws1", "p1", "https://tok.previa.test/")
	if len(w.remover.deleted) != 1 {
		t.Fatalf("a row whose object could not be deleted stays: %+v", w.remover.deleted)
	}
}

func editorShot(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAnEditorUploadReplacesTheThumbnailAtMostOncePerSpacing(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	first, err := w.svc.Upload(ctx, "ws1", "p1", editorShot(t, 1440, 900))
	if err != nil || !first.Accepted || first.RetryAfter != PreviewSpacing {
		t.Fatalf("Upload() = %+v, %v", first, err)
	}
	if len(w.uploader.uploads) != 1 || w.uploader.uploads[0] != (upload{workspaceID: "ws1", name: "images/ws1/f1.jpg", kind: media.MediaTypeProductImage}) || w.projects.current["ws1/p1"] != "media-1" {
		t.Fatalf("uploads %+v projects %+v", w.uploader.uploads, w.projects.current)
	}
	if w.throttle.keys["site:thumbnail:p1"] != PreviewSpacing {
		t.Fatalf("keys %+v", w.throttle.keys)
	}
	w.now = w.now.Add(4 * time.Minute)
	second, err := w.svc.Upload(ctx, "ws1", "p1", editorShot(t, 1440, 900))
	if err != nil || second.Accepted || second.RetryAfter != 6*time.Minute || len(w.uploader.uploads) != 1 {
		t.Fatalf("a second upload inside the spacing is declined with the time left: %+v %v", second, err)
	}
	other, err := w.svc.Upload(ctx, "ws1", "p2", editorShot(t, 1440, 900))
	if err != nil || !other.Accepted || len(w.uploader.uploads) != 2 {
		t.Fatalf("the spacing is per project: %+v %v", other, err)
	}
	w.throttle.err = errors.New("redis down")
	down, err := w.svc.Upload(ctx, "ws1", "p3", editorShot(t, 1440, 900))
	if err != nil || down.Accepted || down.RetryAfter != PreviewSpacing || len(w.uploader.uploads) != 2 {
		t.Fatalf("an unreadable throttle stores nothing: %+v %v", down, err)
	}
}

func TestAnEditorUploadIsCheckedBeforeItSpendsTheSpacing(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	for name, data := range map[string][]byte{
		"html":  []byte("<html><script>alert(1)</script></html>"),
		"tiny":  editorShot(t, 40, 40),
		"huge":  editorShot(t, 3000, 900),
		"empty": nil,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := w.svc.Upload(ctx, "ws1", "p1", data); !errors.Is(err, ErrImageInvalid) {
				t.Fatalf("Upload() = %v, want ErrImageInvalid", err)
			}
		})
	}
	if len(w.throttle.keys) != 0 || len(w.uploader.uploads) != 0 {
		t.Fatalf("a refused image never spends the spacing: %+v", w.throttle.keys)
	}
}

func TestCaptureFailuresNeverLeaveAThumbnailBehind(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	w.renderer.err = errors.New("render down")
	if err := w.svc.Capture(ctx, "ws1", "p1", "https://tok.previa.test/"); err == nil || len(w.uploader.uploads) != 0 {
		t.Fatalf("a failed render uploads nothing: %v", err)
	}
	w.renderer.err = nil
	w.projects.err = errors.New("project gone")
	if err := w.svc.Capture(ctx, "ws1", "p1", "https://tok.previa.test/"); err == nil || len(w.remover.deleted) != 1 || w.remover.deleted[0] != "media-1" || len(w.files.deleted) != 1 {
		t.Fatalf("an upload that cannot be attached is removed again: %v %+v", err, w.remover.deleted)
	}
	w.projects.err = nil
	w.uploader.err = errors.New("library full")
	if err := w.svc.Capture(ctx, "ws1", "p1", "https://tok.previa.test/"); err == nil || !strings.Contains(err.Error(), "library full") {
		t.Fatalf("got %v", err)
	}
	w.uploader.err = nil
	w.svc.Published(ctx, "ws1", "p1", "")
	if len(w.renderer.calls) != 3 {
		t.Fatal("an empty address renders nothing")
	}
}
