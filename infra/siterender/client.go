package siterender

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"

	"vozko/infra/siterendergate"
	renderuc "vozko/usecases/siterender"
)

//go:embed probe.js
var probeSource string

const (
	probeWorld          = "vz-probe"
	sliceQuality        = 90
	screenshotQuality   = 85
	sliceSettle         = 350 * time.Millisecond
	loadSettle          = 800 * time.Millisecond
	referenceSettle     = 1200 * time.Millisecond
	worldWait           = 5 * time.Second
	prerenderReadyWait  = 20 * time.Second
	pollInterval        = 50 * time.Millisecond
	blockedByClientText = "ERR_BLOCKED_BY_CLIENT"
	phoneMaxWidth       = 480
	cleanupTimeout      = 2 * time.Second
	discoveryTimeout    = 20 * time.Second
	discoveryReplyLimit = 64 << 10
)

type Config struct {
	Endpoint       string
	Token          string
	ReferenceProxy string
}

type Client struct {
	config    Config
	endpoint  *url.URL
	discovery *http.Client
}

var _ renderuc.Browser = (*Client)(nil)

func New(config Config) (*Client, error) {
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || (endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawQuery != "" {
		return nil, errors.New("siterender: the renderer endpoint must be an http url without a path")
	}
	if !siterendergate.ValidRelayToken(config.Token) {
		return nil, errors.New("siterender: the renderer token is missing or malformed")
	}
	discovery := &http.Client{Timeout: discoveryTimeout, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	return &Client{config: config, endpoint: endpoint, discovery: discovery}, nil
}

type session struct {
	ctx             context.Context
	policy          renderuc.HostPolicy
	refusal         error
	mu              sync.Mutex
	worldID         runtime.ExecutionContextID
	frameID         string
	inflight        map[network.RequestID]bool
	documentStatus  int
	documentRefused bool
}

func (c *Client) open(ctx context.Context, proxy string, policy renderuc.HostPolicy, refusal error) (*session, func(), error) {
	debugger, err := c.debuggerURL(ctx)
	if err != nil {
		return nil, nil, err
	}
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, debugger, chromedp.NoModifyURL)
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	unavailable := func(err error) error {
		cancelBrowser()
		cancelAlloc()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %w", renderuc.ErrUnavailable, err)
	}
	if err := chromedp.Run(browserCtx); err != nil {
		return nil, nil, unavailable(err)
	}
	browser := cdp.WithExecutor(browserCtx, chromedp.FromContext(browserCtx).Browser)
	create := target.CreateBrowserContext().WithDisposeOnDetach(true)
	if proxy != "" {
		create = create.WithProxyServer(proxy).WithProxyBypassList("<-loopback>")
	}
	browserContextID, err := create.Do(browser)
	if err != nil {
		return nil, nil, unavailable(err)
	}
	dispose := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		_ = target.DisposeBrowserContext(browserContextID).Do(cdp.WithExecutor(cleanup, chromedp.FromContext(browserCtx).Browser))
	}
	targetID, err := target.CreateTarget("about:blank").WithBrowserContextID(browserContextID).WithNewWindow(true).Do(browser)
	if err != nil {
		dispose()
		return nil, nil, unavailable(err)
	}
	tabCtx, cancelTab := chromedp.NewContext(browserCtx, chromedp.WithTargetID(targetID))
	done := func() {
		_ = chromedp.Cancel(tabCtx)
		cancelTab()
		dispose()
		cancelBrowser()
		cancelAlloc()
	}
	if err := chromedp.Run(tabCtx); err != nil {
		done()
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, fmt.Errorf("%w: %w", renderuc.ErrUnavailable, err)
	}
	s := &session{ctx: tabCtx, policy: policy, refusal: refusal, inflight: map[network.RequestID]bool{}}
	if t := chromedp.FromContext(tabCtx).Target; t != nil {
		s.frameID = string(t.TargetID)
	}
	chromedp.ListenTarget(tabCtx, s.listen)
	err = chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		if err := fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "*", RequestStage: fetch.RequestStageRequest}}).Do(ctx); err != nil {
			return err
		}
		if err := network.SetCacheDisabled(true).Do(ctx); err != nil {
			return err
		}
		_, err := page.AddScriptToEvaluateOnNewDocument(probeSource).WithWorldName(probeWorld).Do(ctx)
		return err
	}))
	if err != nil {
		done()
		return nil, nil, fmt.Errorf("%w: %w", renderuc.ErrUnavailable, err)
	}
	return s, done, nil
}

func allowedScheme(scheme string) bool {
	switch scheme {
	case "http", "https", "ws", "wss":
		return true
	}
	return false
}

