package sitebuild

import (
	"context"
	"crypto/hmac"
	"errors"
	"path"
	"strings"
	"time"

	"vozko/domain/hostname"
	"vozko/domain/sitepublish"
)

type Served struct {
	ContentType string
	Body        []byte
}

func (s *Service) grantOf(host string) (grant, error) {
	label, ok := s.addresses.PreviewToken(host)
	if !ok {
		return grant{}, ErrPreviewNotFound
	}
	tok, ok := decodeToken(label)
	if !ok || s.now().Unix() >= tok.expires {
		return grant{}, ErrPreviewNotFound
	}
	g, found, err := s.store.loadGrant(tok.id)
	if err != nil {
		return grant{}, err
	}
	if !found || g.Expires != tok.expires || !hmac.Equal(tokenMAC(s.key, tok.id, tok.expires, g), tok.mac) {
		return grant{}, ErrPreviewNotFound
	}
	if !sameHost(s.addresses.PreviewHost(encodeToken(s.key, tok.id, g)), hostname.Normalize(host)) {
		return grant{}, ErrPreviewNotFound
	}
	return g, nil
}

func (s *Service) PreviewOrigin(origin string) bool {
	return s.addresses.PreviewOrigin(origin)
}

func (s *Service) PreviewGrant(_ context.Context, origin string) (sitepublish.PreviewGrant, bool, error) {
	host, ok := s.addresses.PreviewOriginHost(origin)
	if !ok {
		return sitepublish.PreviewGrant{}, false, nil
	}
	g, err := s.grantOf(host)
	if errors.Is(err, ErrPreviewNotFound) {
		return sitepublish.PreviewGrant{}, false, nil
	}
	if err != nil {
		return sitepublish.PreviewGrant{}, false, err
	}
	return sitepublish.PreviewGrant{
		WorkspaceID: g.Workspace,
		ProjectID:   g.Project,
		Origin:      s.addresses.Origins.Of(host),
		Expires:     time.Unix(g.Expires, 0).UTC(),
	}, true, nil
}

func (s *Service) Serve(_ context.Context, host, requestPath string) (Served, error) {
	g, err := s.grantOf(host)
	if err != nil {
		return Served{}, err
	}
	stored, found, err := s.store.loadBuild(g.Build)
	if err != nil {
		return Served{}, err
	}
	if !found {
		return Served{}, ErrPreviewNotFound
	}
	asset, ok := stored.asset(resolvePath(stored, requestPath))
	if !ok {
		return Served{}, ErrPreviewNotFound
	}
	body, found, err := s.store.loadAsset(asset.SHA)
	if err != nil {
		return Served{}, err
	}
	if !found {
		return Served{}, ErrPreviewNotFound
	}
	return Served{ContentType: asset.Type, Body: body}, nil
}

func resolvePath(stored storedBuild, requestPath string) string {
	clean := path.Clean("/" + requestPath)
	for _, route := range stored.Routes {
		if route.Route == clean {
			return route.HTMLPath
		}
	}
	return strings.TrimPrefix(clean, "/")
}
