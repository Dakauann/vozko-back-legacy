package siterender

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"vozko/domain/hostname"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

const (
	DefaultConcurrency    = 2
	minViewportWidth      = 320
	maxViewportWidth      = 2560
	minViewportHeight     = 320
	maxViewportHeight     = 2000
	referenceSlices       = 4
	maxPrerenderRoutes    = 50
	maxPrerenderHTMLBytes = 5 << 20
	prerenderEntry        = "/__vz/prerender.html"
)

type timeouts struct {
	prerender  time.Duration
	screenshot time.Duration
	reference  time.Duration
}

type Service struct {
	browser  Browser
	hosts    HostChecker
	domain   string
	slots    chan struct{}
	timeouts timeouts
}

var _ Renderer = (*Service)(nil)

func NewService(browser Browser, hosts HostChecker, platformDomain string, concurrency int) *Service {
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	return &Service{
		browser: browser,
		hosts:   hosts,
		domain:  hostname.Normalize(platformDomain),
		slots:   make(chan struct{}, concurrency),
		timeouts: timeouts{
			prerender:  120 * time.Second,
			screenshot: 45 * time.Second,
			reference:  60 * time.Second,
		},
	}
}

type domainPolicy struct{ domain string }

func (p domainPolicy) Allow(_ context.Context, host string) bool {
	return sitepublish.Addresses{Domain: p.domain}.Within(host)
}

type exactHostPolicy struct{ host string }

func (p exactHostPolicy) Allow(_ context.Context, host string) bool {
	return p.host != "" && hostname.Normalize(host) == p.host
}

type guardedPolicy struct {
	hosts    HostChecker
	mu       sync.Mutex
	verdicts map[string]bool
}

func (p *guardedPolicy) Allow(ctx context.Context, host string) bool {
	host = hostname.Normalize(host)
	if host == "" {
		return false
	}
	p.mu.Lock()
	verdict, known := p.verdicts[host]
	p.mu.Unlock()
	if known {
		return verdict
	}
	verdict = p.hosts.CheckHost(ctx, host) == nil
	p.mu.Lock()
	p.verdicts[host] = verdict
	p.mu.Unlock()
	return verdict
}

func (s *Service) previewURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Host == "" {
		return nil, ErrURLRefused
	}
	if !(domainPolicy{domain: s.domain}).Allow(context.Background(), u.Hostname()) {
		return nil, ErrURLRefused
	}
	return u, nil
}

func validViewport(v Viewport) bool {
	return v.Width >= minViewportWidth && v.Width <= maxViewportWidth && v.Height >= minViewportHeight && v.Height <= maxViewportHeight
}

func (s *Service) acquire(ctx context.Context) (func(), error) {
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: waiting for a free renderer: %w", ErrTimeout, ctx.Err())
	}
}

func classify(ctx context.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
		if errors.Is(err, ErrTimeout) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrTimeout, err)
	}
	return err
}

func (s *Service) run(ctx context.Context, timeout time.Duration, call func(context.Context) error) error {
	release, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return classify(callCtx, call(callCtx))
}

func jpegSize(data []byte) (image.Point, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return image.Point{}, fmt.Errorf("siterender: unreadable screenshot: %w", err)
	}
	return image.Pt(config.Width, config.Height), nil
}

func decodeRGBA(data []byte) (*image.RGBA, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("siterender: unreadable screenshot: %w", err)
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	return rgba, nil
}

func (s *Service) Prerender(ctx context.Context, req PrerenderRequest) (map[string]string, error) {
	u, err := s.previewURL(req.URL)
	if err != nil {
		return nil, err
	}
	routes, err := uniqueRoutes(req.Routes)
	if err != nil {
		return nil, err
	}
	entry := *u
	entry.Path = path.Join("/", u.Path, prerenderEntry)
	entry.RawPath, entry.Fragment = "", ""
	job := PrerenderJob{URL: entry.String(), Routes: routes, Policy: exactHostPolicy{host: hostname.Normalize(u.Hostname())}}
	var rendered map[string]string
	err = s.run(ctx, s.timeouts.prerender, func(callCtx context.Context) error {
		var renderErr error
		rendered, renderErr = s.browser.Prerender(callCtx, job)
		return renderErr
	})
	if err != nil {
		if errors.Is(err, ErrTimeout) || errors.Is(err, ErrUnavailable) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrPrerenderFailed, err)
	}
	out := make(map[string]string, len(routes))
	for _, route := range routes {
		markup, ok := rendered[route]
		if !ok || strings.TrimSpace(markup) == "" {
			return nil, fmt.Errorf("%w: route %s produced no markup", ErrPrerenderFailed, route)
		}
		if len(markup) > maxPrerenderHTMLBytes {
			return nil, fmt.Errorf("%w: route %s markup is larger than %d bytes", ErrPrerenderFailed, route, maxPrerenderHTMLBytes)
		}
		out[route] = markup
	}
	return out, nil
}

