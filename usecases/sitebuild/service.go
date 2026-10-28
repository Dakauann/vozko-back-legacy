package sitebuild

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"time"

	"vozko/domain/cache"
	"vozko/domain/inlinemarkup"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/domain/sitesubmission"
)

const minSigningKeyLength = 32

const (
	RuntimeFile    = "vz-runtime.js"
	PowFile        = "pow.js"
	BridgeFile     = "vz-editor-bridge.js"
	ScreenshotFile = "vz-screenshot.js"
)

type Config struct {
	Addresses  sitepublish.Addresses
	SigningKey string
	APIOrigin  string
	Runtime    map[string]string
}

type Deps struct {
	Builder    Builder
	Cache      cache.Cache
	Media      MediaLocator
	Sections   SectionLibrary
	WhatsApp   WhatsAppLinks
	Workspaces WorkspaceNames
	Now        func() time.Time
	Random     io.Reader
}

type Service struct {
	addresses  sitepublish.Addresses
	key        []byte
	api        string
	runtime    map[string]string
	builder    Builder
	store      buildStore
	media      MediaLocator
	sections   SectionLibrary
	whatsapp   WhatsAppLinks
	workspaces WorkspaceNames
	now        func() time.Time
	random     io.Reader
}

type PreviewInput struct {
	WorkspaceID string
	ProjectID   string
	RevisionID  string
	Project     site.Project
	Settings    site.Settings
}

type PreviewOutput struct {
	URL      string
	Revision string
	Build    string
}

type CandidateInput struct {
	WorkspaceID   string
	ProjectID     string
	Project       site.Project
	Settings      site.Settings
	FormKeys      map[string]string
	PrimaryOrigin string
	OGImages      map[string]string
}

type ExtractInput struct {
	WorkspaceID string
	ProjectID   string
	Project     site.Project
	Node        string
	Rev         uint64
	Bases       map[string][]site.File
}

type CandidateOutput struct {
	URL    string
	Result BuildResult
}

type ExecuteInput struct {
	WorkspaceID string
	ProjectID   string
	Project     site.Project
	Batch       site.Batch
	Bases       map[string][]site.File
}

func validOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return false
	}
	host := u.Hostname()
	return u.Scheme == "https" || (u.Scheme == "http" && (host == "localhost" || host == "127.0.0.1"))
}

