package leadsentiment

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	ca "vozko/domain/audience"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
	infracrmfilter "vozko/infra/repositories/crmfilter"
)

const (
	MaxLeadsPerRefresh = 1000
	workspaceLockClass = 74_150
)

var table = schema.LeadSentiment{}.TableName()

var ruleSQL = `WITH targets AS (%s),
picked AS (
	SELECT t.lead_id, pick.id AS analysis_id, pick.sentiment
	FROM targets t
	JOIN LATERAL (
		SELECT newest.id, newest.sentiment
		FROM ` + infracrmfilter.LeadEntriesSource() + `
		JOIN LATERAL (
			SELECT aa.id, aa.sentiment, aa.occurred_at, aa.created_at
			FROM audience_analyses aa
			WHERE aa.workspace_id = ? AND aa.source = lead_entries.entry_type AND aa.subject_id = lead_entries.entry_id::text
				AND aa.subject_kind = ? AND aa.deleted_at IS NULL AND aa.status = ?
			ORDER BY aa.occurred_at DESC, aa.created_at DESC, aa.id DESC
			LIMIT 1
		) newest ON true
		WHERE lead_entries.lead_id = t.lead_id
		ORDER BY newest.occurred_at DESC, newest.created_at DESC, newest.id DESC
		LIMIT 1
	) pick ON true
),
cleared AS (
	DELETE FROM ` + table + ` ls
	WHERE ls.lead_id IN (SELECT lead_id FROM targets)
		AND NOT EXISTS (SELECT 1 FROM picked p WHERE p.lead_id = ls.lead_id AND p.sentiment = ANY(?::text[]))
)
INSERT INTO ` + table + ` (lead_id, workspace_id, sentiment, analysis_id, refreshed_at)
SELECT p.lead_id, ?, p.sentiment, p.analysis_id, now() FROM picked p WHERE p.sentiment = ANY(?::text[])
ON CONFLICT (lead_id) DO UPDATE SET sentiment = EXCLUDED.sentiment, analysis_id = EXCLUDED.analysis_id, refreshed_at = EXCLUDED.refreshed_at
WHERE ` + table + `.sentiment IS DISTINCT FROM EXCLUDED.sentiment OR ` + table + `.analysis_id IS DISTINCT FROM EXCLUDED.analysis_id`

const leadsOfWorkspace = "SELECT l.id AS lead_id FROM leads l WHERE l.workspace_id = ? AND l.id IN (%s)"

func RefreshLeads(db *gorm.DB, workspaceID string, leadIDs []string) error {
	ids := validIDs(leadIDs)
	for start := 0; start < len(ids); start += MaxLeadsPerRefresh {
		chunk := ids[start:min(start+MaxLeadsPerRefresh, len(ids))]
		if err := refresh(db, workspaceID, "SELECT unnest(?::uuid[])", []interface{}{pq.Array(chunk)}); err != nil {
			return err
		}
	}
	return nil
}

func RefreshReferencing(db *gorm.DB, workspaceID string, analysisIDs []string) error {
	ids := validIDs(analysisIDs)
	if len(ids) == 0 {
		return nil
	}
	return refresh(db, workspaceID, "SELECT ls_r.lead_id FROM "+table+" ls_r WHERE ls_r.workspace_id = ? AND ls_r.analysis_id = ANY(?::uuid[])",
		[]interface{}{workspaceID, pq.Array(ids)})
}

type conversations struct {
	analyses []string
	entries  map[shared.EntryType][]string
}

func RefreshForAnalyses(db *gorm.DB, rows []*ca.Analysis) error {
	byWorkspace := map[string]*conversations{}
	for _, a := range rows {
		if a == nil || a.SubjectKind != ca.SubjectKindConversation || strings.TrimSpace(a.WorkspaceID) == "" {
			continue
		}
		c, ok := byWorkspace[a.WorkspaceID]
		if !ok {
			c = &conversations{entries: map[shared.EntryType][]string{}}
			byWorkspace[a.WorkspaceID] = c
		}
		c.analyses = append(c.analyses, a.ID)
		c.entries[a.Source.EntryType()] = append(c.entries[a.Source.EntryType()], a.SubjectID)
	}
	for _, ws := range sortedKeys(byWorkspace) {
		targets, args := byWorkspace[ws].targets(ws)
		if targets == "" {
			continue
		}
		if err := refresh(db, ws, targets, args); err != nil {
			return err
		}
	}
	return nil
}

func (c *conversations) targets(workspaceID string) (string, []interface{}) {
	var arms []string
	var args []interface{}
	types := make([]string, 0, len(c.entries))
	for e := range c.entries {
		types = append(types, string(e))
	}
	sort.Strings(types)
	for _, e := range types {
		ids := validIDs(c.entries[shared.EntryType(e)])
		if len(ids) == 0 {
			continue
		}
		arms = append(arms, "SELECT lead_entries.lead_id FROM "+infracrmfilter.LeadEntriesSource()+" WHERE lead_entries.entry_type = ? AND lead_entries.entry_id = ANY(?::uuid[])")
		args = append(args, e, pq.Array(ids))
	}
	if ids := validIDs(c.analyses); len(ids) > 0 {
		arms = append(arms, "SELECT ls_r.lead_id FROM "+table+" ls_r WHERE ls_r.workspace_id = ? AND ls_r.analysis_id = ANY(?::uuid[])")
		args = append(args, workspaceID, pq.Array(ids))
	}
	return strings.Join(arms, " UNION "), args
}

