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
	DefaultDesignSystemName = "Padrão"
	fallbackSnippetSlug     = "secao"
	maxSnippetKeyLength     = 64
	snippetSaveAttempts     = 3
)

type SaveSnippetInput struct {
	WorkspaceID string
	ProjectID   string
	Node        string
	Rev         uint64
	Name        string
}

type SavedSnippet struct {
	DesignSystem DesignSystemRecord
	Snippet      site.DesignSnippet
}

func (s *Service) SaveSnippet(ctx context.Context, in SaveSnippetInput) (*SavedSnippet, error) {
	name := strings.TrimSpace(in.Name)
	if !validName(name) {
		return nil, ErrNameInvalid
	}
	v, err := s.working(ctx, in.WorkspaceID, in.ProjectID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Node) == "" {
		return nil, &EditRefusal{Failures: []site.Failure{{Code: site.CodeFieldRequired, Field: "node"}}}
	}
	project := v.Source()
	bases, err := s.bases(ctx, v, site.NodeStaleRevs(project, in.Node, in.Rev))
	if err != nil {
		return nil, err
	}
	extracted, err := s.executor.Extract(ctx, ExtractRequest{WorkspaceID: in.WorkspaceID, ProjectID: in.ProjectID, Project: project, Node: in.Node, Rev: in.Rev, Bases: bases})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuilderUnavailable, err)
	}
	if extracted.Failure != nil {
		return nil, &EditRefusal{Failures: []site.Failure{*extracted.Failure}}
	}
	for attempt := 1; ; attempt++ {
		saved, err := s.appendSnippet(ctx, v.Project, name, extracted.Source)
		if errors.Is(err, shared.ErrVersionConflict) && attempt < snippetSaveAttempts {
			continue
		}
		return saved, err
	}
}

func (s *Service) appendSnippet(ctx context.Context, p Project, name, source string) (*SavedSnippet, error) {
	home, err := s.snippetHome(ctx, p)
	if err != nil {
		return nil, err
	}
	list, err := s.store.ListDesignSystems(ctx, p.WorkspaceID)
	if err != nil {
		return nil, err
	}
	snippet := site.DesignSnippet{Key: freeSnippetKey(name, list), Name: name, Stack: p.Stack, Content: source}
	system := home.System
	system.Snippets = append(append([]site.DesignSnippet{}, system.Snippets...), snippet)
	total := 0
	for _, existing := range system.Snippets {
		total += len(existing.Content)
	}
	if total > site.MaxProjectBytes {
		return nil, ErrSnippetsFull
	}
	if err := system.Validate(); err != nil {
		return nil, err
	}
	expected := home.Version
	home.System, home.UpdatedAt = system, s.now()
	if err := s.store.UpdateDesignSystem(ctx, home, expected); err != nil {
		return nil, err
	}
	return &SavedSnippet{DesignSystem: *home, Snippet: snippet}, nil
}

func (s *Service) snippetHome(ctx context.Context, p Project) (*DesignSystemRecord, error) {
	if p.DesignSystemID != "" {
		return s.store.GetDesignSystem(ctx, p.WorkspaceID, p.DesignSystemID)
	}
	list, err := s.store.ListDesignSystems(ctx, p.WorkspaceID)
	if err != nil {
		return nil, err
	}
	for _, d := range list {
		if d.IsDefault {
			return &d, nil
		}
	}
	return s.CreateDesignSystem(ctx, p.WorkspaceID, site.DesignSystem{Name: DefaultDesignSystemName}, true)
}

func freeSnippetKey(name string, systems []DesignSystemRecord) string {
	used := map[string]bool{}
	for _, d := range systems {
		for _, snippet := range d.System.Snippets {
			used[snippet.Key] = true
		}
	}
	slug := sitepublish.SlugCandidate(name)
	if slug == "" {
		slug = fallbackSnippetSlug
	}
	base := site.SnippetKeyPrefix + slug
	base = strings.TrimRight(base[:min(len(base), maxSnippetKeyLength)], "-")
	if !used[base] {
		return base
	}
	for n := 2; ; n++ {
		suffix := "-" + strconv.Itoa(n)
		candidate := strings.TrimRight(base[:min(len(base), maxSnippetKeyLength-len(suffix))], "-") + suffix
		if !used[candidate] {
			return candidate
		}
	}
}

func (s *Service) Snippets(ctx context.Context, workspaceID string) ([]site.DesignSnippet, error) {
	return SnippetsOf(ctx, s.store, workspaceID)
}

func SnippetsOf(ctx context.Context, store DesignSystemStore, workspaceID string) ([]site.DesignSnippet, error) {
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	list, err := store.ListDesignSystems(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := []site.DesignSnippet{}
	for _, d := range list {
		if d.IsDefault {
			out = append(out, d.System.Snippets...)
		}
	}
	for _, d := range list {
		if !d.IsDefault {
			out = append(out, d.System.Snippets...)
		}
	}
	return out, nil
}