func NewService(cfg Config, deps Deps) (*Service, error) {
	switch {
	case deps.Builder == nil || deps.Cache == nil || deps.Media == nil || deps.WhatsApp == nil || deps.Workspaces == nil:
		return nil, errors.New("sitebuild: the builder, the cache, the media locator, the WhatsApp links and the workspace names are required")
	case len(cfg.SigningKey) < minSigningKeyLength:
		return nil, fmt.Errorf("sitebuild: the preview signing key needs at least %d characters", minSigningKeyLength)
	case strings.TrimSpace(cfg.Addresses.Domain) == "" || strings.ContainsAny(cfg.Addresses.Domain, "/:@ "):
		return nil, errors.New("sitebuild: the platform domain is required")
	case !validOrigin(cfg.APIOrigin):
		return nil, errors.New("sitebuild: the api origin must be an https origin")
	}
	for _, file := range []string{RuntimeFile, PowFile, BridgeFile, ScreenshotFile} {
		if strings.TrimSpace(cfg.Runtime[file]) == "" {
			return nil, fmt.Errorf("sitebuild: the runtime file %s is required", file)
		}
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	random := deps.Random
	if random == nil {
		random = rand.Reader
	}
	return &Service{
		addresses:  cfg.Addresses,
		key:        []byte(cfg.SigningKey),
		api:        cfg.APIOrigin,
		runtime:    cfg.Runtime,
		builder:    deps.Builder,
		store:      buildStore{cache: deps.Cache},
		media:      deps.Media,
		sections:   deps.Sections,
		whatsapp:   deps.WhatsApp,
		workspaces: deps.Workspaces,
		now:        now,
		random:     random,
	}, nil
}

func (s *Service) Execute(ctx context.Context, in ExecuteInput) (ExecuteResult, error) {
	texts := map[int][]inlinemarkup.Run{}
	var mediaIDs []string
	sections, jsxSections := map[string]string{}, map[string]string{}
	for i, o := range in.Batch.Operations {
		switch o.Op {
		case site.OpSetText:
			if o.Text == nil {
				continue
			}
			runs, err := runsOf(*o.Text)
			if err != nil {
				return ExecuteResult{}, err
			}
			texts[i] = runs
		case site.OpReplaceImage:
			if o.MediaID != "" && !slices.Contains(mediaIDs, o.MediaID) {
				mediaIDs = append(mediaIDs, o.MediaID)
			}
		case site.OpInsertSection:
			if o.LibraryKey == "" || s.sections == nil {
				continue
			}
			section, ok, err := s.sections.Section(ctx, in.WorkspaceID, in.Project.Stack, o.LibraryKey)
			if err != nil {
				return ExecuteResult{}, fmt.Errorf("%w: %w", ErrSectionsUnavailable, err)
			}
			if !ok {
				continue
			}
			if section.Stack == site.StackReact {
				jsxSections[o.LibraryKey] = section.Source
			} else {
				sections[o.LibraryKey] = section.Source
			}
		}
	}
	media, err := s.locate(ctx, in.WorkspaceID, mediaIDs, false)
	if err != nil {
		return ExecuteResult{}, err
	}
	return s.builder.Execute(ctx, ExecuteRequest{
		Project: in.Project, Operations: in.Batch.Operations, Texts: texts, Media: media, Sections: sections, JSXSections: jsxSections, Bases: in.Bases,
	})
}

func (s *Service) Extract(ctx context.Context, in ExtractInput) (ExtractResult, error) {
	return s.builder.Extract(ctx, ExtractRequest{Project: in.Project, Node: in.Node, Rev: in.Rev, Bases: in.Bases})
}

func runsOf(text string) ([]inlinemarkup.Run, error) {
	if strings.TrimSpace(text) == "" {
		return []inlinemarkup.Run{}, nil
	}
	paragraphs, err := inlinemarkup.Parse(text)
	if err != nil || len(paragraphs) != 1 {
		return nil, fmt.Errorf("%w: %s", site.ErrEditRefused, site.CodeTextMarkupInvalid)
	}
	return paragraphs[0].Runs, nil
}

func (s *Service) locate(ctx context.Context, workspaceID string, ids []string, required bool) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	found, err := s.media.Locate(ctx, workspaceID, ids)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMediaUnavailable, err)
	}
	out := map[string]string{}
	for _, id := range ids {
		raw, ok := found[id]
		u, parseErr := url.Parse(raw)
		if !ok || parseErr != nil || u.Scheme != "https" || u.Host == "" {
			if required {
				return nil, fmt.Errorf("%w: %s", ErrMediaUnavailable, id)
			}
			continue
		}
		out[id] = raw
	}
	return out, nil
}

func socialImageURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && !strings.ContainsAny(raw, "\"'<> ")
}

