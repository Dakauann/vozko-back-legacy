package siterender

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"sync"
	"testing"
	"time"

	"vozko/domain/shared"
)

const testPlatformDomain = "paginas.test"

type fakeBrowser struct {
	mu    sync.Mutex
	block chan struct{}

	prerenderJob PrerenderJob
	prerendered  map[string]string

	screenshotJob ScreenshotJob

	referenceJob ReferenceJob
	reference    ReferenceCapture
	calls        int

	liveJob LiveDOMJob
	live    map[string]string
	liveErr error
}

func (f *fakeBrowser) LiveDOM(_ context.Context, job LiveDOMJob) (map[string]string, error) {
	f.calls++
	f.liveJob = job
	return f.live, f.liveErr
}

func (f *fakeBrowser) Prerender(_ context.Context, job PrerenderJob) (map[string]string, error) {
	f.calls++
	f.prerenderJob = job
	return f.prerendered, nil
}

func (f *fakeBrowser) Screenshot(ctx context.Context, job ScreenshotJob) ([]byte, error) {
	f.mu.Lock()
	f.calls++
	f.screenshotJob = job
	block := f.block
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return jpegOf(job.Viewport.Width, job.Viewport.Height), nil
}

func (f *fakeBrowser) Reference(_ context.Context, job ReferenceJob) (ReferenceCapture, error) {
	f.calls++
	f.referenceJob = job
	return f.reference, nil
}

type fakeHosts struct {
	blocked map[string]bool
	checked []string
}

func (h *fakeHosts) CheckHost(_ context.Context, host string) error {
	h.checked = append(h.checked, host)
	if h.blocked[host] {
		return shared.ErrEgressBlocked
	}
	return nil
}

func jpegOf(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}

func pageCapture(v Viewport, pageHeight int, tops ...int) ViewportCapture {
	c := ViewportCapture{Viewport: v, PageHeight: pageHeight}
	for _, top := range tops {
		c.Slices = append(c.Slices, Frame{Top: top, JPEG: jpegOf(v.Width, v.Height)})
	}
	return c
}

func newTestService(b Browser, h HostChecker) *Service {
	return NewService(b, h, testPlatformDomain, 2)
}

func TestScreenshotRefusesAddressesOutsideThePreviewDomain(t *testing.T) {
	for _, raw := range []string{
		"https://evil.test/",
		"https://paginas.test.evil.test/",
		"https://user:pass@abc.paginas.test/",
		"javascript:alert(1)",
		"ftp://abc.paginas.test/",
		"",
		"https://xpaginas.test/",
	} {
		b := &fakeBrowser{}
		_, err := newTestService(b, &fakeHosts{}).Screenshot(context.Background(), raw, OpenGraph)
		if !errors.Is(err, ErrURLRefused) || ErrorCode(err) != CodeURLRefused {
			t.Fatalf("%q: err %v", raw, err)
		}
		if b.calls != 0 {
			t.Fatalf("%q reached the browser", raw)
		}
	}
}

func TestScreenshotPolicyStaysInsideThePreviewDomain(t *testing.T) {
	b := &fakeBrowser{}
	if _, err := newTestService(b, &fakeHosts{}).Screenshot(context.Background(), "https://abc.paginas.test/", Desktop); err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	policy := b.screenshotJob.Policy
	ctx := context.Background()
	if !policy.Allow(ctx, "abc.paginas.test") || !policy.Allow(ctx, "paginas.test") {
		t.Fatalf("the policy refuses the preview domain")
	}
	if policy.Allow(ctx, "evil.test") || policy.Allow(ctx, "paginas.test.evil.test") || policy.Allow(ctx, "xpaginas.test") {
		t.Fatalf("the policy allows hosts outside the preview domain")
	}
}

