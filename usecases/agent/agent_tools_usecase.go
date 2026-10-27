package agent_usecase

import (
	"context"
	"fmt"
	"strings"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/testinput"
	"vozko/domain/tools"
	"vozko/domain/workspace"
)

type agentToolsUseCase struct {
	agents    agent.Repository
	update    agent.UpdateAgentUseCase
	registry  tools.Service
	revisions agent.ToolRevisionReader
	validator ToolValidator
	published agent.PublishedReader
}

func NewAgentToolsUseCase(agents agent.Repository, update agent.UpdateAgentUseCase, registry tools.Service, revisions agent.ToolRevisionReader) agent.AgentToolsUseCase {
	return &agentToolsUseCase{agents: agents, update: update, registry: registry, revisions: revisions}
}

func (uc *agentToolsUseCase) List(_ context.Context, agentID string) ([]agent.ToolBinding, error) {
	current, err := uc.load(agentID)
	if err != nil {
		return nil, err
	}
	return current.InternalTools, nil
}

func (uc *agentToolsUseCase) Create(ctx context.Context, agentID string, draft agent.ToolDraft, by workspace.Requester) (agent.ToolBinding, error) {
	current, err := uc.load(agentID)
	if err != nil {
		return agent.ToolBinding{}, err
	}
	def, ok := definitionsByName(uc.registry)[strings.ToLower(strings.TrimSpace(draft.Type))]
	if !ok {
		return agent.ToolBinding{}, fmt.Errorf("%w: %s", agenttool.ErrTypeUnknown, draft.Type)
	}
	if def.Instances.Effective() == tools.InstancesMany && strings.TrimSpace(draft.Description) == "" {
		return agent.ToolBinding{}, agenttool.ErrDescriptionRequired
	}
	next := append(cloneBindings(current.InternalTools), agent.ToolBinding{
		Name: def.Name, Function: strings.TrimSpace(draft.Name), Description: strings.TrimSpace(draft.Description),
		Config: draft.Config, Visibility: draft.Visibility,
	})
	saved, err := uc.save(ctx, agentID, next, nil, by)
	if err != nil {
		return agent.ToolBinding{}, err
	}
	return added(current.InternalTools, saved)
}

func (uc *agentToolsUseCase) Update(ctx context.Context, agentID, toolID string, edit agent.ToolEdit, by workspace.Requester) (agent.ToolBinding, error) {
	current, err := uc.load(agentID)
	if err != nil {
		return agent.ToolBinding{}, err
	}
	next := cloneBindings(current.InternalTools)
	i := indexOf(next, toolID)
	if i < 0 {
		return agent.ToolBinding{}, fmt.Errorf("%w: %s", agenttool.ErrToolNotFound, toolID)
	}
	def, ok := definitionsByName(uc.registry)[strings.ToLower(next[i].Name)]
	if ok && def.Instances.Effective() == tools.InstancesMany && strings.TrimSpace(edit.Description) == "" {
		return agent.ToolBinding{}, agenttool.ErrDescriptionRequired
	}
	next[i].Function = strings.TrimSpace(edit.Name)
	next[i].Description = strings.TrimSpace(edit.Description)
	next[i].Config = edit.Config
	next[i].Visibility = edit.Visibility
	saved, err := uc.save(ctx, agentID, next, map[string]int{toolID: edit.Revision}, by)
	if err != nil {
		return agent.ToolBinding{}, err
	}
	return find(saved, toolID)
}

func (uc *agentToolsUseCase) Delete(ctx context.Context, agentID, toolID string, by workspace.Requester) error {
	current, err := uc.load(agentID)
	if err != nil {
		return err
	}
	i := indexOf(current.InternalTools, toolID)
	if i < 0 {
		return fmt.Errorf("%w: %s", agenttool.ErrToolNotFound, toolID)
	}
	next := append(cloneBindings(current.InternalTools[:i]), current.InternalTools[i+1:]...)
	_, err = uc.save(ctx, agentID, next, nil, by)
	return err
}

func (uc *agentToolsUseCase) Duplicate(ctx context.Context, agentID, toolID string, by workspace.Requester) (agent.ToolBinding, error) {
	current, err := uc.load(agentID)
	if err != nil {
		return agent.ToolBinding{}, err
	}
	i := indexOf(current.InternalTools, toolID)
	if i < 0 {
		return agent.ToolBinding{}, fmt.Errorf("%w: %s", agenttool.ErrToolNotFound, toolID)
	}
	source := current.InternalTools[i]
	def := definitionsByName(uc.registry)[strings.ToLower(source.Name)]
	switch def.Instances.Effective() {
	case tools.InstancesSingle:
		return agent.ToolBinding{}, fmt.Errorf("%w: %s", agenttool.ErrSingleTypeTaken, source.Name)
	case tools.InstancesPerTarget:
		return agent.ToolBinding{}, fmt.Errorf("%w: %s", agenttool.ErrTargetTaken, source.Name)
	}
	taken := map[string]bool{}
	for _, b := range current.InternalTools {
		taken[strings.ToLower(b.Function)] = true
	}
	basis := source.Function
	if basis == "" {
		basis = source.Name
	}
	copied := agent.ToolBinding{
		Name: source.Name, Description: source.Description, Config: copyConfig(source.Config), Visibility: source.Visibility,
		Function: agenttool.ProposeName(basis, func(n string) bool { return taken[n] }),
	}
	saved, err := uc.save(ctx, agentID, append(cloneBindings(current.InternalTools), copied), nil, by)
	if err != nil {
		return agent.ToolBinding{}, err
	}
	return added(current.InternalTools, saved)
}

