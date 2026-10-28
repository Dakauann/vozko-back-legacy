package site_usecase

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"
)

func (s *Service) Preview(ctx context.Context, workspaceID, projectID string) (PreviewResult, error) {
	v, err := s.Get(ctx, workspaceID, projectID)
	if err != nil {
		return PreviewResult{}, err
	}
	res, err := s.previewer.Preview(ctx, PreviewRequest{
		WorkspaceID: workspaceID, ProjectID: projectID, RevisionID: v.Revision.ID, Project: v.Source(), Settings: v.Settings.Settings,
	})
	var failed *BuildFailedError
	var issues *IssuesError
	if errors.As(err, &failed) || errors.As(err, &issues) {
		return PreviewResult{}, err
	}
	if err != nil {
		return PreviewResult{}, fmt.Errorf("%w: %w", ErrPreviewUnavailable, err)
	}
	if res.URL == "" {
		return PreviewResult{}, ErrPreviewUnavailable
	}
	s.rememberPreview(v, res)
	return res, nil
}

const (
	previewKeyPrefix = "site:preview:last:"
	PreviewKeep      = 25 * time.Minute
)

type previewStamp struct {
	URL             string `json:"url"`
	RevisionID      string `json:"revisionId"`
	SettingsVersion int64  `json:"settingsVersion"`
}

func settingsVersion(v *View) int64 {
	if v.Settings == nil {
		return 0
	}
	return v.Settings.Version
}

func (s *Service) rememberPreview(v *View, res PreviewResult) {
	raw, err := json.Marshal(previewStamp{URL: res.URL, RevisionID: res.RevisionID, SettingsVersion: settingsVersion(v)})
	if err == nil {
		err = s.previews.SetString(previewKeyPrefix+v.Project.ID, string(raw), PreviewKeep)
	}
	if err != nil {
		log.Printf("[sites] the preview of %s could not be remembered: %v", v.Project.ID, err)
	}
}

func (s *Service) CachedPreview(ctx context.Context, workspaceID, projectID string) (PreviewResult, error) {
	v, err := s.Get(ctx, workspaceID, projectID)
	if err != nil {
		return PreviewResult{}, err
	}
	raw, err := s.previews.GetString(previewKeyPrefix + v.Project.ID)
	if err != nil {
		return PreviewResult{}, fmt.Errorf("%w: %w", ErrPreviewUnavailable, err)
	}
	var stamp previewStamp
	if raw == "" || json.Unmarshal([]byte(raw), &stamp) != nil || stamp.URL == "" ||
		stamp.RevisionID != v.Revision.ID || stamp.SettingsVersion != settingsVersion(v) {
		return PreviewResult{}, ErrPreviewNotBuilt
	}
	return PreviewResult{URL: stamp.URL, RevisionID: stamp.RevisionID}, nil
}

func (s *Service) Export(ctx context.Context, workspaceID, projectID string) (string, []byte, error) {
	v, err := s.working(ctx, workspaceID, projectID)
	if err != nil {
		return "", nil, err
	}
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	modified := v.Revision.UpdatedAt
	if modified.IsZero() {
		modified = time.Unix(0, 0).UTC()
	}
	project := v.Source()
	for _, path := range project.Paths() {
		w, err := archive.CreateHeader(&zip.FileHeader{Name: path, Method: zip.Deflate, Modified: modified})
		if err != nil {
			return "", nil, err
		}
		if _, err := w.Write([]byte(project.Files[path].Content)); err != nil {
			return "", nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return "", nil, err
	}
	return v.Project.Slug + ".zip", buf.Bytes(), nil
}