func TestRenderTimesOutAndFreesItsSlot(t *testing.T) {
	b := &fakeBrowser{block: make(chan struct{})}
	s := NewService(b, &fakeHosts{}, testPlatformDomain, 1)
	s.timeouts.screenshot = 50 * time.Millisecond
	_, err := s.Screenshot(context.Background(), "https://abc.paginas.test/", Desktop)
	if !errors.Is(err, ErrTimeout) || ErrorCode(err) != CodeTimeout {
		t.Fatalf("err %v", err)
	}
	close(b.block)
	if _, err := s.Screenshot(context.Background(), "https://abc.paginas.test/", Desktop); err != nil {
		t.Fatalf("the slot was not released: %v", err)
	}
}

func TestRenderWaitsForAFreeSlotAndGivesUpWithTheCaller(t *testing.T) {
	b := &fakeBrowser{block: make(chan struct{})}
	s := NewService(b, &fakeHosts{}, testPlatformDomain, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		_, _ = s.Screenshot(context.Background(), "https://abc.paginas.test/", Desktop)
	}()
	<-started
	deadline := time.Now().Add(time.Second)
	for {
		b.mu.Lock()
		calls := b.calls
		b.mu.Unlock()
		if calls == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := s.Screenshot(ctx, "https://abc.paginas.test/", Desktop)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err %v", err)
	}
	b.mu.Lock()
	calls := b.calls
	b.mu.Unlock()
	if calls != 1 {
		t.Fatalf("the waiting call reached the browser (%d calls)", calls)
	}
	close(b.block)
}

func TestPrerenderOpensTheEntryOnTheCandidateHostOnly(t *testing.T) {
	b := &fakeBrowser{prerendered: map[string]string{"/": "<div>home</div>", "/precos": "<div>p</div>"}}
	got, err := newTestService(b, &fakeHosts{}).Prerender(context.Background(), PrerenderRequest{URL: "https://r1abc.paginas.test/", Routes: []string{"/", "/precos", "/"}})
	if err != nil {
		t.Fatalf("prerender: %v", err)
	}
	if b.prerenderJob.URL != "https://r1abc.paginas.test/__vz/prerender.html" {
		t.Fatalf("entry url %q", b.prerenderJob.URL)
	}
	if len(b.prerenderJob.Routes) != 2 {
		t.Fatalf("routes %v", b.prerenderJob.Routes)
	}
	ctx := context.Background()
	if !b.prerenderJob.Policy.Allow(ctx, "r1abc.paginas.test") || b.prerenderJob.Policy.Allow(ctx, "other.paginas.test") || b.prerenderJob.Policy.Allow(ctx, "paginas.test") {
		t.Fatalf("the prerender policy is not the candidate host only")
	}
	if got["/precos"] != "<div>p</div>" {
		t.Fatalf("result %v", got)
	}
}

func TestPrerenderFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		routes []string
		result map[string]string
		want   error
	}{
		{"no routes", nil, nil, ErrRequestInvalid},
		{"bad route", []string{"/../etc"}, nil, ErrRequestInvalid},
		{"missing route in the result", []string{"/", "/a"}, map[string]string{"/": "<p>x</p>"}, ErrPrerenderFailed},
		{"empty markup", []string{"/"}, map[string]string{"/": " "}, ErrPrerenderFailed},
		{"oversized markup", []string{"/"}, map[string]string{"/": strings.Repeat("a", maxPrerenderHTMLBytes+1)}, ErrPrerenderFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &fakeBrowser{prerendered: c.result}
			_, err := newTestService(b, &fakeHosts{}).Prerender(context.Background(), PrerenderRequest{URL: "https://r1.paginas.test/", Routes: c.routes})
			if !errors.Is(err, c.want) {
				t.Fatalf("err %v, want %v", err, c.want)
			}
		})
	}
}

