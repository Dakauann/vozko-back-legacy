package sitehosting

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type fakeBucket struct {
	objects map[string][]byte
	types   map[string]string
	err     error
}

func (b *fakeBucket) Exists(_ context.Context, key string) (bool, error) {
	if b.err != nil {
		return false, b.err
	}
	_, ok := b.objects[key]
	return ok, nil
}

func (b *fakeBucket) PutStream(_ context.Context, key, contentType string, body io.Reader) error {
	if b.err != nil {
		return b.err
	}
	raw, _ := io.ReadAll(body)
	b.objects[key] = raw
	b.types[key] = contentType
	return nil
}

type recordingKeys struct {
	values  map[string][]byte
	deleted []string
}

func (k *recordingKeys) Put(_ context.Context, key string, value []byte) error {
	k.values[key] = value
	return nil
}

func (k *recordingKeys) Delete(_ context.Context, key string) error {
	k.deleted = append(k.deleted, key)
	delete(k.values, key)
	return nil
}

func TestStoreWritesContentAddressedBlobsManifestsAndHostKeys(t *testing.T) {
	bucket := &fakeBucket{objects: map[string][]byte{}, types: map[string]string{}}
	keys := &recordingKeys{values: map[string][]byte{}}
	store := New(bucket, keys)
	ctx := context.Background()
	sha := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

	if has, err := store.HasBlob(ctx, sha); err != nil || has {
		t.Fatalf("got %v %v", has, err)
	}
	if err := store.PutBlob(ctx, sha, "text/css", []byte("abc")); err != nil {
		t.Fatal(err)
	}
	if has, _ := store.HasBlob(ctx, sha); !has || bucket.types["blobs/"+sha] != "text/css" {
		t.Fatalf("blob missing: %v", bucket.objects)
	}
	if err := store.PutBlob(ctx, sha, "text/css", []byte("abd")); err == nil {
		t.Fatal("a body that does not match its address must refuse")
	}
	if err := store.PutBlob(ctx, "../x", "text/css", []byte("abc")); err == nil {
		t.Fatal("a malformed address must refuse")
	}
	if err := store.PutManifest(ctx, "dep_1", []byte(`{}`)); err != nil || !bytes.Equal(bucket.objects["deploys/dep_1.json"], []byte(`{}`)) || bucket.types["deploys/dep_1.json"] != "application/json" {
		t.Fatalf("manifest: %v", err)
	}
	if err := store.PutHost(ctx, "loja.paginas.example", []byte(`{"v":1}`)); err != nil || string(keys.values["host:loja.paginas.example"]) != `{"v":1}` {
		t.Fatalf("host: %v %v", err, keys.values)
	}
	if err := store.PutDeployment(ctx, "dep_1", []byte(`{"v":1}`)); err != nil || string(keys.values["deploy:dep_1"]) != `{"v":1}` {
		t.Fatalf("deployment: %v %v", err, keys.values)
	}
	if err := store.DeleteHost(ctx, "loja.paginas.example"); err != nil || keys.deleted[0] != "host:loja.paginas.example" {
		t.Fatalf("delete: %v %v", err, keys.deleted)
	}
	if err := store.DeleteDeployment(ctx, "dep_1"); err != nil || keys.deleted[1] != "deploy:dep_1" {
		t.Fatalf("delete deployment: %v %v", err, keys.deleted)
	}
}

func TestStoreSurfacesBucketErrors(t *testing.T) {
	store := New(&fakeBucket{err: errors.New("r2 down")}, &recordingKeys{values: map[string][]byte{}})
	if _, err := store.HasBlob(context.Background(), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"); err == nil {
		t.Fatal("expected error")
	}
}
