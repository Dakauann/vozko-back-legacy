package sitepublish_usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

func ogPath(shot []byte) string {
	return sitepublish.MediaPathPrefix + sitepublish.SHA256Hex(shot) + ".jpg"
}

func TestPublishRendersTheSocialImageOfARouteWithoutOne(t *testing.T) {
	r := newRig(t)
	r.renderer.og = []byte("og-jpeg")
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.renderer.shots) != 1 || r.renderer.shots[0] != "https://cand.preview.test/" {
		t.Fatalf("the open graph image is rendered from the candidate: %v", r.renderer.shots)
	}
	if len(r.builder.requests) != 2 || r.builder.requests[0].OGImages != nil {
		t.Fatalf("the candidate is built again with the image: %+v", r.builder.requests)
	}
	want := "https://" + liveHost + ogPath([]byte("og-jpeg"))
	if got := r.builder.requests[1].OGImages["/"]; got != want {
		t.Fatalf("og image %q want %q", got, want)
	}
	manifest, err := sitepublish.DecodeDeploymentManifest(r.hosting.manifests[res.Deployment.ID])
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := manifest.Files[ogPath([]byte("og-jpeg"))]
	if !ok || entry.ContentType != "image/jpeg" || string(r.hosting.blobs[entry.SHA256]) != "og-jpeg" {
		t.Fatalf("the image is a deployment blob: %+v %v", entry, ok)
	}
}

func TestPublishKeepsTheSocialImageChosenInSettings(t *testing.T) {
	r := newRig(t)
	r.renderer.og = []byte("og-jpeg")
	r.projects.snap.Settings.Pages = map[string]site.PageMeta{"/": {OGMediaID: "og1"}}
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err != nil {
		t.Fatal(err)
	}
	if len(r.renderer.shots) != 0 || len(r.builder.requests) != 1 {
		t.Fatalf("shots %v builds %d", r.renderer.shots, len(r.builder.requests))
	}
}

func TestPublishGoesOnWithoutASocialImageWhenItCannotBeMade(t *testing.T) {
	r := newRig(t)
	r.renderer.ogErr = errors.New("render down")
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.builder.requests) != 1 {
		t.Fatalf("no second build without an image: %d", len(r.builder.requests))
	}
	manifest, _ := sitepublish.DecodeDeploymentManifest(r.hosting.manifests[res.Deployment.ID])
	for path := range manifest.Files {
		if strings.HasSuffix(path, ".jpg") {
			t.Fatalf("no image is served: %s", path)
		}
	}
}

type secondBuildFails struct {
	fakeBuilder
}

func (b *secondBuildFails) Candidate(ctx context.Context, req CandidateRequest) (Candidate, error) {
	c, err := b.fakeBuilder.Candidate(ctx, req)
	if len(b.requests) > 1 {
		return Candidate{}, errors.New("builder down")
	}
	return c, err
}

func TestPublishFallsBackToTheFirstBuildWhenTheImageBuildFails(t *testing.T) {
	r := newRig(t)
	r.renderer.og = []byte("og-jpeg")
	builder := &secondBuildFails{fakeBuilder: fakeBuilder{candidate: htmlCandidate(homeHTML)}}
	r.svc.Builder = builder
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _ := sitepublish.DecodeDeploymentManifest(r.hosting.manifests[res.Deployment.ID])
	if _, ok := manifest.Files[ogPath([]byte("og-jpeg"))]; ok || len(builder.requests) != 2 {
		t.Fatalf("an image without its page is never served: %d builds", len(builder.requests))
	}
}

func TestALivePublishCapturesTheThumbnailFromTheCandidate(t *testing.T) {
	r := newRig(t)
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err != nil {
		t.Fatal(err)
	}
	if len(r.thumbnails.published) != 1 || r.thumbnails.published[0] != (thumbnailCall{workspaceID: wsID, projectID: projectID, url: "https://cand.preview.test/"}) {
		t.Fatalf("got %+v", r.thumbnails.published)
	}
	r.builder.err = errors.New("builder down")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err == nil || len(r.thumbnails.published) != 1 {
		t.Fatalf("a failed publish captures nothing: %v", err)
	}
}
