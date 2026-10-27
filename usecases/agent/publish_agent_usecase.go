package agent_usecase

import (
	"context"
	"errors"
	"fmt"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/automationneeds"
	"vozko/domain/workspace"
	automationneeds_usecase "vozko/usecases/automationneeds"
)

const publishRepairBatch = 200

type AgentPublishing interface {
	Publish(ctx context.Context, agentID string, by workspace.Requester) (agent.Version, error)
	Publication(ctx context.Context, agentID string) (agent.Publication, error)
	Versions(ctx context.Context, agentID string) ([]agent.Version, error)
	Version(ctx context.Context, agentID string, version int) (agent.Version, error)
	Restore(ctx context.Context, agentID string, version int, by workspace.Requester) (*agent.Agent, error)
}

type SwitchingPublisher interface {
	PublishVersionSwitchingOff(ctx context.Context, agentID string, snapshot agent.Snapshot, publishedBy, workspaceID string, offs []automationneeds.SwitchOff) (agent.Version, []string, error)
}

type PublishPlanner interface {
	PlanAgent(ctx context.Context, draft *agent.Agent) (automationneeds_usecase.Plan, error)
	Done(ctx context.Context, plan automationneeds_usecase.Plan, containers []string)
}

type PublishDeps struct {
	Agents    agent.Repository
	Versions  agent.VersionRepository
	Switching SwitchingPublisher
	Planner   PublishPlanner
	Published agent.PublishedReader
	Update    agent.UpdateAgentUseCase
	Validator ToolValidator
}

type publishAgentUseCase struct {
	deps PublishDeps
}

func NewPublishAgentUseCase(deps PublishDeps) AgentPublishing {
	return &publishAgentUseCase{deps: deps}
}

func (uc *publishAgentUseCase) Publish(ctx context.Context, agentID string, by workspace.Requester) (agent.Version, error) {
	draft, err := uc.draft(agentID)
	if err != nil {
		return agent.Version{}, err
	}
	snapshot := agent.SnapshotOf(draft)
	current, err := uc.deps.Versions.CurrentVersion(ctx, agentID)
	if err != nil {
		return agent.Version{}, err
	}
	if current != nil && current.Snapshot.Same(snapshot) {
		return *current, nil
	}
	if uc.deps.Validator == nil {
		return agent.Version{}, agenttool.BlockedBy(agenttool.ErrNotPublishable, agenttool.Unavailable("a conferência das ferramentas não está ligada"))
	}
	report := uc.deps.Validator.Validate(ctx, draft, publishedToolsOr(uc.deps.Published, agentID, nil))
	if err := agenttool.BlockedBy(agenttool.ErrNotPublishable, report); err != nil {
		return agent.Version{}, err
	}
	if uc.deps.Planner == nil || uc.deps.Switching == nil {
		return agent.Version{}, automationneeds.ErrUnreadable
	}
	plan, err := uc.deps.Planner.PlanAgent(ctx, draft)
	if err != nil {
		return agent.Version{}, err
	}
	version, containers, err := uc.deps.Switching.PublishVersionSwitchingOff(ctx, agentID, snapshot, by.UserID, plan.WorkspaceID, plan.Offs)
	if err != nil {
		return agent.Version{}, err
	}
	uc.deps.Planner.Done(ctx, plan, containers)
	return version, nil
}

func (uc *publishAgentUseCase) Publication(ctx context.Context, agentID string) (agent.Publication, error) {
	draft, err := uc.draft(agentID)
	if err != nil {
		return agent.Publication{}, err
	}
	current, err := uc.deps.Versions.CurrentVersion(ctx, agentID)
	if err != nil {
		return agent.Publication{}, err
	}
	if current == nil {
		return agent.Publication{HasDraftChanges: true}, nil
	}
	version, at := current.Version, current.PublishedAt
	return agent.Publication{
		PublishedVersion: &version,
		PublishedAt:      &at,
		PublishedBy:      current.PublishedBy,
		HasDraftChanges:  !current.Snapshot.Same(agent.SnapshotOf(draft)),
	}, nil
}

func (uc *publishAgentUseCase) Versions(ctx context.Context, agentID string) ([]agent.Version, error) {
	return uc.deps.Versions.Versions(ctx, agentID)
}

func (uc *publishAgentUseCase) Version(ctx context.Context, agentID string, version int) (agent.Version, error) {
	return uc.deps.Versions.VersionOf(ctx, agentID, version)
}

func (uc *publishAgentUseCase) Restore(ctx context.Context, agentID string, version int, by workspace.Requester) (*agent.Agent, error) {
	chosen, err := uc.deps.Versions.VersionOf(ctx, agentID, version)
	if err != nil {
		return nil, err
	}
	pinned, ok := uc.deps.Versions.(interface {
		PinnedTools(ctx context.Context, agentID string, pins []agent.ToolPin) ([]agent.ToolBinding, error)
	})
	if !ok {
		return nil, errors.New("agent versions: pinned tools are not readable")
	}
	tools, err := pinned.PinnedTools(ctx, agentID, chosen.Snapshot.Tools)
	if err != nil {
		return nil, err
	}
	draft, err := uc.draft(agentID)
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, t := range draft.InternalTools {
		live[t.ID] = true
	}
	for i := range tools {
		if !live[tools[i].ID] {
			tools[i].ID = ""
		}
		tools[i].Revision = 0
	}
	s := chosen.Snapshot
	ragEnabled := s.RAGEnabled
	return uc.deps.Update.Execute(ctx, agentID, agent.UpdateAgentInput{
		Name: &s.Name, Description: &s.Description, MessagingPrompt: &s.MessagingPrompt, MessagingModel: &s.MessagingModel,
		AvatarURL: &s.AvatarURL, Tags: s.Tags, Metadata: s.Metadata, KnowledgeBaseIDs: s.KnowledgeBaseIDs,
		RAGEnabled: &ragEnabled, ReplaceRAGConfig: true, RAGConfig: s.RAGConfig, MediaIDs: s.MediaIDs, Variables: s.Variables,
		InternalTools: tools, RequestedBy: by,
	})
}

func (uc *publishAgentUseCase) draft(agentID string) (*agent.Agent, error) {
	draft, err := uc.deps.Agents.FindByID(agentID)
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, agent.ErrAgentNotFound
	}
	return draft, nil
}

type PublishExistingAgents struct {
	agents   agent.Repository
	versions agent.VersionRepository
	scanner  agent.UnpublishedScanner
}

func NewPublishExistingAgents(agents agent.Repository, versions agent.VersionRepository, scanner agent.UnpublishedScanner) *PublishExistingAgents {
	return &PublishExistingAgents{agents: agents, versions: versions, scanner: scanner}
}

func (p *PublishExistingAgents) Run(ctx context.Context) (int, error) {
	published := 0
	after := toolSecretRepairStart
	for {
		ids, err := p.scanner.UnpublishedAgentIDs(ctx, after, publishRepairBatch)
		if err != nil {
			return published, err
		}
		if len(ids) == 0 {
			return published, nil
		}
		for _, id := range ids {
			after = id
			a, err := p.agents.FindByID(id)
			if err != nil {
				return published, fmt.Errorf("agent %s: %w", id, err)
			}
			if a == nil {
				continue
			}
			if _, err := p.versions.PublishVersion(ctx, id, agent.SnapshotOf(a), ""); err != nil {
				return published, fmt.Errorf("agent %s: %w", id, err)
			}
			published++
		}
	}
}
