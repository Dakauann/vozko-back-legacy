package siterender

import (
	"context"
	"errors"
)

type Viewport struct {
	Width  int `json:"w"`
	Height int `json:"h"`
}

var (
	Desktop   = Viewport{Width: 1440, Height: 900}
	Mobile    = Viewport{Width: 390, Height: 844}
	OpenGraph = Viewport{Width: 1200, Height: 630}
)

func DefaultViewports() []Viewport {
	return []Viewport{Desktop, Mobile}
}

type PrerenderRequest struct {
	URL    string
	Routes []string
}

type Shot struct {
	Viewport Viewport
	JPEG     []byte
}

type Reference struct {
	URL    string
	Title  string
	Image  []byte
	Shots  []Shot
	Tokens Tokens
}

type Renderer interface {
	Prerender(ctx context.Context, req PrerenderRequest) (map[string]string, error)
	Screenshot(ctx context.Context, url string, viewport Viewport) ([]byte, error)
	CaptureReference(ctx context.Context, url string) (Reference, error)
}

type HostChecker interface {
	CheckHost(ctx context.Context, host string) error
}

type HostPolicy interface {
	Allow(ctx context.Context, host string) bool
}

type Frame struct {
	Top  int
	JPEG []byte
}

type TextSample struct {
	Node      string  `json:"node"`
	FG        [3]int  `json:"fg"`
	BG        [3]int  `json:"bg"`
	Px        float64 `json:"px"`
	Weight    int     `json:"weight"`
	OverMedia bool    `json:"overMedia"`
}

type ImageSample struct {
	Node     string `json:"node"`
	Src      string `json:"src"`
	NaturalW int    `json:"naturalW"`
	NaturalH int    `json:"naturalH"`
	ShownW   int    `json:"shownW"`
	ShownH   int    `json:"shownH"`
}

type AxeViolation struct {
	ID     string   `json:"id"`
	Impact string   `json:"impact"`
	Count  int      `json:"count"`
	Nodes  []string `json:"nodes"`
}

type Metrics struct {
	LCP float64 `json:"lcp"`
	CLS float64 `json:"cls"`
	TBT float64 `json:"tbt"`
}

type FailedRequest struct {
	URL     string
	Status  int
	Reason  string
	Blocked bool
}

type Facts struct {
	ScrollWidth    int             `json:"scrollWidth"`
	Overflow       []string        `json:"overflow"`
	Texts          []TextSample    `json:"texts"`
	Images         []ImageSample   `json:"images"`
	Axe            []AxeViolation  `json:"axe"`
	Metrics        Metrics         `json:"metrics"`
	ConsoleErrors  []string        `json:"-"`
	FailedRequests []FailedRequest `json:"-"`
	TransferBytes  int64           `json:"-"`
}

type ViewportCapture struct {
	Viewport   Viewport
	PageHeight int
	Slices     []Frame
}

type PrerenderJob struct {
	URL    string
	Routes []string
	Policy HostPolicy
}

type ScreenshotJob struct {
	URL      string
	Viewport Viewport
	Policy   HostPolicy
}

type ColorSample struct {
	RGB    [3]int  `json:"rgb"`
	Weight float64 `json:"w"`
}

type FontSample struct {
	Family string  `json:"family"`
	Weight float64 `json:"w"`
}

type TokenSamples struct {
	Backgrounds []ColorSample `json:"backgrounds"`
	Texts       []ColorSample `json:"texts"`
	Fonts       []FontSample  `json:"fonts"`
	Spacing     []float64     `json:"spacing"`
}

type LiveDOMRequest struct {
	URL    string
	Routes []string
}

type LiveDOMJob struct {
	URL    string
	Routes []string
	Policy HostPolicy
}

type ReferenceJob struct {
	URL       string
	Policy    HostPolicy
	Viewports []Viewport
	MaxSlices int
}

type ReferenceCapture struct {
	URL       string
	Title     string
	Viewports []ViewportCapture
	Samples   TokenSamples
}

type Browser interface {
	Prerender(ctx context.Context, job PrerenderJob) (map[string]string, error)
	Screenshot(ctx context.Context, job ScreenshotJob) ([]byte, error)
	Reference(ctx context.Context, job ReferenceJob) (ReferenceCapture, error)
	LiveDOM(ctx context.Context, job LiveDOMJob) (map[string]string, error)
}

var (
	ErrURLRefused       = errors.New("siterender: the address is not a page preview")
	ErrReferenceRefused = errors.New("siterender: the reference address cannot be opened")
	ErrRequestInvalid   = errors.New("siterender: the render request is invalid")
	ErrTimeout          = errors.New("siterender: the renderer did not finish in time")
	ErrUnavailable      = errors.New("siterender: the renderer is unavailable")
	ErrPrerenderFailed  = errors.New("siterender: the page could not be prerendered")
	ErrLiveScanFailed   = errors.New("siterender: the published page could not be scanned after its scripts ran")
)

const (
	CodeURLRefused       = "render_url_refused"
	CodeReferenceRefused = "reference_refused"
	CodeRequestInvalid   = "render_request_invalid"
	CodeTimeout          = "render_timeout"
	CodeUnavailable      = "render_unavailable"
	CodePrerenderFailed  = "prerender_failed"
	CodeLiveScanFailed   = "live_scan_failed"
	CodeRenderFailed     = "render_failed"
)

var errorCodes = []struct {
	err  error
	code string
}{
	{ErrURLRefused, CodeURLRefused},
	{ErrReferenceRefused, CodeReferenceRefused},
	{ErrRequestInvalid, CodeRequestInvalid},
	{ErrTimeout, CodeTimeout},
	{ErrUnavailable, CodeUnavailable},
	{ErrPrerenderFailed, CodePrerenderFailed},
	{ErrLiveScanFailed, CodeLiveScanFailed},
}

func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	for _, known := range errorCodes {
		if errors.Is(err, known.err) {
			return known.code
		}
	}
	return CodeRenderFailed
}
