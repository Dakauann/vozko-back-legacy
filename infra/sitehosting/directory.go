package sitehosting

import (
	"context"
	"time"

	"vozko/domain/sitepublish"
	"vozko/infra/cache"
)

const (
	DirectoryFreshness = 30 * time.Second
	directoryEntries   = 10_000
)

type KeyStore interface {
	Keys
	Get(ctx context.Context, key string) ([]byte, bool, error)
}

type directoryEntry struct {
	settings sitepublish.ServingSettings
	found    bool
	readAt   time.Time
}

type Directory struct {
	keys    KeyStore
	now     func() time.Time
	entries *cache.LRU[string, directoryEntry]
}

func NewDirectory(keys KeyStore, now func() time.Time) *Directory {
	return &Directory{keys: keys, now: now, entries: cache.NewLRU[string, directoryEntry](directoryEntries)}
}

func (d *Directory) Put(ctx context.Context, key string, value []byte) error {
	defer d.entries.Delete(key)
	return d.keys.Put(ctx, key, value)
}

func (d *Directory) Delete(ctx context.Context, key string) error {
	defer d.entries.Delete(key)
	return d.keys.Delete(ctx, key)
}

func (d *Directory) Serving(ctx context.Context, key string) (sitepublish.ServingSettings, bool, error) {
	now := d.now()
	if e, ok := d.entries.Get(key); ok && now.Sub(e.readAt) < DirectoryFreshness {
		return e.settings, e.found, nil
	}
	raw, found, err := d.keys.Get(ctx, key)
	if err != nil {
		return sitepublish.ServingSettings{}, false, err
	}
	entry := directoryEntry{found: found, readAt: now}
	if found {
		if entry.settings, err = sitepublish.DecodeServing(raw); err != nil {
			return sitepublish.ServingSettings{}, false, err
		}
	}
	d.entries.Put(key, entry)
	return entry.settings, entry.found, nil
}