func TestScreenshotValidatesTheViewportAndTheHost(t *testing.T) {
	b := &fakeBrowser{}
	s := newTestService(b, &fakeHosts{})
	data, err := s.Screenshot(context.Background(), "https://r1.paginas.test/", OpenGraph)
	if err != nil || len(data) == 0 {
		t.Fatalf("screenshot: %v", err)
	}
	if b.screenshotJob.Viewport != OpenGraph || !b.screenshotJob.Policy.Allow(context.Background(), "cdn.paginas.test") {
		t.Fatalf("job %+v", b.screenshotJob)
	}
	if _, err := s.Screenshot(context.Background(), "https://r1.paginas.test/", Viewport{Width: 10, Height: 10}); !errors.Is(err, ErrRequestInvalid) {
		t.Fatalf("tiny viewport: %v", err)
	}
	if _, err := s.Screenshot(context.Background(), "https://example.com/", OpenGraph); !errors.Is(err, ErrURLRefused) {
		t.Fatalf("foreign host: %v", err)
	}
}

func TestCaptureReferenceRefusesPrivateAddressesBeforeRendering(t *testing.T) {
	for _, raw := range []string{"http://metadata.internal/", "file:///etc/passwd", "https://user@example.com/", "https://example.com:8443/", "https://"} {
		b := &fakeBrowser{}
		hosts := &fakeHosts{blocked: map[string]bool{"metadata.internal": true}}
		_, err := newTestService(b, hosts).CaptureReference(context.Background(), raw)
		if !errors.Is(err, ErrReferenceRefused) || ErrorCode(err) != CodeReferenceRefused {
			t.Fatalf("%q: err %v", raw, err)
		}
		if b.calls != 0 {
			t.Fatalf("%q reached the browser", raw)
		}
	}
}

func TestCaptureReferenceRefusesARedirectToABlockedHost(t *testing.T) {
	b := &fakeBrowser{reference: ReferenceCapture{URL: "http://10.0.0.5/admin", Viewports: []ViewportCapture{pageCapture(Desktop, 900, 0)}}}
	hosts := &fakeHosts{blocked: map[string]bool{"10.0.0.5": true}}
	_, err := newTestService(b, hosts).CaptureReference(context.Background(), "https://example.com/")
	if !errors.Is(err, ErrReferenceRefused) {
		t.Fatalf("err %v", err)
	}
}

func TestCaptureReferenceReturnsShotsSheetAndTokens(t *testing.T) {
	b := &fakeBrowser{reference: ReferenceCapture{
		URL:       "https://www.example.com/",
		Title:     "Example",
		Viewports: []ViewportCapture{pageCapture(Desktop, 1800, 0, 900), pageCapture(Mobile, 844, 0)},
		Samples: TokenSamples{
			Backgrounds: []ColorSample{{RGB: [3]int{255, 255, 255}, Weight: 10}},
			Fonts:       []FontSample{{Family: "Inter", Weight: 1}},
			Spacing:     []float64{8, 16, 24},
		},
	}}
	hosts := &fakeHosts{blocked: map[string]bool{"10.0.0.1": true}}
	ref, err := newTestService(b, hosts).CaptureReference(context.Background(), "https://example.com/")
	if err != nil {
		t.Fatalf("reference: %v", err)
	}
	if ref.URL != "https://www.example.com/" || ref.Title != "Example" || len(ref.Shots) != 2 {
		t.Fatalf("reference %+v", ref)
	}
	if len(ref.Image) == 0 || len(ref.Image) > MaxImageBytes {
		t.Fatalf("sheet has %d bytes", len(ref.Image))
	}
	if len(ref.Tokens.Backgrounds) != 1 || ref.Tokens.Fonts[0].Family != "Inter" || ref.Tokens.Spacing.Base != 8 {
		t.Fatalf("tokens %+v", ref.Tokens)
	}
	ctx := context.Background()
	if !b.referenceJob.Policy.Allow(ctx, "cdn.example.com") || b.referenceJob.Policy.Allow(ctx, "10.0.0.1") {
		t.Fatalf("the reference policy does not follow the host guard")
	}
	if len(b.referenceJob.Viewports) != 2 || b.referenceJob.MaxSlices != referenceSlices {
		t.Fatalf("job %+v", b.referenceJob)
	}
}
