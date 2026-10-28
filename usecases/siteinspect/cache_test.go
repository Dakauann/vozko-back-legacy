package siteinspect

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	site_usecase "vozko/usecases/site"
)

type memoryCache struct {
	values map[string]string
	ttls   map[string]time.Duration
	err    error
}

func (m *memoryCache) GetString(key string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.values[key], nil
}

func (m *memoryCache) SetString(key, value string, ttl time.Duration) error {
	if m.err != nil {
		return m.err
	}
	m.values[key], m.ttls[key] = value, ttl
	return nil
}

func TestTheSameBuildIsCapturedOnce(t *testing.T) {
	documents := &fakeDocuments{pages: map[string]string{"/": home, "/obrigado": thanks}}
	cache := &memoryCache{values: map[string]string{}, ttls: map[string]time.Duration{}}
	previews := &countingPreviews{urls: []string{"https://a.previa.test/", "https://b.previa.test/"}, build: "build-1"}
	svc := Service{Previews: previews, Cache: cache}
	first := svc.Inspect(context.Background(), view(), documents)
	second := svc.Inspect(context.Background(), view(), documents)
	if !first.Known || !second.Known || len(documents.asked) != 1 {
		t.Fatalf("one capture per build: %d", len(documents.asked))
	}
	if second.PreviewURL != "https://b.previa.test/" || len(second.Gaps) != len(first.Gaps) || len(second.Routes) != 2 {
		t.Fatalf("the cached report carries the fresh preview: %+v", second)
	}
	for key, ttl := range cache.ttls {
		if ttl != InspectionKeep || !strings.HasPrefix(key, "site:inspect:advisory:") {
			t.Fatalf("%s kept for %s", key, ttl)
		}
	}
	changed := view()
	changed.Settings.Settings.Consent.Banner = true
	svc.Inspect(context.Background(), changed, documents)
	if len(documents.asked) != 2 {
		t.Fatal("other settings are another inspection")
	}
	previews.build = "build-2"
	svc.Inspect(context.Background(), view(), documents)
	if len(documents.asked) != 3 {
		t.Fatal("another build is captured again")
	}
}

func TestAnUnreadableCacheCapturesAndUnknownReportsAreNotKept(t *testing.T) {
	documents := &fakeDocuments{err: errors.New("no tab")}
	cache := &memoryCache{values: map[string]string{}, ttls: map[string]time.Duration{}}
	svc := Service{Previews: &countingPreviews{urls: []string{"https://a.previa.test/"}, build: "b"}, Cache: cache}
	if r := svc.Inspect(context.Background(), view(), documents); r.Known || len(cache.values) != 0 {
		t.Fatalf("a failed capture is not kept: %+v", cache.values)
	}
	stale := strings.Replace(thanks, "&quot;pages/obrigado.html&quot;:2", "&quot;pages/obrigado.html&quot;:1", 1)
	documents.err, documents.pages = nil, map[string]string{"/": home, "/obrigado": stale}
	if r := svc.Inspect(context.Background(), view(), documents); r.Known || len(cache.values) != 0 {
		t.Fatalf("a stale capture is not kept: %+v", cache.values)
	}
	cache.err = errors.New("redis down")
	documents.pages = map[string]string{"/": home, "/obrigado": thanks}
	if r := svc.Inspect(context.Background(), view(), documents); !r.Known {
		t.Fatalf("the cache is only a shortcut: %+v", r)
	}
}

type countingPreviews struct {
	urls  []string
	build string
	n     int
}

func (c *countingPreviews) Preview(context.Context, string, string) (site_usecase.PreviewResult, error) {
	url := c.urls[min(c.n, len(c.urls)-1)]
	c.n++
	return site_usecase.PreviewResult{URL: url, RevisionID: "r1", Build: c.build}, nil
}
