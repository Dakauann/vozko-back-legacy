package sitehttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"vozko/domain/site"
	workspace_domain "vozko/domain/workspace"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siteinspect"
	"vozko/usecases/sitethumbnail"
)

type fakeSnapshots struct {
	got    []siteinspect.Snapshot
	report siteinspect.Report
}

func (f *fakeSnapshots) Submit(_ context.Context, _ *site_usecase.View, snapshot siteinspect.Snapshot) siteinspect.Report {
	f.got = append(f.got, snapshot)
	return f.report
}

type fakeUploads struct {
	got    [][]byte
	answer sitethumbnail.Upload
	err    error
}

func (f *fakeUploads) Upload(_ context.Context, workspaceID, projectID string, image []byte) (sitethumbnail.Upload, error) {
	f.got = append(f.got, image)
	return f.answer, f.err
}

type missingProjects struct{}

func (missingProjects) Get(context.Context, string, string) (*site_usecase.View, error) {
	return nil, site_usecase.ErrProjectNotFound
}

func captureRouter(svc *fakeService, snapshots *fakeSnapshots, uploads *fakeUploads) (*mux.Router, map[string]grant) {
	r := mux.NewRouter()
	grants := map[string]grant{}
	ac := func(res workspace_domain.Resource, action workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			tmpl, _ := mux.CurrentRoute(req).GetPathTemplate()
			grants[req.Method+" "+tmpl] = grant{res, action}
			next(w, req)
		}
	}
	RegisterCaptureRoutes(r, NewCaptureHandler(svc, snapshots, uploads), ac)
	return r, grants
}

func jpegURL(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1440, 900)), nil); err != nil {
		t.Fatal(err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestCapturesNeedTheRightToEditTheSite(t *testing.T) {
	r, grants := captureRouter(newFake(), &fakeSnapshots{}, &fakeUploads{})
	call(r, http.MethodPost, "/sites/projects/p-1/outline", `{"documents":{}}`, nil)
	call(r, http.MethodPost, "/sites/projects/p-1/thumbnail", `{"image":""}`, nil)
	for _, route := range []string{"POST /sites/projects/{id}/outline", "POST /sites/projects/{id}/thumbnail"} {
		if grants[route] != (grant{workspace_domain.ResourceSites, workspace_domain.ActionUpdate}) {
			t.Fatalf("%s guarded by %+v", route, grants[route])
		}
	}
}

func TestASubmittedSnapshotAnswersTheOutlineOfThatSnapshot(t *testing.T) {
	snapshots := &fakeSnapshots{report: siteinspect.Report{Known: true, Gaps: []site.Issue{{Code: site.CodeAltMissing, Route: "/"}}, Routes: []site.RouteOutline{{Route: "/", Rendered: true, Nodes: []site.OutlineNode{}}}}}
	r, _ := captureRouter(newFake(), snapshots, &fakeUploads{})
	rec := call(r, http.MethodPost, "/sites/projects/p-1/outline", `{"documents":{"/":"<!doctype html><html></html>"}}`, nil)
	if rec.Code != http.StatusOK || len(snapshots.got) != 1 || snapshots.got[0]["/"] != "<!doctype html><html></html>" {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var body OutlineResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.GapsChecked || len(body.OpenGaps) != 1 || body.Checks == nil {
		t.Fatalf("got %s", rec.Body.String())
	}
}

func TestAnUnreadableSnapshotNeverReachesTheInspection(t *testing.T) {
	cases := map[string]struct {
		body string
		code string
	}{
		"no documents":   {`{}`, "invalid_body"},
		"unknown field":  {`{"documents":{},"script":"x"}`, "invalid_body"},
		"not strings":    {`{"documents":{"/":1}}`, "invalid_body"},
		"two objects":    {`{"documents":{}}{"documents":{}}`, "invalid_body"},
		"over the limit": {`{"documents":{"/":"` + strings.Repeat("x", snapshotBodyBytes) + `"}}`, "project_too_large"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			snapshots := &fakeSnapshots{}
			r, _ := captureRouter(newFake(), snapshots, &fakeUploads{})
			rec := call(r, http.MethodPost, "/sites/projects/p-1/outline", c.body, nil)
			if rec.Code < 400 || decodeError(t, rec).Code != c.code || len(snapshots.got) != 0 {
				t.Fatalf("got %d %s", rec.Code, rec.Body.String())
			}
		})
	}
	snapshots, uploads := &fakeSnapshots{}, &fakeUploads{}
	r := mux.NewRouter()
	RegisterCaptureRoutes(r, NewCaptureHandler(missingProjects{}, snapshots, uploads), func(_ workspace_domain.Resource, _ workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		return next
	})
	if rec := call(r, http.MethodPost, "/sites/projects/p-9/outline", `{"documents":{}}`, nil); rec.Code != http.StatusNotFound || len(snapshots.got) != 0 {
		t.Fatalf("another workspace's page is not found: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(r, http.MethodPost, "/sites/projects/p-9/thumbnail", `{"image":"`+jpegURL(t)+`"}`, nil); rec.Code != http.StatusNotFound || len(uploads.got) != 0 {
		t.Fatalf("another workspace's page gets no thumbnail: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAThumbnailUploadAnswersWhenTheNextOneIsWelcome(t *testing.T) {
	uploads := &fakeUploads{answer: sitethumbnail.Upload{Accepted: false, RetryAfter: 361500 * time.Millisecond}}
	r, _ := captureRouter(newFake(), &fakeSnapshots{}, uploads)
	rec := call(r, http.MethodPost, "/sites/projects/p-1/thumbnail", `{"image":"`+jpegURL(t)+`"}`, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"accepted":false`) || !strings.Contains(rec.Body.String(), `"retryAfterSeconds":362`) || len(uploads.got) != 1 {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestAThumbnailThatIsNotAJPEGIsRefusedWithItsCode(t *testing.T) {
	cases := map[string]struct {
		image  string
		err    error
		status int
		code   string
	}{
		"svg":        {"data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte("<svg/>")), nil, http.StatusUnprocessableEntity, CodeThumbnailInvalid},
		"not base64": {"data:image/jpeg;base64,***", nil, http.StatusUnprocessableEntity, CodeThumbnailInvalid},
		"undecoded":  {jpegURL(t), sitethumbnail.ErrImageInvalid, http.StatusUnprocessableEntity, CodeThumbnailInvalid},
		"storage":    {jpegURL(t), errors.New("r2 down"), http.StatusInternalServerError, CodeInternal},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r, _ := captureRouter(newFake(), &fakeSnapshots{}, &fakeUploads{err: c.err})
			rec := call(r, http.MethodPost, "/sites/projects/p-1/thumbnail", `{"image":"`+c.image+`"}`, nil)
			if rec.Code != c.status || decodeError(t, rec).Code != c.code {
				t.Fatalf("got %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
