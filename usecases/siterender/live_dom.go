package siterender

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"vozko/domain/site"
	"vozko/domain/hostname"
)

const liveScanTimeout = 120 * time.Second

func LiveRouteURL(base, route string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Host == "" || !site.ValidRoutePath(route) {
		return "", fmt.Errorf("%w: route %q on %q", ErrRequestInvalid, route, base)
	}
	switch {
	case route != site.HomeRoute:
		u.Path = path.Join("/", u.Path, route)
	case u.Path == "":
		u.Path = "/"
	}
	u.RawPath, u.Fragment = "", ""
	return u.String(), nil
}

func (s *Service) LiveDOM(ctx context.Context, req LiveDOMRequest) (map[string]string, error) {
	u, err := s.previewURL(req.URL)
	if err != nil {
		return nil, err
	}
	routes, err := uniqueRoutes(req.Routes)
	if err != nil {
		return nil, err
	}
	entry := *u
	entry.Fragment = ""
	job := LiveDOMJob{URL: entry.String(), Routes: routes, Policy: exactHostPolicy{host: hostname.Normalize(u.Hostname())}}
	var documents map[string]string
	err = s.run(ctx, liveScanTimeout, func(callCtx context.Context) error {
		var scanErr error
		documents, scanErr = s.browser.LiveDOM(callCtx, job)
		return scanErr
	})
	if err != nil {
		if errors.Is(err, ErrTimeout) || errors.Is(err, ErrUnavailable) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrLiveScanFailed, err)
	}
	out := make(map[string]string, len(routes))
	for _, route := range routes {
		document, ok := documents[route]
		if !ok || strings.TrimSpace(document) == "" {
			return nil, fmt.Errorf("%w: route %s produced no document", ErrLiveScanFailed, route)
		}
		if len(document) > maxPrerenderHTMLBytes {
			return nil, fmt.Errorf("%w: route %s document is larger than %d bytes", ErrLiveScanFailed, route, maxPrerenderHTMLBytes)
		}
		out[route] = document
	}
	return out, nil
}