func (s *Service) shell(ctx context.Context, workspaceID string, p site.Project, settings site.Settings, forms map[string]string, primaryOrigin string, ogImages map[string]string) (Shell, error) {
	if primaryOrigin != "" && !(strings.HasPrefix(primaryOrigin, "https://") && validOrigin(primaryOrigin)) {
		return Shell{}, fmt.Errorf("%w: the primary origin must be an https origin", ErrBuilderRefused)
	}
	for route, image := range ogImages {
		if !socialImageURL(image) {
			return Shell{}, fmt.Errorf("%w: the social image of %s must be an https url", ErrBuilderRefused, route)
		}
	}
	raw, _ := p.Content(site.ManifestPath)
	manifest, err := site.DecodeManifest(p.Stack, []byte(raw))
	if err != nil {
		return Shell{}, &ProjectInvalid{Issues: []site.Issue{{Code: site.CodeManifestInvalid, Path: site.ManifestPath, Detail: err.Error()}}}
	}
	var ogIDs []string
	for _, route := range manifest.Routes {
		if id := settings.Pages[route.Path].OGMediaID; id != "" && !slices.Contains(ogIDs, id) {
			ogIDs = append(ogIDs, id)
		}
	}
	if id := settings.Site.FaviconMediaID; id != "" && !slices.Contains(ogIDs, id) {
		ogIDs = append(ogIDs, id)
	}
	images, err := s.locate(ctx, workspaceID, ogIDs, true)
	if err != nil {
		return Shell{}, err
	}
	pages := map[string]PageMeta{}
	for _, route := range manifest.Routes {
		meta := settings.Pages[route.Path]
		ogImage := images[meta.OGMediaID]
		if meta.OGMediaID == "" {
			ogImage = ogImages[route.Path]
		}
		pages[route.Path] = PageMeta{Title: meta.Title, Description: meta.Description, Canonical: meta.Canonical, OGImage: ogImage, NoIndex: meta.NoIndex}
	}
	integrations, loaders := map[string]string{}, map[string]string{}
	for _, kind := range settings.Integrations.Enabled() {
		id, _ := settings.Integrations.ID(kind)
		loader, ok := site.IntegrationLoader(kind)
		if !ok {
			return Shell{}, fmt.Errorf("%w: integration %s has no loader", ErrBuilderRefused, kind)
		}
		integrations[string(kind)], loaders[string(kind)] = id, loader
	}
	if forms == nil {
		forms = map[string]string{}
	}
	shell := Shell{API: s.api, Forms: forms, Pages: pages, Integrations: integrations, Loaders: loaders, ConsentText: settings.Consent.Text, PrimaryOrigin: primaryOrigin, Favicon: images[settings.Site.FaviconMediaID]}
	if settings.Site.StructuredData {
		data, err := s.organization(ctx, workspaceID, primaryOrigin, shell.Favicon)
		if err != nil {
			return Shell{}, err
		}
		shell.StructuredData = []json.RawMessage{data}
	}
	return shell, nil
}

func (s *Service) organization(ctx context.Context, workspaceID, origin, logo string) (json.RawMessage, error) {
	name, err := s.workspaces.WorkspaceName(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: the workspace name: %w", ErrBuilderRefused, err)
	}
	data, err := site.OrganizationData(name, origin, logo)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuilderRefused, err)
	}
	return data, nil
}

func previewForms(p site.Project, settings site.Settings) map[string]string {
	forms := map[string]string{}
	for _, id := range sitepublish.FormIDs(p) {
		forms[id] = sitesubmission.PreviewFormKey
	}
	for id := range settings.Forms {
		forms[id] = sitesubmission.PreviewFormKey
	}
	return forms
}

func (s *Service) linkPreviewWhatsApp(ctx context.Context, workspaceID string, result BuildResult) (BuildResult, error) {
	resolve := func(a sitepublish.WhatsAppAnchor) (string, bool, error) {
		link, err := s.whatsapp.DirectLink(ctx, workspaceID, a.NumberRef, a.Text)
		if err != nil {
			return "", false, nil
		}
		return link, true, nil
	}
	assets := make([]Asset, len(result.Assets))
	for i, a := range result.Assets {
		assets[i] = a
		if !strings.HasPrefix(strings.ToLower(a.ContentType), "text/html") {
			continue
		}
		linked, err := sitepublish.LinkWhatsApp(string(a.Body), resolve)
		if err != nil {
			return BuildResult{}, &BuilderError{Kind: ErrBuilderReply, Message: a.Path + ": " + err.Error()}
		}
		assets[i].Body = []byte(linked)
	}
	result.Assets = assets
	return result, nil
}

type hashedRequest struct {
	Mode    Mode              `json:"mode"`
	Project site.Project      `json:"project"`
	Shell   Shell             `json:"shell"`
	Runtime map[string]string `json:"runtime"`
}

