package conversation_repository

import (
	"fmt"

	"github.com/lib/pq"

	"vozko/domain/conversation"
)

const unstagedStageID = "__unstaged__"

type inboxSet struct {
	with     string
	withArgs []interface{}
	rows     string
	rowsArgs []interface{}
	order    inboxOrderColumns
}

func (s inboxSet) stageCountQuery(stageWorkspaceID string, stageIDs []string) (string, []interface{}) {
	sql := s.with + `
		SELECT requested.position, COUNT(matched.entry_id) AS total
		FROM unnest(?::uuid[]) WITH ORDINALITY AS requested(stage_id, position)
		LEFT JOIN LATERAL (
			SELECT DISTINCT es.entry_id
			FROM entry_stages es
			WHERE es.stage_id = requested.stage_id AND es.workspace_id = ? AND es.deleted_at IS NULL
		) staged ON true
		LEFT JOIN (` + s.rows + `) matched ON matched.entry_id = staged.entry_id
		GROUP BY requested.position`
	args := append(append([]interface{}{}, s.withArgs...), pq.Array(stageIDs), stageWorkspaceID)
	return sql, append(args, s.rowsArgs...)
}

type inboxTarget struct {
	workspace   bool
	channel     channelQuery
	filteredIDs []string
}

func inboxTargetOf(input conversation.SearchEntriesInput) (inboxTarget, bool, error) {
	if input.WorkspaceID != "" && input.CampaignID == "" {
		return inboxTarget{workspace: true}, true, nil
	}
	var filteredIDs []string
	if input.CampaignID == "" {
		for _, id := range input.EntryIDs {
			if id != "" {
				filteredIDs = append(filteredIDs, id)
			}
		}
		if len(filteredIDs) == 0 {
			return inboxTarget{}, false, nil
		}
	}
	ch, ok := channelQueryFor(input.EntryType)
	if !ok {
		return inboxTarget{}, false, fmt.Errorf("invalid entry type: %s", input.EntryType)
	}
	return inboxTarget{channel: ch, filteredIDs: filteredIDs}, true, nil
}

func (t inboxTarget) set(input conversation.SearchEntriesInput) (inboxSet, bool) {
	if t.workspace {
		return workspaceInboxSet(input)
	}
	return containerInboxSet(input, t.channel, t.filteredIDs), true
}

func (t inboxTarget) queries(input conversation.SearchEntriesInput) (countedPage, bool) {
	pageSize, offset := inboxPage(input.Page, input.PageSize)
	if t.workspace {
		return workspaceInboxQueries(input, pageSize, offset)
	}
	return containerInboxQueries(input, t.channel, t.filteredIDs, pageSize, offset), true
}

func withoutStageFilter(input conversation.SearchEntriesInput) conversation.SearchEntriesInput {
	input.StageIDs = nil
	return input
}

func (r *repository) CountEntriesByStages(input conversation.SearchEntriesInput, stageIDs []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(stageIDs))
	for _, id := range stageIDs {
		counts[id] = 0
	}
	target, ok, err := inboxTargetOf(input)
	if err != nil {
		return nil, err
	}
	if !ok || len(stageIDs) == 0 {
		return counts, nil
	}
	base := withoutStageFilter(input)

	if input.StageWorkspaceID == "" {
		total, err := r.countInbox(target, base)
		if err != nil {
			return nil, err
		}
		for _, id := range stageIDs {
			counts[id] = total
		}
		return counts, nil
	}

	var grouped []string
	unstaged := false
	for _, id := range stageIDs {
		if id == unstagedStageID {
			unstaged = true
		} else {
			grouped = append(grouped, id)
		}
	}

	if len(grouped) > 0 {
		set, ok := target.set(base)
		if ok {
			sql, args := set.stageCountQuery(input.StageWorkspaceID, grouped)
			var rows []struct {
				Position int64 `gorm:"column:position"`
				Total    int64 `gorm:"column:total"`
			}
			if err := r.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
				return nil, fmt.Errorf("error counting entries by stage: %w", err)
			}
			for _, row := range rows {
				if row.Position < 1 || row.Position > int64(len(grouped)) {
					return nil, fmt.Errorf("error counting entries by stage: position %d outside the %d requested", row.Position, len(grouped))
				}
				counts[grouped[row.Position-1]] = row.Total
			}
		}
	}

	if unstaged {
		per := base
		per.StageIDs = []string{unstagedStageID}
		total, err := r.countInbox(target, per)
		if err != nil {
			return nil, err
		}
		counts[unstagedStageID] = total
	}
	return counts, nil
}

func (r *repository) countInbox(target inboxTarget, input conversation.SearchEntriesInput) (int64, error) {
	q, ok := target.queries(input)
	if !ok {
		return 0, nil
	}
	var total int64
	if err := r.db.Raw(q.countSQL, q.countArgs...).Scan(&total).Error; err != nil {
		return 0, fmt.Errorf("error counting inbox entries: %w", err)
	}
	return total, nil
}
