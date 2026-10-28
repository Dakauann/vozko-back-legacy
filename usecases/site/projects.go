package site_usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

const (
	fallbackSlug   = "pagina"
	maxSlugRetries = 5
)

type CreateInput struct {
	WorkspaceID    string
	UserID         string
	Name           string
	Brief          string
	Stack          site.Stack
	Starter        string
	DesignSystemID string
	Origin         Origin
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*View, error) {
	if in.WorkspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	author := PersonAuthor(in.UserID)
	if in.UserID == "" || !author.Valid() {
		return nil, ErrAuthorInvalid
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = site.NameFromBrief(in.Brief)
	}
	if !validName(in.Name) {
		return nil, ErrNameInvalid
	}
	starter, err := StarterFor(in.Stack, in.Starter)
	if err != nil {
		return nil, err
	}
	designID, err := s.designFor(ctx, in.WorkspaceID, in.DesignSystemID)
	if err != nil {
		return nil, err
	}
	project := starter.Project()
	manifest, blobs := encodeFiles(project.Files)
	now := s.now()
	p := Project{
		ID: s.newID(), WorkspaceID: in.WorkspaceID, Name: strings.TrimSpace(in.Name), Stack: in.Stack, Status: StatusDraft,
		DesignSystemID: designID, Origin: in.Origin, Version: 1, CreatedBy: in.UserID, CreatedAt: now, UpdatedAt: now,
	}
	rev := Revision{
		ID: s.newID(), WorkspaceID: in.WorkspaceID, ProjectID: p.ID, Manifest: manifest, Author: author,
		Label: starter.Title, Kind: RevisionCheckpoint, CreatedAt: now, UpdatedAt: now,
	}
	p.WorkingRevisionID = rev.ID
	settings := StoredSettings{ProjectID: p.ID, WorkspaceID: in.WorkspaceID, Settings: starter.Settings, Version: 1, UpdatedAt: now}
	return s.insert(ctx, p, rev, blobs, settings)
}

func (s *Service) insert(ctx context.Context, p Project, rev Revision, blobs []Blob, settings StoredSettings) (*View, error) {
	for attempt := 0; ; attempt++ {
		slug, err := s.freeSlug(ctx, p.WorkspaceID, p.Name)
		if err != nil {
			return nil, err
		}
		p.Slug = slug
		created, err := s.store.CreateProject(ctx, NewProject{Project: p, Revision: rev, Blobs: blobs, Settings: settings, FileRevs: newFileRevs(rev.Manifest, nil)})
		if errors.Is(err, ErrSlugTaken) && attempt < maxSlugRetries {
			continue
		}
		if err != nil {
			return nil, err
		}
		return s.Get(ctx, p.WorkspaceID, created.ID)
	}
}

type DuplicateInput struct {
	WorkspaceID string
	UserID      string
	ProjectID   string
	Name        string
}

const copySuffix = " (cópia)"

func (s *Service) Duplicate(ctx context.Context, in DuplicateInput) (*View, error) {
	if in.WorkspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	author := PersonAuthor(in.UserID)
	if in.UserID == "" || !author.Valid() {
		return nil, ErrAuthorInvalid
	}
	source, err := s.Get(ctx, in.WorkspaceID, in.ProjectID)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = source.Project.Name + copySuffix
	}
	if !validName(name) {
		return nil, ErrNameInvalid
	}
	manifest, blobs := encodeFiles(source.Files)
	now := s.now()
	p := Project{
		ID: s.newID(), WorkspaceID: in.WorkspaceID, Name: name, Stack: source.Project.Stack, Status: StatusDraft,
		DesignSystemID: source.Project.DesignSystemID, Version: 1, CreatedBy: in.UserID, CreatedAt: now, UpdatedAt: now,
	}
	rev := Revision{
		ID: s.newID(), WorkspaceID: in.WorkspaceID, ProjectID: p.ID, Manifest: manifest, Author: author,
		Label: "Cópia de " + source.Project.Name, Kind: RevisionCheckpoint, CreatedAt: now, UpdatedAt: now,
	}
	p.WorkingRevisionID = rev.ID
	var copied site.Settings
	if source.Settings != nil {
		copied = source.Settings.Settings
	}
	settings := StoredSettings{ProjectID: p.ID, WorkspaceID: in.WorkspaceID, Settings: copied, Version: 1, UpdatedAt: now}
	return s.insert(ctx, p, rev, blobs, settings)
}

