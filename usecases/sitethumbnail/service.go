package sitethumbnail

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"vozko/domain/imageguard"
	"vozko/domain/media"
	"vozko/domain/shared/deps"
	"vozko/usecases/siterender"
)

const (
	PreviewSpacing   = 10 * time.Minute
	captureTimeout   = 2 * time.Minute
	throttlePrefix   = "site:thumbnail:"
	mediaFolder      = "images"
	mediaExtension   = ".jpg"
	mediaDescription = "Miniatura do site"
)

type Renderer interface {
	Screenshot(ctx context.Context, url string, viewport siterender.Viewport) ([]byte, error)
}

type Uploader interface {
	UploadMedia(workspaceID string, data []byte, mediaName string, mediaType media.MediaType, description string) (media.Media, error)
}

type Projects interface {
	SetThumbnail(ctx context.Context, workspaceID, projectID, mediaID string) (string, error)
}

type MediaLibrary interface {
	GetMediaForWorkspace(workspaceID, mediaID string) (*media.Media, error)
	DeleteMedia(mediaID string) error
}

type Files interface {
	KeyFromURL(url string) (string, bool)
	DeleteFile(ctx context.Context, key string) error
}

type Throttle interface {
	SetNX(key string, value string, ttl time.Duration) (bool, error)
	GetString(key string) (string, error)
}

var (
	ErrImageInvalid = errors.New("sitethumbnail: the thumbnail is not a JPEG of a page")
	ImageLimits     = imageguard.Limits{MaxBytes: 1536 << 10, MinWidth: 320, MinHeight: 200, MaxWidth: 2560, MaxHeight: 2560}
)

type Upload struct {
	Accepted   bool
	RetryAfter time.Duration
}

type Deps struct {
	Renderer Renderer
	Uploader Uploader
	Projects Projects
	Media    MediaLibrary
	Files    Files
	Throttle Throttle
	Spawn    func(func())
	NewID    func() string
	Now      func() time.Time
}

type Service struct {
	d Deps
}

func New(d Deps) (*Service, error) {
	for name, dep := range map[string]any{"renderer": d.Renderer, "uploader": d.Uploader, "projects": d.Projects, "media": d.Media, "files": d.Files, "throttle": d.Throttle} {
		if deps.IsNil(dep) {
			return nil, fmt.Errorf("sitethumbnail: %s is required", name)
		}
	}
	if d.Spawn == nil {
		d.Spawn = func(fn func()) { go fn() }
	}
	if d.NewID == nil {
		d.NewID = uuid.NewString
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}, nil
}

func (s *Service) Upload(ctx context.Context, workspaceID, projectID string, image []byte) (Upload, error) {
	clean, _, err := imageguard.Clean(image, ImageLimits)
	if err != nil {
		return Upload{}, fmt.Errorf("%w: %w", ErrImageInvalid, err)
	}
	now := s.d.Now()
	fresh, err := s.d.Throttle.SetNX(throttlePrefix+projectID, strconv.FormatInt(now.Add(PreviewSpacing).Unix(), 10), PreviewSpacing)
	if err != nil {
		log.Printf("[sites] thumbnail of %s declined, the throttle is unreadable: %v", projectID, err)
		return Upload{RetryAfter: PreviewSpacing}, nil
	}
	if !fresh {
		return Upload{RetryAfter: s.retryAfter(projectID, now)}, nil
	}
	if err := s.store(ctx, workspaceID, projectID, clean); err != nil {
		return Upload{}, err
	}
	return Upload{Accepted: true, RetryAfter: PreviewSpacing}, nil
}

func (s *Service) retryAfter(projectID string, now time.Time) time.Duration {
	raw, err := s.d.Throttle.GetString(throttlePrefix + projectID)
	if err != nil {
		return PreviewSpacing
	}
	until, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return PreviewSpacing
	}
	return min(max(time.Unix(until, 0).Sub(now), 0), PreviewSpacing)
}

func (s *Service) Published(ctx context.Context, workspaceID, projectID, candidateURL string) {
	s.background(ctx, workspaceID, projectID, candidateURL)
}

func (s *Service) background(ctx context.Context, workspaceID, projectID, pageURL string) {
	if strings.TrimSpace(pageURL) == "" {
		return
	}
	detached := context.WithoutCancel(ctx)
	s.d.Spawn(func() {
		callCtx, cancel := context.WithTimeout(detached, captureTimeout)
		defer cancel()
		if err := s.Capture(callCtx, workspaceID, projectID, pageURL); err != nil {
			log.Printf("[sites] thumbnail of %s was not captured: %v", projectID, err)
		}
	})
}

func (s *Service) Capture(ctx context.Context, workspaceID, projectID, pageURL string) error {
	shot, err := s.d.Renderer.Screenshot(ctx, pageURL, siterender.Desktop)
	if err != nil {
		return err
	}
	return s.store(ctx, workspaceID, projectID, shot)
}

func (s *Service) store(ctx context.Context, workspaceID, projectID string, shot []byte) error {
	name := mediaFolder + "/" + workspaceID + "/" + s.d.NewID() + mediaExtension
	stored, err := s.d.Uploader.UploadMedia(workspaceID, shot, name, media.MediaTypeProductImage, mediaDescription)
	if err != nil {
		return err
	}
	previous, err := s.d.Projects.SetThumbnail(ctx, workspaceID, projectID, stored.ID)
	if err != nil {
		return errors.Join(err, s.remove(ctx, stored))
	}
	if previous != "" && previous != stored.ID {
		if err := s.removeByID(ctx, workspaceID, previous); err != nil {
			log.Printf("[sites] the previous thumbnail %s of %s stays in the library: %v", previous, projectID, err)
		}
	}
	return nil
}

func (s *Service) removeByID(ctx context.Context, workspaceID, mediaID string) error {
	row, err := s.d.Media.GetMediaForWorkspace(workspaceID, mediaID)
	if errors.Is(err, media.ErrMediaNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.remove(ctx, *row)
}

func (s *Service) remove(ctx context.Context, row media.Media) error {
	key, ok := s.d.Files.KeyFromURL(row.URL)
	if !ok {
		return fmt.Errorf("sitethumbnail: %s is not a stored library file", row.URL)
	}
	if err := s.d.Files.DeleteFile(ctx, key); err != nil {
		return err
	}
	return s.d.Media.DeleteMedia(row.ID)
}