func requestHash(req BuildRequest) (string, error) {
	raw, err := json.Marshal(hashedRequest{Mode: req.Mode, Project: req.Project, Shell: req.Shell, Runtime: req.Runtime})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func validate(p site.Project) error {
	if structural, _ := site.Split(site.Validate(p)); len(structural) > 0 {
		return &ProjectInvalid{Issues: structural}
	}
	return nil
}

func (s *Service) request(ctx context.Context, mode Mode, workspaceID string, p site.Project, settings site.Settings, forms map[string]string, primaryOrigin string, ogImages map[string]string) (BuildRequest, string, error) {
	if err := validate(p); err != nil {
		return BuildRequest{}, "", err
	}
	shell, err := s.shell(ctx, workspaceID, p, settings, forms, primaryOrigin, ogImages)
	if err != nil {
		return BuildRequest{}, "", err
	}
	req := BuildRequest{Mode: mode, Project: p, Shell: shell, Runtime: s.runtime}
	hash, err := requestHash(req)
	if err != nil {
		return BuildRequest{}, "", err
	}
	return req, hash, nil
}

func (s *Service) build(ctx context.Context, workspaceID string, req BuildRequest, hash string) (BuildResult, error) {
	result, err := s.builder.Build(ctx, req)
	if err != nil {
		return BuildResult{}, err
	}
	if req.Mode == ModePreview && len(result.Errors) == 0 {
		if result, err = s.linkPreviewWhatsApp(ctx, workspaceID, result); err != nil {
			return BuildResult{}, err
		}
	}
	if len(result.Errors) > 0 {
		return BuildResult{}, &BuildFailure{Errors: result.Errors, Warnings: result.Warnings}
	}
	if issues := site.CheckImports(req.Project.Stack, result.Imports); len(issues) > 0 {
		return BuildResult{}, &ImportsRefused{Issues: issues}
	}
	if len(result.Routes) == 0 {
		return BuildResult{}, &BuilderError{Kind: ErrBuilderReply, Message: "the build has no routes"}
	}
	if err := s.store.saveBuild(hash, result, s.now()); err != nil {
		return BuildResult{}, err
	}
	return result, nil
}

func (s *Service) Preview(ctx context.Context, in PreviewInput) (PreviewOutput, error) {
	req, hash, err := s.request(ctx, ModePreview, in.WorkspaceID, in.Project, in.Settings, previewForms(in.Project, in.Settings), "", nil)
	if err != nil {
		return PreviewOutput{}, err
	}
	stored, ok, err := s.store.loadBuild(hash)
	if err != nil {
		return PreviewOutput{}, err
	}
	if !ok || s.now().Sub(time.Unix(stored.Created, 0)) > BuildTTL-PreviewTTL {
		if _, err := s.build(ctx, in.WorkspaceID, req, hash); err != nil {
			return PreviewOutput{}, err
		}
	}
	link, err := s.grant(in.WorkspaceID, in.ProjectID, hash)
	if err != nil {
		return PreviewOutput{}, err
	}
	return PreviewOutput{URL: link, Revision: in.RevisionID, Build: hash}, nil
}

func (s *Service) Candidate(ctx context.Context, in CandidateInput) (CandidateOutput, error) {
	req, hash, err := s.request(ctx, ModePublish, in.WorkspaceID, in.Project, in.Settings, in.FormKeys, in.PrimaryOrigin, in.OGImages)
	if err != nil {
		return CandidateOutput{}, err
	}
	result, err := s.build(ctx, in.WorkspaceID, req, hash)
	if err != nil {
		return CandidateOutput{}, err
	}
	link, err := s.grant(in.WorkspaceID, in.ProjectID, hash)
	if err != nil {
		return CandidateOutput{}, err
	}
	return CandidateOutput{URL: link, Result: result}, nil
}

func (s *Service) grant(workspaceID, projectID, hash string) (string, error) {
	id := make([]byte, grantIDBytes)
	if _, err := io.ReadFull(s.random, id); err != nil {
		return "", fmt.Errorf("%w: %w", ErrPreviewUnavailable, err)
	}
	g := grant{Workspace: workspaceID, Project: projectID, Build: hash, Expires: s.now().Add(PreviewTTL).Unix()}
	if err := s.store.saveGrant(id, g, PreviewTTL); err != nil {
		return "", err
	}
	return s.addresses.PreviewURL(encodeToken(s.key, id, g)), nil
}
