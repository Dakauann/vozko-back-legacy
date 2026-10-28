package sitehosting

import (
	"context"
	"fmt"

	"vozko/domain/sitepublish"
	"vozko/infra/cache"
)

const (
	manifestEntries = 1024
	blobEntries     = 512
	maxCachedBlob   = 256 << 10
)

type Reader interface {
	DownloadFile(ctx context.Context, key string) ([]byte, string, error)
}

type Files struct {
	reader    Reader
	manifests *cache.LRU[string, sitepublish.DeploymentManifest]
	blobs     *cache.LRU[string, []byte]
}

func NewFiles(reader Reader) *Files {
	return &Files{
		reader:    reader,
		manifests: cache.NewLRU[string, sitepublish.DeploymentManifest](manifestEntries),
		blobs:     cache.NewLRU[string, []byte](blobEntries),
	}
}

func (f *Files) Manifest(ctx context.Context, deploymentID string) (sitepublish.DeploymentManifest, error) {
	if m, ok := f.manifests.Get(deploymentID); ok {
		return m, nil
	}
	raw, _, err := f.reader.DownloadFile(ctx, sitepublish.ManifestKey(deploymentID))
	if err != nil {
		return sitepublish.DeploymentManifest{}, err
	}
	m, err := sitepublish.DecodeDeploymentManifest(raw)
	if err != nil {
		return sitepublish.DeploymentManifest{}, err
	}
	if m.DeploymentID != deploymentID {
		return sitepublish.DeploymentManifest{}, fmt.Errorf("sitehosting: the manifest at %s belongs to %s", deploymentID, m.DeploymentID)
	}
	f.manifests.Put(deploymentID, m)
	return m, nil
}

func (f *Files) Blob(ctx context.Context, sha string) ([]byte, error) {
	if !blobAddress.MatchString(sha) {
		return nil, fmt.Errorf("sitehosting: %q is not a blob address", sha)
	}
	if body, ok := f.blobs.Get(sha); ok {
		return body, nil
	}
	body, _, err := f.reader.DownloadFile(ctx, sitepublish.BlobKey(sha))
	if err != nil {
		return nil, err
	}
	if sitepublish.SHA256Hex(body) != sha {
		return nil, fmt.Errorf("sitehosting: the blob at %s does not match its address", sha)
	}
	if len(body) <= maxCachedBlob {
		f.blobs.Put(sha, body)
	}
	return body, nil
}
