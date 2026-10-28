package sitehosting

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"

	"vozko/domain/sitepublish"
)

var blobAddress = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Bucket interface {
	Exists(ctx context.Context, key string) (bool, error)
	PutStream(ctx context.Context, key, contentType string, body io.Reader) error
}

type Keys interface {
	Put(ctx context.Context, key string, value []byte) error
	Delete(ctx context.Context, key string) error
}

type Store struct {
	bucket Bucket
	keys   Keys
}

func New(bucket Bucket, keys Keys) *Store {
	return &Store{bucket: bucket, keys: keys}
}

func (s *Store) HasBlob(ctx context.Context, sha string) (bool, error) {
	if !blobAddress.MatchString(sha) {
		return false, fmt.Errorf("sitehosting: %q is not a blob address", sha)
	}
	return s.bucket.Exists(ctx, sitepublish.BlobKey(sha))
}

func (s *Store) PutBlob(ctx context.Context, sha, contentType string, body []byte) error {
	if !blobAddress.MatchString(sha) || sitepublish.SHA256Hex(body) != sha {
		return fmt.Errorf("sitehosting: the body does not match the address %q", sha)
	}
	return s.bucket.PutStream(ctx, sitepublish.BlobKey(sha), contentType, bytes.NewReader(body))
}

func (s *Store) PutManifest(ctx context.Context, deploymentID string, body []byte) error {
	return s.bucket.PutStream(ctx, sitepublish.ManifestKey(deploymentID), "application/json", bytes.NewReader(body))
}

func (s *Store) PutHost(ctx context.Context, hostname string, value []byte) error {
	return s.keys.Put(ctx, sitepublish.HostKey(hostname), value)
}

func (s *Store) DeleteHost(ctx context.Context, hostname string) error {
	return s.keys.Delete(ctx, sitepublish.HostKey(hostname))
}

func (s *Store) PutDeployment(ctx context.Context, deploymentID string, value []byte) error {
	return s.keys.Put(ctx, sitepublish.DeploymentKey(deploymentID), value)
}

func (s *Store) DeleteDeployment(ctx context.Context, deploymentID string) error {
	return s.keys.Delete(ctx, sitepublish.DeploymentKey(deploymentID))
}