func Relink(db *gorm.DB, model interface{}, id string, update func(tx *gorm.DB) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		before, err := linkOf(tx.Clauses(clause.Locking{Strength: "UPDATE"}), model, id)
		if err != nil {
			return err
		}
		if err := update(tx); err != nil {
			return err
		}
		after, err := linkOf(tx, model, id)
		if err != nil {
			return err
		}
		if before.lead() == after.lead() {
			return nil
		}
		ws := after.WorkspaceID
		if ws == "" {
			ws = before.WorkspaceID
		}
		return RefreshLeads(tx, ws, []string{before.lead(), after.lead()})
	})
}

type link struct {
	WorkspaceID string
	LeadID      *string
}

func (l link) lead() string {
	if l.LeadID == nil {
		return ""
	}
	return *l.LeadID
}

func linkOf(tx *gorm.DB, model interface{}, id string) (link, error) {
	var rows []link
	if err := tx.Model(model).Select("workspace_id::text AS workspace_id, lead_id::text AS lead_id").Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return link{}, fmt.Errorf("lead sentiment: reading the lead of %s: %w", id, err)
	}
	if len(rows) == 0 {
		return link{}, nil
	}
	return rows[0], nil
}

func refresh(db *gorm.DB, workspaceID, targets string, targetArgs []interface{}) error {
	if _, err := uuid.Parse(workspaceID); err != nil {
		return fmt.Errorf("lead sentiment: the workspace %q is not an id", workspaceID)
	}
	sql, args := statement(workspaceID, targets, targetArgs)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL jit = off").Error; err != nil {
			return fmt.Errorf("lead sentiment: session: %w", err)
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?, hashtext(?))", workspaceLockClass, workspaceID).Error; err != nil {
			return fmt.Errorf("lead sentiment: locking the workspace: %w", err)
		}
		if err := tx.Exec(sql, args...).Error; err != nil {
			return fmt.Errorf("lead sentiment: refreshing: %w", err)
		}
		return nil
	})
}

func statement(workspaceID, targets string, targetArgs []interface{}) (string, []interface{}) {
	valid := pq.Array(shared.SentimentValues())
	sql := fmt.Sprintf(ruleSQL, fmt.Sprintf(leadsOfWorkspace, targets))
	args := append(append([]interface{}{workspaceID}, targetArgs...),
		workspaceID, string(ca.SubjectKindConversation), string(ca.StatusAnalyzed), valid, workspaceID, valid)
	return sql, args
}

func validIDs(raw []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		id, err := uuid.Parse(strings.TrimSpace(v))
		if err != nil || seen[id.String()] {
			continue
		}
		seen[id.String()] = true
		out = append(out, id.String())
	}
	sort.Strings(out)
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func RefreshLeadsReadingEntries(db *gorm.DB, entryType shared.EntryType, entriesSQL string, args ...interface{}) error {
	var reading []struct{ WorkspaceID, LeadID string }
	sql := "SELECT ls_e.workspace_id::text AS workspace_id, ls_e.lead_id::text AS lead_id FROM " + table + " ls_e" +
		" JOIN audience_analyses aa_e ON aa_e.id = ls_e.analysis_id" +
		" WHERE aa_e.source = ? AND aa_e.subject_kind = ? AND aa_e.subject_id IN (SELECT entries.id::text FROM (" + entriesSQL + ") entries)"
	if err := db.Raw(sql, append([]interface{}{string(entryType), string(ca.SubjectKindConversation)}, args...)...).Scan(&reading).Error; err != nil {
		return fmt.Errorf("lead sentiment: the leads reading the removed conversations: %w", err)
	}
	byWorkspace := map[string][]string{}
	for _, r := range reading {
		byWorkspace[r.WorkspaceID] = append(byWorkspace[r.WorkspaceID], r.LeadID)
	}
	for _, ws := range sortedKeys(byWorkspace) {
		if err := RefreshLeads(db, ws, byWorkspace[ws]); err != nil {
			return err
		}
	}
	return nil
}

func RefreshChunk(ctx context.Context, db *gorm.DB, workspaceID, after string, size int) (string, int, error) {
	size = max(1, min(size, MaxLeadsPerRefresh))
	var ids []string
	q := db.WithContext(ctx).Table("leads").Select("id::text").Where("workspace_id = ?", workspaceID)
	if after != "" {
		q = q.Where("id > ?", after)
	}
	if err := q.Order("id").Limit(size).Pluck("id", &ids).Error; err != nil {
		return "", 0, fmt.Errorf("lead sentiment: the leads after %q: %w", after, err)
	}
	if len(ids) == 0 {
		return "", 0, nil
	}
	if err := RefreshLeads(db.WithContext(ctx), workspaceID, ids); err != nil {
		return "", 0, err
	}
	if len(ids) < size {
		return "", len(ids), nil
	}
	return ids[len(ids)-1], len(ids), nil
}