func (s *session) intercept(e *fetch.EventRequestPaused) {
	t := chromedp.FromContext(s.ctx).Target
	if t == nil {
		return
	}
	executor := cdp.WithExecutor(s.ctx, t)
	u, err := url.Parse(e.Request.URL)
	inline := err == nil && (u.Scheme == "data" || u.Scheme == "blob")
	if inline || (err == nil && allowedScheme(u.Scheme) && s.policy.Allow(s.ctx, u.Hostname())) {
		_ = fetch.ContinueRequest(e.RequestID).Do(executor)
		return
	}
	_ = fetch.FailRequest(e.RequestID, network.ErrorReasonBlockedByClient).Do(executor)
}

func (s *session) listen(ev any) {
	switch e := ev.(type) {
	case *fetch.EventRequestPaused:
		go s.intercept(e)
	case *runtime.EventExecutionContextCreated:
		if e.Context == nil || e.Context.Name != probeWorld {
			return
		}
		var aux struct {
			FrameID string `json:"frameId"`
		}
		_ = json.Unmarshal(e.Context.AuxData, &aux)
		s.mu.Lock()
		if s.frameID == "" || aux.FrameID == s.frameID {
			s.worldID = e.Context.ID
		}
		s.mu.Unlock()
	case *network.EventRequestWillBeSent:
		if e.Request != nil {
			s.mu.Lock()
			s.inflight[e.RequestID] = true
			s.mu.Unlock()
		}
	case *network.EventResponseReceived:
		if e.Response == nil {
			return
		}
		if e.Type == network.ResourceTypeDocument && string(e.FrameID) == s.frameID {
			s.mu.Lock()
			s.documentStatus = int(e.Response.Status)
			s.documentRefused = gatewayRefused(e.Response.Headers)
			s.mu.Unlock()
		}
	case *network.EventLoadingFailed:
		s.settleRequest(e.RequestID)
	case *network.EventLoadingFinished:
		s.settleRequest(e.RequestID)
	}
}

func (s *session) settleRequest(id network.RequestID) {
	s.mu.Lock()
	delete(s.inflight, id)
	s.mu.Unlock()
}

func exceptionText(d *runtime.ExceptionDetails) string {
	if d.Exception != nil && d.Exception.Description != "" {
		return strings.SplitN(d.Exception.Description, "\n", 2)[0]
	}
	return d.Text
}

func (s *session) run(action func(ctx context.Context) error) error {
	return chromedp.Run(s.ctx, chromedp.ActionFunc(action))
}

func (s *session) navigate(rawURL string, viewport renderuc.Viewport) error {
	s.mu.Lock()
	s.worldID = 0
	s.documentStatus, s.documentRefused = 0, false
	s.inflight = map[network.RequestID]bool{}
	s.mu.Unlock()
	phone := viewport.Width <= phoneMaxWidth
	err := chromedp.Run(s.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := emulation.SetDeviceMetricsOverride(int64(viewport.Width), int64(viewport.Height), 1, phone).Do(ctx); err != nil {
				return err
			}
			return emulation.SetTouchEmulationEnabled(phone).Do(ctx)
		}),
		chromedp.Navigate(rawURL),
	)
	if err != nil {
		if strings.Contains(err.Error(), blockedByClientText) {
			return fmt.Errorf("%w: %w", s.refusal, err)
		}
		return err
	}
	s.mu.Lock()
	status, refused := s.documentStatus, s.documentRefused
	s.mu.Unlock()
	if refused {
		return fmt.Errorf("%w: the egress gateway refused %s", s.refusal, rawURL)
	}
	if status >= 400 {
		return fmt.Errorf("siterender: the page answered HTTP %d", status)
	}
	return nil
}

func gatewayRefused(headers network.Headers) bool {
	for key, value := range headers {
		if strings.EqualFold(key, siterendergate.RefusalHeader) {
			text, _ := value.(string)
			return strings.TrimSpace(text) == "refused"
		}
	}
	return false
}

