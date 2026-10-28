package sitebuild

import (
	"context"
	"crypto/hmac"
	"path"
	"strings"

	"vozko/domain/hostname"
)

type Served struct {
	ContentType string
	Body        []byte
}

func (s *Service) Serve(_ context.Context, host, requestPath string) (Served, error) {
	label, ok := s.addresses.PreviewToken(host)
	if !ok {
		return Served{}, ErrPreviewNotFound
	}
	tok, ok := decodeToken(label)
	if !ok || s.now().Unix() >= tok.expires {
		return Served{}, ErrPreviewNotFound
	}
	g, found, err := s.store.loadGrant(tok.id)
	if err != nil {
		return Served{}, err
	}
	if !found || g.Expires != tok.expires || !hmac.Equal(tokenMAC(s.key, tok.id, tok.expires, g), tok.mac) {
		return Served{}, ErrPreviewNotFound
	}
	if !sameHost(s.addresses.PreviewHost(encodeToken(s.key, tok.id, g)), hostname.Normalize(host)) {
		return Served{}, ErrPreviewNotFound
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
