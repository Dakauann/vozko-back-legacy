package siteserve_usecase

import (
	"context"
	"time"

	"vozko/domain/cache"
	"vozko/domain/sitepublish"
)

const (
	BeaconPath            = "/__vz/e"
	PasswordPath          = "/__vz/password"
	MaxBeaconBytes        = 4096
	PassCookie            = "__Host-vz_pass"
	VariantCookie         = "vz_variant"
	VariantCookieLifetime = 30 * 24 * time.Hour
)

type Directory interface {
	Serving(ctx context.Context, key string) (sitepublish.ServingSettings, bool, error)
}

type Files interface {
	Manifest(ctx context.Context, deploymentID string) (sitepublish.DeploymentManifest, error)
	Blob(ctx context.Context, sha string) ([]byte, error)
}

type Visits interface {
	Record(ctx context.Context, v sitepublish.Visit) error
}

type Config struct {
	Addresses       sitepublish.Addresses
	PlatformDomains []string
	CookieSecret    []byte
}

type Deps struct {
	Directory Directory
	Files     Files
	Visits    Visits
	Beacons   cache.RateLimiter
	Passwords cache.RateLimiter
	Now       func() time.Time
	Roll      func(total int) int
}

type Request struct {
	Host      string
	Method    string
	Path      string
	RawQuery  string
	Cookie    func(name string) string
	UserAgent string
	Referer   string
	ClientIP  string
	Password  string
	Next      string
	Body      []byte
}

type Kind int

const (
	KindNotFound Kind = iota
	KindUnavailable
	KindSuspended
	KindPasswordPrompt
	KindRedirect
	KindContent
	KindAccepted
	KindTooManyRequests
	KindBadRequest
	KindPayloadTooLarge
	KindMethodNotAllowed
)

type Cookie struct {
	Name     string
	Value    string
	Path     string
	MaxAge   time.Duration
	HTTPOnly bool
}

type Prompt struct {
	Next   string
	Failed bool
}

type Content struct {
	Body         []byte
	ContentType  string
	CacheControl string
	ETag         string
	Policy       string
	Missing      bool
	NoIndex      bool
}

type Outcome struct {
	Kind     Kind
	Location string
	Status   int
	Allow    []string
	Prompt   Prompt
	Content  Content
	Cookies  []Cookie
}