func (s *Service) designFor(ctx context.Context, workspaceID, requested string) (string, error) {
	if requested != "" {
		d, err := s.store.GetDesignSystem(ctx, workspaceID, requested)
		if err != nil {
			return "", err
		}
		return d.ID, nil
	}
	list, err := s.store.ListDesignSystems(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	for _, d := range list {
		if d.IsDefault {
			return d.ID, nil
		}
	}
	return "", nil
}

func slugBase(name string) string {
	slug := sitepublish.SlugCandidate(name)
	if len(slug) < sitepublish.MinSlugLength {
		return fallbackSlug
	}
	if sitepublish.ValidateSlug(slug) != nil {
		return sitepublish.SlugCandidate(slug + " " + fallbackSlug)
	}
	return slug
}

func (s *Service) freeSlug(ctx context.Context, workspaceID, name string) (string, error) {
	base := slugBase(name)
	taken, err := s.store.TakenSlugs(ctx, workspaceID, base)
	if err != nil {
		return "", err
	}
	used := map[string]bool{}
	for _, slug := range taken {
		used[slug] = true
	}
	if !used[base] {
		return base, nil
	}
	for n := 2; ; n++ {
		suffix := "-" + strconv.Itoa(n)
		candidate := strings.TrimRight(base[:min(len(base), sitepublish.MaxSlugLength-len(suffix))], "-") + suffix
		if !used[candidate] {
			return candidate, nil
		}
	}
}

func (s *Service) Get(ctx context.Context, workspaceID, id string) (*View, error) {
	v, err := s.working(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	settings, err := s.store.GetSettings(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	v.Settings = settings
	projects := []Project{v.Project}
	if err := s.present(ctx, workspaceID, projects); err != nil {
		return nil, err
	}
	v.Project, v.HasUnpublishedChanges, v.PrimaryURL = projects[0], projects[0].HasUnpublishedChanges, projects[0].PrimaryURL
	return v, nil
}

func (s *Service) List(ctx context.Context, q ListQuery) ([]Project, int64, error) {
	if q.WorkspaceID == "" {
		return nil, 0, ErrWorkspaceRequired
	}
	if q.Status != "" && !q.Status.Valid() {
		return nil, 0, ErrStatusInvalid
	}
	q.Limit = min(max(q.Limit, 1), MaxListLimit)
	q.Offset = max(q.Offset, 0)
	q.Search = strings.TrimSpace(q.Search)
	items, total, err := s.store.ListProjects(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	if err := s.present(ctx, q.WorkspaceID, items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

type Change struct {
	Name           *string
	Slug           *string
	DesignSystemID *string
	Archived       *bool
}

func (s *Service) Update(ctx context.Context, workspaceID, id string, expected int64, ch Change) (*Project, error) {
	if err := shared.RequireVersion(expected); err != nil {
		return nil, err
	}
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	p, err := s.store.GetProject(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	if err := shared.ExpectVersion(p.Version, &expected); err != nil {
		return nil, err
	}
	if err := s.applyChange(ctx, p, ch); err != nil {
		return nil, err
	}
	unpublished, err := s.unpublished(ctx, *p)
	if err != nil {
		return nil, err
	}
	if err := s.store.UpdateProject(ctx, p, expected); err != nil {
		return nil, err
	}
	p.HasUnpublishedChanges = unpublished
	return p, nil
}

func (s *Service) applyChange(ctx context.Context, p *Project, ch Change) error {
	if ch.Name != nil {
		if !validName(*ch.Name) {
			return ErrNameInvalid
		}
		p.Name = strings.TrimSpace(*ch.Name)
	}
	if ch.Slug != nil && *ch.Slug != p.Slug {
		if p.Status == StatusPublished || p.Status == StatusSuspended {
			return ErrProjectPublished
		}
		if err := sitepublish.ValidateSlug(*ch.Slug); err != nil {
			return err
		}
		p.Slug = *ch.Slug
	}
	if ch.DesignSystemID != nil {
		p.DesignSystemID = ""
		if *ch.DesignSystemID != "" {
			d, err := s.store.GetDesignSystem(ctx, p.WorkspaceID, *ch.DesignSystemID)
			if err != nil {
				return err
			}
			p.DesignSystemID = d.ID
		}
	}
	if ch.Archived != nil {
		return archive(p, *ch.Archived)
	}
	return nil
}

func archive(p *Project, archived bool) error {
	switch {
	case archived && (p.Status == StatusPublished || p.Status == StatusSuspended):
		return ErrProjectPublished
	case archived:
		p.Status = StatusArchived
	case p.Status == StatusArchived:
		p.Status = StatusDraft
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, workspaceID, id string) error {
	if workspaceID == "" {
		return ErrWorkspaceRequired
	}
	p, err := s.store.GetProject(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if p.Status == StatusPublished || p.Status == StatusSuspended || p.PublishedDeploymentID != "" {
		return fmt.Errorf("%w: %s", ErrProjectPublished, p.Status)
	}
	if err := s.forms.Deactivate(ctx, p.ID); err != nil {
		return err
	}
	if err := s.retirement.Retire(ctx, workspaceID, p.ID); err != nil {
		return err
	}
	return s.store.DeleteProject(ctx, workspaceID, id)
}

func (s *Service) SetPublication(ctx context.Context, workspaceID, id string, status Status, deploymentID string) (*Project, error) {
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	switch {
	case status == StatusPublished || status == StatusSuspended:
		if deploymentID == "" {
			return nil, ErrPublicationInvalid
		}
	case status == StatusDraft:
		if deploymentID != "" {
			return nil, ErrPublicationInvalid
		}
	default:
		return nil, ErrPublicationInvalid
	}
	return s.store.SetPublication(ctx, workspaceID, id, status, deploymentID)
}