func (uc *agentToolsUseCase) Reorder(ctx context.Context, agentID string, ids []string, by workspace.Requester) ([]agent.ToolBinding, error) {
	current, err := uc.load(agentID)
	if err != nil {
		return nil, err
	}
	if len(ids) != len(current.InternalTools) {
		return nil, agenttool.ErrOrderMismatch
	}
	next := make([]agent.ToolBinding, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		i := indexOf(current.InternalTools, id)
		if i < 0 || seen[id] {
			return nil, agenttool.ErrOrderMismatch
		}
		seen[id] = true
		next = append(next, current.InternalTools[i])
	}
	return uc.save(ctx, agentID, next, nil, by)
}

func (uc *agentToolsUseCase) Revisions(ctx context.Context, agentID, toolID string) ([]agent.ToolRevision, error) {
	if uc.revisions == nil {
		return nil, agenttool.ErrValidationOffline
	}
	return uc.revisions.ToolRevisions(ctx, agentID, toolID)
}

func (uc *agentToolsUseCase) Revision(ctx context.Context, agentID, toolID string, revision int) (agent.ToolRevision, error) {
	if uc.revisions == nil {
		return agent.ToolRevision{}, agenttool.ErrValidationOffline
	}
	return uc.revisions.ToolRevision(ctx, agentID, toolID, revision)
}

func (uc *agentToolsUseCase) load(agentID string) (*agent.Agent, error) {
	current, err := uc.agents.FindByID(agentID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, agent.ErrAgentNotFound
	}
	return current, nil
}

func (uc *agentToolsUseCase) save(ctx context.Context, agentID string, next []agent.ToolBinding, expected map[string]int, by workspace.Requester) ([]agent.ToolBinding, error) {
	if next == nil {
		next = []agent.ToolBinding{}
	}
	updated, err := uc.update.Execute(ctx, agentID, agent.UpdateAgentInput{InternalTools: next, ExpectedTools: expected, RequestedBy: by})
	if err != nil {
		return nil, err
	}
	return updated.InternalTools, nil
}

func cloneBindings(in []agent.ToolBinding) []agent.ToolBinding {
	return append([]agent.ToolBinding(nil), in...)
}

func indexOf(bindings []agent.ToolBinding, id string) int {
	for i, b := range bindings {
		if b.ID == id && id != "" {
			return i
		}
	}
	return -1
}

func find(bindings []agent.ToolBinding, id string) (agent.ToolBinding, error) {
	if i := indexOf(bindings, id); i >= 0 {
		return bindings[i], nil
	}
	return agent.ToolBinding{}, fmt.Errorf("%w: %s", agenttool.ErrToolNotFound, id)
}

func added(before, after []agent.ToolBinding) (agent.ToolBinding, error) {
	for _, b := range after {
		if indexOf(before, b.ID) < 0 {
			return b, nil
		}
	}
	return agent.ToolBinding{}, agenttool.ErrToolNotFound
}

func copyConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}
	out := make(map[string]interface{}, len(config))
	for k, v := range config {
		out[k] = v
	}
	return out
}

func (uc *agentToolsUseCase) ValidateToolsWith(validator ToolValidator, published agent.PublishedReader) {
	uc.validator = validator
	uc.published = published
}

func (uc *agentToolsUseCase) Validate(ctx context.Context, agentID string, draft []agent.ToolBinding) (agenttool.Report, error) {
	current, err := uc.load(agentID)
	if err != nil {
		return agenttool.Report{}, err
	}
	stored := cloneBindings(current.InternalTools)
	if draft != nil {
		prepared, err := agenttool.PrepareSet(setRulesFor(uc.registry, definitionsByName(uc.registry)), stored, draft)
		if err != nil {
			return agenttool.Report{}, err
		}
		current.InternalTools = prepared
	}
	if uc.validator == nil {
		return agenttool.Unavailable("a conferência das ferramentas não está ligada"), nil
	}
	return uc.validator.Validate(ctx, current, publishedToolsOr(uc.published, agentID, stored)), nil
}

var _ agenttool.ToolsValidation = (*agentToolsUseCase)(nil)

func publishedToolsOr(published agent.PublishedReader, agentID string, fallback []agent.ToolBinding) []agent.ToolBinding {
	if published == nil {
		return fallback
	}
	live, err := published.FindPublished(agentID)
	if err != nil || live == nil {
		return fallback
	}
	return live.InternalTools
}

func (uc *agentToolsUseCase) SaveTestInputs(ctx context.Context, agentID, toolID string, inputs []testinput.TestInput) ([]testinput.TestInput, error) {
	current, err := uc.load(agentID)
	if err != nil {
		return nil, err
	}
	if indexOf(current.InternalTools, toolID) < 0 {
		return nil, fmt.Errorf("%w: %s", agenttool.ErrToolNotFound, toolID)
	}
	if inputs == nil {
		inputs = []testinput.TestInput{}
	}
	if err := testinput.Validate(inputs); err != nil {
		return nil, err
	}
	store, ok := uc.agents.(agent.ToolTestInputStore)
	if !ok {
		return nil, agenttool.ErrValidationOffline
	}
	if err := store.SaveToolTestInputs(ctx, agentID, toolID, inputs); err != nil {
		return nil, err
	}
	return inputs, nil
}
