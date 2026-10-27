package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	"vozko/domain/testinput"
)

var errVersionsUnsupported = errors.New("agent repository: the inner store has no versions")

func publishedCacheKey(id string) string { return "agent_published:" + id }

type cachedPublished struct {
	Agent   agent.Agent `json:"agent"`
	Version int         `json:"version"`
}

func (r *CachedRepository) FindPublished(id string) (*agent.Agent, error) {
	reader, ok := r.inner.(agent.PublishedReader)
	if !ok {
		return nil, errVersionsUnsupported
	}
	if strings.TrimSpace(id) == "" || r.shared == nil {
		return reader.FindPublished(id)
	}
	if raw, err := r.shared.GetString(publishedCacheKey(id)); err == nil && raw != "" {
		var entry cachedPublished
		if json.Unmarshal([]byte(raw), &entry) == nil && entry.Agent.ID != "" && entry.Version > 0 {
			entry.Agent.RunningVersion = entry.Version
			return &entry.Agent, nil
		}
	}
	a, err := reader.FindPublished(id)
	if err != nil || a == nil {
		return a, err
	}
	if data, mErr := json.Marshal(cachedPublished{Agent: *a, Version: a.RunningVersion}); mErr == nil {
		_ = r.shared.SetString(publishedCacheKey(id), string(data), r.ttl)
	}
	return a, nil
}

func (r *CachedRepository) versions() (agent.VersionRepository, error) {
	versions, ok := r.inner.(agent.VersionRepository)
	if !ok {
		return nil, errVersionsUnsupported
	}
	return versions, nil
}

func (r *CachedRepository) PublishVersion(ctx context.Context, agentID string, snapshot agent.Snapshot, publishedBy string) (agent.Version, error) {
	versions, err := r.versions()
	if err != nil {
		return agent.Version{}, err
	}
	version, err := versions.PublishVersion(ctx, agentID, snapshot, publishedBy)
	if err == nil {
		r.invalidate(agentID)
	}
	return version, err
}

type switchingPublisher interface {
	PublishVersionSwitchingOff(ctx context.Context, agentID string, snapshot agent.Snapshot, publishedBy, workspaceID string, offs []automationneeds.SwitchOff) (agent.Version, []string, error)
}

func (r *CachedRepository) PublishVersionSwitchingOff(ctx context.Context, agentID string, snapshot agent.Snapshot, publishedBy, workspaceID string, offs []automationneeds.SwitchOff) (agent.Version, []string, error) {
	inner, ok := r.inner.(switchingPublisher)
	if !ok {
		return agent.Version{}, nil, errVersionsUnsupported
	}
	version, containers, err := inner.PublishVersionSwitchingOff(ctx, agentID, snapshot, publishedBy, workspaceID, offs)
	if err == nil {
		r.invalidate(agentID)
	}
	return version, containers, err
}

func (r *CachedRepository) CurrentVersion(ctx context.Context, agentID string) (*agent.Version, error) {
	versions, err := r.versions()
	if err != nil {
		return nil, err
	}
	return versions.CurrentVersion(ctx, agentID)
}

func (r *CachedRepository) Versions(ctx context.Context, agentID string) ([]agent.Version, error) {
	versions, err := r.versions()
	if err != nil {
		return nil, err
	}
	return versions.Versions(ctx, agentID)
}

func (r *CachedRepository) VersionOf(ctx context.Context, agentID string, version int) (agent.Version, error) {
	versions, err := r.versions()
	if err != nil {
		return agent.Version{}, err
	}
	return versions.VersionOf(ctx, agentID, version)
}

type PublishedView struct {
	agent.Repository
	published agent.PublishedReader
}

func NewPublishedView(inner agent.Repository) (*PublishedView, error) {
	published, ok := inner.(agent.PublishedReader)
	if !ok {
		return nil, errVersionsUnsupported
	}
	return &PublishedView{Repository: inner, published: published}, nil
}

func (v *PublishedView) FindByID(id string) (*agent.Agent, error) {
	return v.published.FindPublished(id)
}

var (
	_ agent.PublishedReader   = (*CachedRepository)(nil)
	_ agent.VersionRepository = (*CachedRepository)(nil)
)

func (r *CachedRepository) UnpublishedAgentIDs(ctx context.Context, afterID string, limit int) ([]string, error) {
	scanner, ok := r.inner.(agent.UnpublishedScanner)
	if !ok {
		return nil, errVersionsUnsupported
	}
	return scanner.UnpublishedAgentIDs(ctx, afterID, limit)
}

func (r *CachedRepository) PinnedTools(ctx context.Context, agentID string, pins []agent.ToolPin) ([]agent.ToolBinding, error) {
	pinned, ok := r.inner.(interface {
		PinnedTools(ctx context.Context, agentID string, pins []agent.ToolPin) ([]agent.ToolBinding, error)
	})
	if !ok {
		return nil, errVersionsUnsupported
	}
	return pinned.PinnedTools(ctx, agentID, pins)
}

func (r *CachedRepository) SaveToolTestInputs(ctx context.Context, agentID, toolID string, inputs []testinput.TestInput) error {
	store, ok := r.inner.(agent.ToolTestInputStore)
	if !ok {
		return errVersionsUnsupported
	}
	err := store.SaveToolTestInputs(ctx, agentID, toolID, inputs)
	if err == nil {
		r.invalidate(agentID)
	}
	return err
}