func (s *session) world() (runtime.ExecutionContextID, error) {
	deadline := time.Now().Add(worldWait)
	for {
		s.mu.Lock()
		id := s.worldID
		s.mu.Unlock()
		if id != 0 {
			return id, nil
		}
		if time.Now().After(deadline) {
			return 0, errors.New("siterender: the probe world did not start")
		}
		select {
		case <-s.ctx.Done():
			return 0, s.ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (s *session) evaluate(expression string, isolated bool, out any) error {
	var contextID runtime.ExecutionContextID
	if isolated {
		id, err := s.world()
		if err != nil {
			return err
		}
		contextID = id
	}
	return s.run(func(ctx context.Context) error {
		params := runtime.Evaluate(expression).WithReturnByValue(true).WithAwaitPromise(true)
		if contextID != 0 {
			params = params.WithContextID(contextID)
		}
		result, exception, err := params.Do(ctx)
		if err != nil {
			return err
		}
		if exception != nil {
			return fmt.Errorf("siterender: page script failed: %s", exceptionText(exception))
		}
		if out == nil || result == nil || len(result.Value) == 0 {
			return nil
		}
		return json.Unmarshal(result.Value, out)
	})
}

func (s *session) probe(call string, out any) error {
	return s.evaluate("globalThis.__vzProbe."+call, true, out)
}

func jsArg(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func (s *session) settle(wait time.Duration) error {
	return s.probe(fmt.Sprintf("settled(%d)", wait.Milliseconds()), nil)
}

func (s *session) screenshot(quality int64) ([]byte, error) {
	var data []byte
	err := s.run(func(ctx context.Context) error {
		var err error
		data, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatJpeg).WithQuality(quality).WithFromSurface(true).Do(ctx)
		return err
	})
	return data, err
}

type visitResult struct {
	Y int `json:"y"`
}

func (s *session) walk(tops []int) ([]renderuc.Frame, error) {
	shots := make([]renderuc.Frame, 0, len(tops))
	for _, top := range tops {
		var visited visitResult
		if err := s.probe(fmt.Sprintf("visit(%d, %d)", top, sliceSettle.Milliseconds()), &visited); err != nil {
			return nil, err
		}
		shot, err := s.screenshot(sliceQuality)
		if err != nil {
			return nil, err
		}
		shots = append(shots, renderuc.Frame{Top: visited.Y, JPEG: shot})
	}
	return shots, nil
}

func (c *Client) Screenshot(ctx context.Context, job renderuc.ScreenshotJob) ([]byte, error) {
	s, done, err := c.open(ctx, "", job.Policy, renderuc.ErrURLRefused)
	if err != nil {
		return nil, err
	}
	defer done()
	if err := s.navigate(job.URL, job.Viewport); err != nil {
		return nil, err
	}
	if err := s.settle(loadSettle); err != nil {
		return nil, err
	}
	return s.screenshot(screenshotQuality)
}

func (c *Client) Prerender(ctx context.Context, job renderuc.PrerenderJob) (map[string]string, error) {
	s, done, err := c.open(ctx, "", job.Policy, renderuc.ErrURLRefused)
	if err != nil {
		return nil, err
	}
	defer done()
	if err := s.navigate(job.URL, renderuc.Desktop); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(prerenderReadyWait)
	for {
		var ready bool
		if err := s.evaluate(`typeof window.__vzPrerender === "function"`, false, &ready); err != nil {
			return nil, err
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: the prerender entry did not define window.__vzPrerender", renderuc.ErrPrerenderFailed)
		}
		select {
		case <-s.ctx.Done():
			return nil, s.ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	out := make(map[string]string, len(job.Routes))
	for _, route := range job.Routes {
		var markup string
		expression := fmt.Sprintf(`(async () => { const html = await window.__vzPrerender(%s); if (typeof html !== "string") throw new Error("prerender returned " + typeof html); return html; })()`, jsArg(route))
		if err := s.evaluate(expression, false, &markup); err != nil {
			return nil, fmt.Errorf("%w: route %s: %w", renderuc.ErrPrerenderFailed, route, err)
		}
		out[route] = markup
	}
	return out, nil
}

func (c *Client) Reference(ctx context.Context, job renderuc.ReferenceJob) (renderuc.ReferenceCapture, error) {
	var captured renderuc.ReferenceCapture
	s, done, err := c.open(ctx, c.config.ReferenceProxy, job.Policy, renderuc.ErrReferenceRefused)
	if err != nil {
		return captured, err
	}
	defer done()
	for i, viewport := range job.Viewports {
		if err := s.navigate(job.URL, viewport); err != nil {
			return captured, err
		}
		if err := s.settle(referenceSettle); err != nil {
			return captured, err
		}
		capture := renderuc.ViewportCapture{Viewport: viewport}
		if err := s.probe("pageHeight()", &capture.PageHeight); err != nil {
			return captured, err
		}
		slices, _ := renderuc.SliceTops(capture.PageHeight, viewport.Height, job.MaxSlices)
		capture.Slices, err = s.walk(slices)
		if err != nil {
			return captured, err
		}
		if i == 0 {
			var top visitResult
			if err := s.probe("visit(0, 0)", &top); err != nil {
				return captured, err
			}
			var described struct {
				URL   string `json:"url"`
				Title string `json:"title"`
			}
			if err := s.probe("describe()", &described); err != nil {
				return captured, err
			}
			captured.URL, captured.Title = described.URL, described.Title
			if err := s.probe("tokens()", &captured.Samples); err != nil {
				return captured, err
			}
		}
		captured.Viewports = append(captured.Viewports, capture)
	}
	return captured, nil
}
