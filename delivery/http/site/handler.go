package sitehttp

import (
	"context"
	"io"
	"net/http"
	"strings"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/shared"
	"vozko/domain/site"
	workspace_domain "vozko/domain/workspace"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
	"vozko/usecases/siteinspect"
)

const (
	defaultPageSize   = 30
	envelopeBytes     = 64 << 10
	sourceBodyBytes   = 2*site.MaxProjectBytes + envelopeBytes
	settingsBodyBytes = 512 << 10
)

type Service interface {
	Create(ctx context.Context, in site_usecase.CreateInput) (*site_usecase.View, error)
	Duplicate(ctx context.Context, in site_usecase.DuplicateInput) (*site_usecase.View, error)
	List(ctx context.Context, q site_usecase.ListQuery) ([]site_usecase.Project, int64, error)
	Get(ctx context.Context, workspaceID, id string) (*site_usecase.View, error)
	Update(ctx context.Context, workspaceID, id string, expected int64, ch site_usecase.Change) (*site_usecase.Project, error)
	Delete(ctx context.Context, workspaceID, id string) error
	SaveFiles(ctx context.Context, in site_usecase.SaveFilesInput) (*site_usecase.View, error)
	Edit(ctx context.Context, in site_usecase.EditInput) (*site_usecase.EditResult, error)
	ListRevisions(ctx context.Context, workspaceID, projectID string, limit, offset int) ([]site_usecase.Revision, int64, error)
	SaveVersion(ctx context.Context, in site_usecase.SaveVersionInput) (*site_usecase.View, error)
	Restore(ctx context.Context, in site_usecase.RestoreInput) (*site_usecase.View, error)
	GetSettings(ctx context.Context, workspaceID, projectID string) (*site_usecase.StoredSettings, error)
	PutSettings(ctx context.Context, workspaceID, projectID string, expected int64, raw []byte) (*site_usecase.StoredSettings, error)
	Preview(ctx context.Context, workspaceID, projectID string) (site_usecase.PreviewResult, error)
	CachedPreview(ctx context.Context, workspaceID, projectID string) (site_usecase.PreviewResult, error)
	Export(ctx context.Context, workspaceID, projectID string) (string, []byte, error)
	CreateDesignSystem(ctx context.Context, workspaceID string, system site.DesignSystem, isDefault bool) (*site_usecase.DesignSystemRecord, error)
	ListDesignSystems(ctx context.Context, workspaceID string) ([]site_usecase.DesignSystemRecord, error)
	GetDesignSystem(ctx context.Context, workspaceID, id string) (*site_usecase.DesignSystemRecord, error)
	UpdateDesignSystem(ctx context.Context, workspaceID, id string, expected int64, system site.DesignSystem, isDefault bool) (*site_usecase.DesignSystemRecord, error)
	DeleteDesignSystem(ctx context.Context, workspaceID, id string) error
	SaveSnippet(ctx context.Context, in site_usecase.SaveSnippetInput) (*site_usecase.SavedSnippet, error)
}

type Sections interface {
	List(ctx context.Context, workspaceID string, stack site.Stack) ([]sitebuild.SectionInfo, error)
}

type Outliner interface {
	Outline(ctx context.Context, v *site_usecase.View) siteinspect.Report
}

type Access interface {
	Execute(userID, workspaceID string, resource workspace_domain.Resource, action workspace_domain.Action) error
}

type Handler struct {
	svc       Service
	outlines  Outliner
	sections  Sections
	access    Access
	apiOrigin string
}

func NewHandler(svc Service, outlines Outliner, sections Sections, access Access, apiOrigin string) *Handler {
	return &Handler{svc: svc, outlines: outlines, sections: sections, access: access, apiOrigin: strings.TrimSuffix(apiOrigin, "/")}
}

func (h *Handler) canEdit(r *http.Request, workspaceID string) bool {
	person, ok := httpx.PersonFromContext(r.Context())
	return ok && h.access.Execute(person.UserID, workspaceID, workspace_domain.ResourceSites, workspace_domain.ActionUpdate) == nil
}

