package conversationad

import (
	"context"
	"errors"
	"fmt"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/sitepublish"
)

var ErrPageOriginForeign = errors.New("conversationad: the page origin belongs to another workspace than the conversation")

type PageProjects interface {
	PageProject(ctx context.Context, workspaceID, projectID string) (*conversation.PageProject, error)
}

type PageHostnames interface {
	ListByProject(ctx context.Context, workspaceID, projectID string) ([]sitepublish.HostnameRecord, error)
}

type PageReader struct {
	origins   conversation.PageOriginRepository
	entries   EntryWorkspaces
	projects  PageProjects
	hostnames PageHostnames
	addresses sitepublish.Origins
}

func NewPageReader(origins conversation.PageOriginRepository, entries EntryWorkspaces, projects PageProjects, hostnames PageHostnames, addresses sitepublish.Origins) *PageReader {
	return &PageReader{origins: origins, entries: entries, projects: projects, hostnames: hostnames, addresses: addresses}
}

func (r *PageReader) PageOrigin(ctx context.Context, entryID string, entryType shared.EntryType) (*conversation.PageOriginView, error) {
	origin, err := r.origins.Get(ctx, entryID, entryType)
	if err != nil || origin == nil {
		return nil, err
	}
	workspaceID, err := r.entries.GetEntryWorkspaceID(entryID, string(entryType))
	if err != nil {
		return nil, fmt.Errorf("workspace of %s:%s: %w", entryType, entryID, err)
	}
	if workspaceID != origin.WorkspaceID {
		return nil, ErrPageOriginForeign
	}
	view := &conversation.PageOriginView{ProjectID: origin.ProjectID, Route: origin.Route, Reference: origin.Reference, ArrivedAt: origin.ArrivedAt}
	project, err := r.projects.PageProject(ctx, origin.WorkspaceID, origin.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("page %s: %w", origin.ProjectID, err)
	}
	if project == nil {
		return view, nil
	}
	view.ProjectName = project.Name
	if !project.Live {
		return view, nil
	}
	hosts, err := r.hostnames.ListByProject(ctx, origin.WorkspaceID, origin.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("hostnames of page %s: %w", origin.ProjectID, err)
	}
	if base, ok := sitepublish.PrimaryOrigin(hosts, r.addresses); ok {
		view.URL = base + origin.Route
	}
	return view, nil
}
