package siteserve_usecase

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"

	"vozko/domain/hostname"
	"vozko/domain/sitepublish"
)

const minCookieSecretLength = 32

type Service struct {
	directory Directory
	files     Files
	visits    Visits
	beacons   limiter
	passwords limiter
	now       func() time.Time
	roll      func(int) int
	cfg       Config
}

type limiter interface {
	Allow(key string) (bool, time.Duration, error)
}

func New(d Deps, cfg Config) (*Service, error) {
	if d.Directory == nil || d.Files == nil || d.Visits == nil || d.Beacons == nil || d.Passwords == nil || d.Now == nil || d.Roll == nil {
		return nil, errors.New("siteserve: the directory, files, visits, limiters, clock and roll are required")
	}
	if len(cfg.CookieSecret) < minCookieSecretLength {
		return nil, errors.New("siteserve: the cookie secret is too short")
	}
	cfg.Addresses.Domain = hostname.Normalize(cfg.Addresses.Domain)
	if cfg.Addresses.Domain == "" {
		return nil, errors.New("siteserve: the platform domain is required")
	}
	return &Service{directory: d.Directory, files: d.Files, visits: d.Visits, beacons: d.Beacons, passwords: d.Passwords, now: d.Now, roll: d.Roll, cfg: cfg}, nil
}

func (s *Service) Claims(ctx context.Context, rawHost string) (bool, error) {
	host := hostname.Normalize(rawHost)
	if _, preview := s.cfg.Addresses.PreviewToken(host); preview {
		return false, nil
	}
	if s.cfg.Addresses.Within(host) {
		return true, nil
	}
	if _, err := sitepublish.ValidateCustomHostname(host, s.cfg.PlatformDomains); err != nil {
		return false, nil
	}
	_, found, err := s.directory.Serving(ctx, sitepublish.HostKey(host))
	return found, err
}

type visit struct {
	Request
	host     string
	readOnly bool
	settings sitepublish.ServingSettings
	query    url.Values
}

func (s *Service) Serve(ctx context.Context, req Request) Outcome {
	host := hostname.Normalize(req.Host)
	key, readOnly := s.servingKey(host)
	settings, found, err := s.directory.Serving(ctx, key)
	if err != nil {
		log.Printf("[siteserve] serving settings of %s: %v", host, err)
		return Outcome{Kind: KindUnavailable}
	}
	if !found {
		return Outcome{Kind: KindNotFound}
	}
	if settings.Suspended {
		return Outcome{Kind: KindSuspended}
	}
	query, _ := url.ParseQuery(req.RawQuery)
	v := visit{Request: req, host: host, readOnly: readOnly, settings: settings, query: query}
	if req.Path == BeaconPath {
		return s.beacon(ctx, v)
	}
	if settings.PasswordHash != "" {
		if gate, closed := s.passwordGate(v); closed {
			return gate
		}
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return Outcome{Kind: KindMethodNotAllowed, Allow: []string{http.MethodGet, http.MethodHead}}
	}
	path := sitepublish.ServedPath(req.Path)
	for _, r := range settings.Redirects {
		if r.From == path {
			return Outcome{Kind: KindRedirect, Location: r.To, Status: redirectStatus(r.Status)}
		}
	}
	return s.content(ctx, v, path)
}

func (s *Service) servingKey(host string) (string, bool) {
	if id, ok := s.cfg.Addresses.DeploymentID(host); ok {
		return sitepublish.DeploymentKey(id), true
	}
	return sitepublish.HostKey(host), false
}

func redirectStatus(status int) int {
	if status == http.StatusFound {
		return http.StatusFound
	}
	return http.StatusMovedPermanently
}

func (s *Service) manifest(ctx context.Context, v visit) (sitepublish.DeploymentManifest, bool) {
	m, err := s.files.Manifest(ctx, v.settings.DeploymentID)
	if err != nil {
		log.Printf("[siteserve] manifest of deployment %s: %v", v.settings.DeploymentID, err)
		return sitepublish.DeploymentManifest{}, false
	}
	return m, true
}
