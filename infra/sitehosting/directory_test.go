package sitehosting

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeKeys struct {
	values map[string][]byte
	reads  int
	err    error
}

func (k *fakeKeys) Put(_ context.Context, key string, value []byte) error {
	k.values[key] = value
	return nil
}

func (k *fakeKeys) Delete(_ context.Context, key string) error {
	delete(k.values, key)
	return nil
}

func (k *fakeKeys) Get(_ context.Context, key string) ([]byte, bool, error) {
	k.reads++
	if k.err != nil {
		return nil, false, k.err
	}
	v, ok := k.values[key]
	return v, ok, nil
}

func TestDirectoryCachesAnswersIncludingAbsentHosts(t *testing.T) {
	keys := &fakeKeys{values: map[string][]byte{"host:a.test": []byte(`{"v":1,"deployment":"d1"}`)}}
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	dir := NewDirectory(keys, func() time.Time { return now })
	ctx := context.Background()
	for range 3 {
		s, found, err := dir.Serving(ctx, "host:a.test")
		if err != nil || !found || s.DeploymentID != "d1" {
			t.Fatalf("got %+v %v %v", s, found, err)
		}
		if _, found, err := dir.Serving(ctx, "host:b.test"); err != nil || found {
			t.Fatalf("got %v %v", found, err)
		}
	}
	if keys.reads != 2 {
		t.Fatalf("each key is read once while fresh, got %d reads", keys.reads)
	}
	now = now.Add(DirectoryFreshness + time.Second)
	dir.Serving(ctx, "host:a.test")
	if keys.reads != 3 {
		t.Fatalf("a stale entry is read again, got %d", keys.reads)
	}
}

func TestDirectoryWritesRefreshThisProcessAtOnce(t *testing.T) {
	keys := &fakeKeys{values: map[string][]byte{}}
	dir := NewDirectory(keys, time.Now)
	ctx := context.Background()
	if _, found, _ := dir.Serving(ctx, "host:a.test"); found {
		t.Fatal("nothing yet")
	}
	if err := dir.Put(ctx, "host:a.test", []byte(`{"v":1,"deployment":"d2"}`)); err != nil {
		t.Fatal(err)
	}
	if s, found, _ := dir.Serving(ctx, "host:a.test"); !found || s.DeploymentID != "d2" {
		t.Fatalf("got %+v %v", s, found)
	}
	if err := dir.Delete(ctx, "host:a.test"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := dir.Serving(ctx, "host:a.test"); found {
		t.Fatal("a deleted key stops serving at once")
	}
}

func TestDirectoryNeverCachesFailures(t *testing.T) {
	keys := &fakeKeys{values: map[string][]byte{"host:bad.test": []byte(`{"v":2}`)}, err: errors.New("db down")}
	dir := NewDirectory(keys, time.Now)
	ctx := context.Background()
	if _, _, err := dir.Serving(ctx, "host:a.test"); err == nil {
		t.Fatal("a failed read is an error")
	}
	keys.err = nil
	if _, _, err := dir.Serving(ctx, "host:bad.test"); err == nil {
		t.Fatal("an unreadable value is an error, never a page")
	}
	if keys.reads != 2 {
		t.Fatalf("got %d", keys.reads)
	}
	dir.Serving(ctx, "host:bad.test")
	if keys.reads != 3 {
		t.Fatal("failures are retried on the next request")
	}
}
