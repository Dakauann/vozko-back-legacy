package sitebuild

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"vozko/domain/cache"
	"vozko/domain/strictjson"
)

const (
	PreviewTTL = 30 * time.Minute
	BuildTTL   = 2 * time.Hour
	AssetTTL   = 4 * time.Hour

	buildKeyPrefix   = "site:build:"
	assetKeyPrefix   = "site:asset:"
	previewKeyPrefix = "site:preview:"
)

type storedAsset struct {
	Path string `json:"p"`
	Type string `json:"t"`
	SHA  string `json:"s"`
}

type storedRoute struct {
	Route    string `json:"r"`
	HTMLPath string `json:"h"`
}

type storedBuild struct {
	Created   int64         `json:"c"`
	Assets    []storedAsset `json:"a"`
	Routes    []storedRoute `json:"r"`
	Prerender string        `json:"x,omitempty"`
}

func (b storedBuild) asset(path string) (storedAsset, bool) {
	for _, a := range b.Assets {
		if a.Path == path {
			return a, true
		}
	}
	return storedAsset{}, false
}

type buildStore struct {
	cache cache.Cache
}

func bytesOf(value any) ([]byte, bool, error) {
	switch v := value.(type) {
	case nil:
		return nil, false, nil
	case string:
		return []byte(v), true, nil
	case []byte:
		return v, true, nil
	}
	return nil, false, fmt.Errorf("%w: unexpected cached value %T", ErrPreviewUnavailable, value)
}

func (s buildStore) get(key string) ([]byte, bool, error) {
	value, err := s.cache.Get(key)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %w", ErrPreviewUnavailable, err)
	}
	return bytesOf(value)
}

func (s buildStore) put(key string, value []byte, ttl time.Duration) error {
	if err := s.cache.Set(key, value, ttl); err != nil {
		return fmt.Errorf("%w: %w", ErrPreviewUnavailable, err)
	}
	return nil
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (s buildStore) saveBuild(hash string, result BuildResult, now time.Time) error {
	stored := storedBuild{Created: now.Unix(), Prerender: result.PrerenderPath}
	for _, asset := range result.Assets {
		sha := sha256Hex(asset.Body)
		if err := s.put(assetKeyPrefix+sha, asset.Body, AssetTTL); err != nil {
			return err
		}
		stored.Assets = append(stored.Assets, storedAsset{Path: asset.Path, Type: asset.ContentType, SHA: sha})
	}
	for _, route := range result.Routes {
		stored.Routes = append(stored.Routes, storedRoute{Route: route.Route, HTMLPath: route.HTMLPath})
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	return s.put(buildKeyPrefix+hash, raw, BuildTTL)
}

func (s buildStore) loadBuild(hash string) (storedBuild, bool, error) {
	raw, ok, err := s.get(buildKeyPrefix + hash)
	if err != nil || !ok {
		return storedBuild{}, false, err
	}
	var stored storedBuild
	if err := strictjson.Decode(raw, &stored); err != nil {
		return storedBuild{}, false, fmt.Errorf("%w: %w", ErrPreviewUnavailable, err)
	}
	return stored, true, nil
}

func (s buildStore) loadAsset(sha string) ([]byte, bool, error) {
	body, ok, err := s.get(assetKeyPrefix + sha)
	if err != nil || !ok {
		return nil, ok, err
	}
	if sha256Hex(body) != sha {
		return nil, false, fmt.Errorf("%w: a cached asset does not match its hash", ErrPreviewUnavailable)
	}
	return body, true, nil
}

func (s buildStore) saveGrant(id []byte, g grant, ttl time.Duration) error {
	raw, err := json.Marshal(g)
	if err != nil {
		return err
	}
	return s.put(previewKeyPrefix+hex.EncodeToString(id), raw, ttl)
}

func (s buildStore) loadGrant(id []byte) (grant, bool, error) {
	raw, ok, err := s.get(previewKeyPrefix + hex.EncodeToString(id))
	if err != nil || !ok {
		return grant{}, false, err
	}
	var g grant
	if err := strictjson.Decode(raw, &g); err != nil {
		return grant{}, false, fmt.Errorf("%w: %w", ErrPreviewUnavailable, err)
	}
	return g, true, nil
}
