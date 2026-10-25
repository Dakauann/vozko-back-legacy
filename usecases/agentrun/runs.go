package agentrun_usecase

import (
	"log/slog"

	"vozko/domain/agentrun"
	"vozko/domain/conversation"
	"vozko/domain/shared"
)

type Runs interface {
	List(person shared.Person, workspaceID, agentID string, filter agentrun.Filter, pagination shared.Pagination) (*shared.PaginatedResult[*agentrun.Run], error)
	Get(person shared.Person, workspaceID, agentID, runID string) (*agentrun.Detail, error)
}

type runs struct {
	store      agentrun.Repository
	visibility conversation.Visibility
}

func NewRuns(store agentrun.Repository, visibility conversation.Visibility) Runs {
	if store == nil || visibility == nil {
		panic("agent runs: reading runs needs the run store and the conversation rule")
	}
	return &runs{store: store, visibility: visibility}
}

func (uc *runs) List(person shared.Person, workspaceID, agentID string, filter agentrun.Filter, pagination shared.Pagination) (*shared.PaginatedResult[*agentrun.Run], error) {
	if err := filter.Check(); err != nil {
		return nil, err
	}
	scope, err := uc.visibility.Scope(person, workspaceID)
	if err != nil {
		return nil, err
	}
	return uc.store.List(workspaceID, agentID, filter, scope, pagination)
}

func (uc *runs) Get(person shared.Person, workspaceID, agentID, runID string) (*agentrun.Detail, error) {
	detail, err := uc.store.Find(workspaceID, agentID, runID)
	if err != nil {
		return nil, err
	}
	if !detail.Run.InConversation() {
		return detail, nil
	}
	if err := conversation.RequireVisible(uc.visibility, person, workspaceID, detail.Run.EntryID, detail.Run.EntryType); err != nil {
		if err != conversation.ErrNotVisible {
			slog.Warn("agent_runs.visibility_unreadable", slog.String("workspace_id", workspaceID), slog.String("run_id", runID), slog.String("error", err.Error()))
		}
		return nil, agentrun.ErrRunNotFound
	}
	return detail, nil
}