func uniqueRoutes(routes []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(routes))
	for _, route := range routes {
		if !site.ValidRoutePath(route) {
			return nil, fmt.Errorf("%w: route %q is not a page path", ErrRequestInvalid, route)
		}
		if !seen[route] {
			seen[route] = true
			out = append(out, route)
		}
	}
	if len(out) == 0 || len(out) > maxPrerenderRoutes {
		return nil, fmt.Errorf("%w: between 1 and %d routes are required", ErrRequestInvalid, maxPrerenderRoutes)
	}
	return out, nil
}

func (s *Service) Screenshot(ctx context.Context, rawURL string, viewport Viewport) ([]byte, error) {
	u, err := s.previewURL(rawURL)
	if err != nil {
		return nil, err
	}
	if !validViewport(viewport) {
		return nil, fmt.Errorf("%w: viewport %dx%d is out of range", ErrRequestInvalid, viewport.Width, viewport.Height)
	}
	job := ScreenshotJob{URL: u.String(), Viewport: viewport, Policy: domainPolicy{domain: s.domain}}
	var data []byte
	err = s.run(ctx, s.timeouts.screenshot, func(callCtx context.Context) error {
		var shotErr error
		data, shotErr = s.browser.Screenshot(callCtx, job)
		return shotErr
	})
	if err != nil {
		return nil, err
	}
	if _, err := jpegSize(data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Service) referenceURL(ctx context.Context, raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Hostname() == "" {
		return nil, ErrReferenceRefused
	}
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		return nil, fmt.Errorf("%w: only the standard web ports are allowed", ErrReferenceRefused)
	}
	if err := s.hosts.CheckHost(ctx, hostname.Normalize(u.Hostname())); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReferenceRefused, err)
	}
	return u, nil
}

func (s *Service) CaptureReference(ctx context.Context, rawURL string) (Reference, error) {
	u, err := s.referenceURL(ctx, rawURL)
	if err != nil {
		return Reference{}, err
	}
	job := ReferenceJob{
		URL:       u.String(),
		Policy:    &guardedPolicy{hosts: s.hosts, verdicts: map[string]bool{}},
		Viewports: DefaultViewports(),
		MaxSlices: referenceSlices,
	}
	var captured ReferenceCapture
	err = s.run(ctx, s.timeouts.reference, func(callCtx context.Context) error {
		var refErr error
		captured, refErr = s.browser.Reference(callCtx, job)
		return refErr
	})
	if err != nil {
		return Reference{}, err
	}
	if _, err := s.referenceURL(ctx, captured.URL); err != nil {
		return Reference{}, err
	}
	reference := Reference{URL: captured.URL, Title: truncateRunes(strings.TrimSpace(captured.Title), maxDetailRunes), Tokens: ExtractTokens(captured.Samples)}
	var tiles []tile
	for v, capture := range captured.Viewports {
		for i, frame := range capture.Slices {
			img, err := decodeRGBA(frame.JPEG)
			if err != nil {
				return Reference{}, err
			}
			if i == 0 {
				reference.Shots = append(reference.Shots, Shot{Viewport: capture.Viewport, JPEG: frame.JPEG})
			}
			tiles = append(tiles, tile{group: v, image: img, label: fmt.Sprintf("%d %d/%d", capture.Viewport.Width, i+1, len(capture.Slices))})
		}
	}
	if len(tiles) == 0 {
		return Reference{}, fmt.Errorf("siterender: the reference produced no screenshots")
	}
	reference.Image, err = composeSheet(tiles, MaxImageBytes)
	if err != nil {
		return Reference{}, err
	}
	return reference, nil
}
