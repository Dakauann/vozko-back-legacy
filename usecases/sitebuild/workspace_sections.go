package sitebuild

import (
	"context"
	"fmt"
	"slices"

	"vozko/domain/site"
)

type SnippetSource interface {
	Snippets(ctx context.Context, workspaceID string) ([]site.DesignSnippet, error)
}

type WorkspaceSections struct {
	library  *Library
	snippets SnippetSource
}

var _ SectionLibrary = (*WorkspaceSections)(nil)

func NewWorkspaceSections(library *Library, snippets SnippetSource) *WorkspaceSections {
	return &WorkspaceSections{library: library, snippets: snippets}
}

type workspaceSection struct {
	info   SectionInfo
	source Section
}

func snippetStacks(stack site.Stack) []site.Stack {
	if stack == site.StackReact {
		return []site.Stack{site.StackReact}
	}
	return slices.Clone(sectionStacks)
}

func (w *WorkspaceSections) all(ctx context.Context, workspaceID string) ([]workspaceSection, error) {
	var out []workspaceSection
	taken := map[string]bool{}
	for _, info := range w.library.List() {
		taken[info.Key] = true
		out = append(out, workspaceSection{info: info, source: Section{Stack: site.StackHTML, Source: w.library.sources[info.Key]}})
	}
	snippets, err := w.snippets.Snippets(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSectionsUnavailable, err)
	}
	for _, snippet := range snippets {
		if taken[snippet.Key] || !snippet.Stack.Valid() {
			continue
		}
		taken[snippet.Key] = true
		out = append(out, workspaceSection{
			info:   SnippetSection(snippet),
			source: Section{Stack: snippet.Stack, Source: snippet.Content},
		})
	}
	return out, nil
}

func (w *WorkspaceSections) List(ctx context.Context, workspaceID string, stack site.Stack) ([]SectionInfo, error) {
	if stack != "" && !slices.Contains(sectionStacks, stack) {
		return nil, ErrSectionStackInvalid
	}
	all, err := w.all(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := []SectionInfo{}
	for _, s := range all {
		if stack == "" || slices.Contains(s.info.Stacks, stack) {
			out = append(out, s.info)
		}
	}
	return out, nil
}

func (w *WorkspaceSections) Section(ctx context.Context, workspaceID string, stack site.Stack, key string) (Section, bool, error) {
	if !slices.Contains(sectionStacks, stack) {
		return Section{}, false, nil
	}
	if source, ok := w.library.Section(stack, key); ok {
		return Section{Stack: site.StackHTML, Source: source}, true, nil
	}
	all, err := w.all(ctx, workspaceID)
	if err != nil {
		return Section{}, false, err
	}
	for _, s := range all {
		if s.info.Key == key {
			return s.source, slices.Contains(s.info.Stacks, stack), nil
		}
	}
	return Section{}, false, nil
}
