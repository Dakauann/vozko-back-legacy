package sitehosting

import (
	"context"
	"errors"
	"strings"
	"testing"

	"vozko/domain/sitepublish"
)

type fakeReader struct {
	objects map[string][]byte
	reads   map[string]int
}

func (r *fakeReader) DownloadFile(_ context.Context, key string) ([]byte, string, error) {
	r.reads[key]++
	body, ok := r.objects[key]
	if !ok {
		return nil, "", errors.New("NoSuchKey")
	}
	return body, "application/octet-stream", nil
}

func manifestBody(t *testing.T, id string) []byte {
	t.Helper()
	page := []byte("<html></html>")
	raw, err := sitepublish.DeploymentManifest{
		DeploymentID: id, ProjectID: "p1", APIOrigin: "https://api.vozkoia.com",
		Files:  map[string]sitepublish.FileEntry{"/index.html": sitepublish.NewFileEntry(page, "text/html", "/index.html")},
		Routes: map[string]string{"/": "/index.html"},
	}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestFilesDecodeAndKeepManifests(t *testing.T) {
	reader := &fakeReader{objects: map[string][]byte{"deploys/d1.json": manifestBody(t, "d1"), "deploys/d2.json": manifestBody(t, "d1"), "deploys/d3.json": []byte("{")}, reads: map[string]int{}}
	files := NewFiles(reader)
	ctx := context.Background()
	for range 3 {
		m, err := files.Manifest(ctx, "d1")
		if err != nil || m.DeploymentID != "d1" {
			t.Fatalf("got %+v %v", m, err)
		}
	}
	if reader.reads["deploys/d1.json"] != 1 {
		t.Fatalf("a manifest never changes, got %d reads", reader.reads["deploys/d1.json"])
	}
	if _, err := files.Manifest(ctx, "d2"); err == nil {
		t.Fatal("a manifest of another deployment is refused")
	}
	if _, err := files.Manifest(ctx, "d3"); err == nil {
		t.Fatal("an unreadable manifest is refused")
	}
	if _, err := files.Manifest(ctx, "missing"); err == nil {
		t.Fatal("a missing manifest is an error")
	}
}

func TestFilesCheckBlobsAgainstTheirAddressAndKeepSmallOnes(t *testing.T) {
	small := []byte("body{}")
	large := []byte(strings.Repeat("a", maxCachedBlob+1))
	tampered := sitepublish.SHA256Hex([]byte("original"))
	reader := &fakeReader{objects: map[string][]byte{
		sitepublish.BlobKey(sitepublish.SHA256Hex(small)): small,
		sitepublish.BlobKey(sitepublish.SHA256Hex(large)): large,
		sitepublish.BlobKey(tampered):                     []byte("changed"),
	}, reads: map[string]int{}}
	files := NewFiles(reader)
	ctx := context.Background()
	for range 2 {
		if body, err := files.Blob(ctx, sitepublish.SHA256Hex(small)); err != nil || string(body) != "body{}" {
			t.Fatalf("got %s %v", body, err)
		}
		if _, err := files.Blob(ctx, sitepublish.SHA256Hex(large)); err != nil {
			t.Fatal(err)
		}
	}
	if reader.reads[sitepublish.BlobKey(sitepublish.SHA256Hex(small))] != 1 || reader.reads[sitepublish.BlobKey(sitepublish.SHA256Hex(large))] != 2 {
		t.Fatalf("got %v", reader.reads)
	}
	if _, err := files.Blob(ctx, tampered); err == nil {
		t.Fatal("a blob that does not match its address is refused")
	}
	if _, err := files.Blob(ctx, "../secret"); err == nil {
		t.Fatal("a malformed address is refused")
	}
}