func presentProject(p site_usecase.Project) ProjectResponse {
	return ProjectResponse{
		ID: p.ID, Name: p.Name, Slug: p.Slug, Stack: string(p.Stack), Status: string(p.Status), WorkingRevisionID: p.WorkingRevisionID,
		PublishedDeploymentID: p.PublishedDeploymentID, DesignSystemID: p.DesignSystemID, ThumbnailURL: p.ThumbnailURL, HasUnpublishedChanges: p.HasUnpublishedChanges,
		PublishState: string(p.PublishState()), Version: p.Version, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func presentListItem(p site_usecase.Project) ListItemResponse {
	return ListItemResponse{ProjectResponse: presentProject(p), PrimaryURL: p.PrimaryURL}
}

func presentRevision(r site_usecase.Revision) RevisionResponse {
	kind, id := r.Author.Parts()
	return RevisionResponse{ID: r.ID, ParentID: r.ParentID, Author: string(r.Author), AuthorKind: string(kind), AuthorID: id, Label: r.Label, Kind: string(r.Kind), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func presentFacts(v *site_usecase.View) FactsResponse {
	f := v.Facts()
	return FactsResponse{Routes: f.Routes, RoutesIssue: f.RoutesIssue, Tokens: f.Tokens, Knobs: f.Knobs, KnobsIssue: f.KnobsIssue, Breakpoints: f.Breakpoints}
}

func presentSaved(v *site_usecase.View) SavedResponse {
	return SavedResponse{Project: presentProject(v.Project), Revision: presentRevision(v.Revision), Revs: revs(v.Files), Changes: changesOf(v), Facts: presentFacts(v)}
}

func changesOf(v *site_usecase.View) site.FileChanges {
	c := v.Changes
	if c.Files == nil {
		c.Files = map[string]site.File{}
	}
	c.Deleted, c.Renamed = nonNil(c.Deleted), nonNil(c.Renamed)
	return c
}

func presentSettings(s *site_usecase.StoredSettings) SettingsResponse {
	if s == nil {
		return SettingsResponse{}
	}
	return SettingsResponse{Settings: s.Settings, Version: s.Version, UpdatedAt: s.UpdatedAt}
}

func presentDesign(d site_usecase.DesignSystemRecord) DesignSystemResponse {
	return DesignSystemResponse{ID: d.ID, System: d.System, IsDefault: d.IsDefault, Version: d.Version, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
}

func revs(files map[string]site.File) map[string]uint64 {
	out := make(map[string]uint64, len(files))
	for path, file := range files {
		out[path] = file.Rev
	}
	return out
}

func presentFiles(v *site_usecase.View) FilesResponse {
	return FilesResponse{Project: presentProject(v.Project), Files: v.Files, Revision: presentRevision(v.Revision), Changes: changesOf(v), Facts: presentFacts(v)}
}

func presentDetail(v *site_usecase.View) DetailResponse {
	return DetailResponse{
		Project: presentProject(v.Project), Files: v.Files, Settings: presentSettings(v.Settings), Revision: presentRevision(v.Revision), HasUnpublishedChanges: v.HasUnpublishedChanges,
		PublishState: string(v.Project.PublishState()), PrimaryURL: v.PrimaryURL, Facts: presentFacts(v),
	}
}

func decode(w http.ResponseWriter, r *http.Request, limit int64, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	return httpx.DecodeStrictJSON(w, r, into)
}

func pageOf(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	limit, limitOK := httpx.QueryInt(r, "limit", defaultPageSize)
	offset, offsetOK := httpx.QueryInt(r, "offset", 0)
	if !limitOK || !offsetOK || limit < 1 || limit > site_usecase.MaxListLimit || offset < 0 {
		response.WriteErrorWithCode(w, http.StatusBadRequest, CodePageInvalid, "limit vai de 1 a 100 e offset começa em 0", nil)
		return 0, 0, false
	}
	return limit, offset, true
}

func (h *Handler) person(w http.ResponseWriter, r *http.Request) (string, shared.Person, bool) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return "", shared.Person{}, false
	}
	person, ok := httpx.RequirePerson(w, r)
	return workspaceID, person, ok
}

func (h *Handler) writeConflict(w http.ResponseWriter, r *http.Request, workspaceID, id string) {
	current, err := h.svc.Get(r.Context(), workspaceID, id)
	if err != nil {
		writeError(w, err, "Failed to load the page")
		return
	}
	httpx.WriteVersionConflict(w, "A página foi salva em outra aba ou por outra pessoa", presentDetail(current))
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, workspaceID, id string, err error, fallback string) {
	if isConflict(err) {
		h.writeConflict(w, r, workspaceID, id)
		return
	}
	writeError(w, err, fallback)
}

func (h *Handler) decodeSource(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, sourceBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, err, "Failed to read the body")
		return false
	}
	r.Body = io.NopCloser(bytesReader(raw))
	return httpx.DecodeStrictJSON(w, r, into)
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
