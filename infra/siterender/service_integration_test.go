package siterender

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	renderuc "vozko/usecases/siterender"
)

type serviceRig struct {
	client   *Client
	pageURL  string
	pageHost string
}

func newServiceRig(t *testing.T) serviceRig {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("VOZKO_SITERENDER_SERVICE_URL"))
	if endpoint == "" {
		t.Skip("VOZKO_SITERENDER_SERVICE_URL is not set; skipping the renderer service test")
	}
	page := strings.TrimSpace(os.Getenv("VOZKO_SITERENDER_SERVICE_PAGE"))
	proxy := strings.TrimSpace(os.Getenv("VOZKO_SITERENDER_SERVICE_REFERENCE_PROXY"))
	token := strings.TrimSpace(os.Getenv("VOZKO_SITERENDER_SERVICE_TOKEN"))
	if page == "" || proxy == "" || token == "" {
		t.Fatal("VOZKO_SITERENDER_SERVICE_PAGE (a page on the gateway preview domain), VOZKO_SITERENDER_SERVICE_REFERENCE_PROXY and VOZKO_SITERENDER_SERVICE_TOKEN are required")
	}
	parsed, err := url.Parse(page)
	if err != nil || parsed.Hostname() == "" {
		t.Fatalf("VOZKO_SITERENDER_SERVICE_PAGE %q is not a url", page)
	}
	client, err := New(Config{Endpoint: endpoint, Token: token, ReferenceProxy: proxy})
	if err != nil {
		t.Fatal(err)
	}
	return serviceRig{client: client, pageURL: page, pageHost: parsed.Hostname()}
}

type allowEverything struct{}

func (allowEverything) Allow(context.Context, string) bool { return true }

func serviceContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestRenderServiceScreenshotsAPreviewThroughTheGateway(t *testing.T) {
	r := newServiceRig(t)
	data, err := r.client.Screenshot(serviceContext(t), renderuc.ScreenshotJob{
		URL: r.pageURL, Viewport: renderuc.OpenGraph, Policy: hostPolicy{hosts: map[string]bool{r.pageHost: true}},
	})
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("the screenshot is empty")
	}
}

func TestRenderServiceGatewayRefusesHostsOutsideThePreviewDomain(t *testing.T) {
	r := newServiceRig(t)
	_, err := r.client.Screenshot(serviceContext(t), renderuc.ScreenshotJob{URL: "https://www.wikipedia.org/", Viewport: renderuc.OpenGraph, Policy: allowEverything{}})
	if err == nil {
		t.Fatal("the gateway let a preview render reach a host outside the preview domain")
	}
}

func TestRenderServiceReferenceUsesThePublicGateway(t *testing.T) {
	r := newServiceRig(t)
	captured, err := r.client.Reference(serviceContext(t), renderuc.ReferenceJob{
		URL: "https://www.wikipedia.org/", Policy: allowEverything{}, Viewports: []renderuc.Viewport{renderuc.Desktop}, MaxSlices: 2,
	})
	if err != nil {
		t.Fatalf("reference: %v", err)
	}
	if len(captured.Viewports) != 1 || len(captured.Samples.Fonts) == 0 {
		t.Fatalf("reference %+v", captured.Samples)
	}
}

func TestRenderServiceReferenceGatewayRefusesPrivateAddresses(t *testing.T) {
	r := newServiceRig(t)
	_, err := r.client.Reference(serviceContext(t), renderuc.ReferenceJob{
		URL: "http://169.254.169.254/latest/meta-data/", Policy: allowEverything{}, Viewports: []renderuc.Viewport{renderuc.Desktop}, MaxSlices: 1,
	})
	if err == nil {
		t.Fatal("the public gateway reached a metadata address")
	}
}
