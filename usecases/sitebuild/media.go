package sitebuild

import (
	"context"
	"errors"
	"net/url"

	"vozko/domain/media"
)

type MediaReader interface {
	GetMediaForWorkspace(workspaceID, mediaID string) (*media.Media, error)
}

type repositoryMedia struct {
	repo MediaReader
}

func MediaFromRepository(repo MediaReader) MediaLocator {
	return repositoryMedia{repo: repo}
}

func (r repositoryMedia) Locate(_ context.Context, workspaceID string, mediaIDs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range mediaIDs {
		row, err := r.repo.GetMediaForWorkspace(workspaceID, id)
		if errors.Is(err, media.ErrMediaNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if row == nil || row.WorkspaceID != workspaceID || row.Type.Private() {
			continue
		}
		if u, err := url.Parse(row.URL); err == nil && u.Scheme == "https" && u.Host != "" {
			out[id] = row.URL
		}
	}
	return out, nil
}
