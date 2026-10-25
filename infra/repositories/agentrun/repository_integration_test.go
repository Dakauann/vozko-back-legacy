package agentrun_repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/agentrun"
	aa "vozko/domain/ai_attendance"
	"vozko/domain/calltrace"
	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

type runFixture struct {
	t         *testing.T
	db        *gorm.DB
	workspace string
	agent     string
}

func newRunFixture(t *testing.T) *runFixture {
	models := append(repotest.ConversationVisibilityModels(), &schema.AgentRunSchema{}, &schema.AIToolCallSchema{})
	db := repotest.IsolatedDB(t, "agent_runs", models...)
	repotest.DropForeignKeys(t, db)
	return &runFixture{t: t, db: db, workspace: uuid.NewString(), agent: uuid.NewString()}
}

func (f *runFixture) whatsappEntry(departmentID string) string {
	f.t.Helper()
	campaign, entry := uuid.NewString(), uuid.NewString()
	for _, stmt := range []struct {
		sql  string
		args []interface{}
	}{
		{`INSERT INTO whatsapp_campaigns (id, workspace_id, department_id, name) VALUES (?, ?, ?, 'Campanha')`, []interface{}{campaign, f.workspace, departmentID}},
		{`INSERT INTO whatsapp_campaign_entries (id, campaign_id, lead_id) VALUES (?, ?, ?)`, []interface{}{entry, campaign, uuid.NewString()}},
	} {
		if err := f.db.Exec(stmt.sql, stmt.args...).Error; err != nil {
			f.t.Fatalf("%s: %v", stmt.sql, err)
		}
	}
	return entry
}

func (f *runFixture) record(entryID string, source agentrun.Source, at time.Time) string {
	f.t.Helper()
	run := &agentrun.Run{ID: uuid.NewString(), WorkspaceID: f.workspace, AgentID: f.agent, EntryID: entryID, Source: source, Model: "m", Status: agentrun.StatusToolFailed, ToolCalls: 1, FailedToolCalls: 1, UserMessage: "Cadê o pedido 42?", Reply: "Já foi enviado.", StartedAt: at}
	if entryID != "" {
		run.EntryType = "whatsapp"
	}
	span := aa.ToolCall{
		ID: uuid.NewString(), WorkspaceID: f.workspace, AgentID: f.agent, EntryID: entryID, EntryType: run.EntryType, ToolName: "consultar_pedido",
		Args: map[string]interface{}{"numero": "42"}, Outcome: aa.ToolCallError, AgentRunID: run.ID, Source: string(source), ToolInstanceID: "tool-1",
		ErrorKind: "timeout", ErrorChain: []string{"deadline"}, HTTP: []calltrace.HTTPExchange{{Method: "GET", Status: 504}}, ModelResult: `{"is_error":true}`, CreatedAt: at,
	}
	if err := NewRepository(f.db).Record(run, []aa.ToolCall{span}); err != nil {
		f.t.Fatalf("record: %v", err)
	}
	return run.ID
}

func TestRunsAreListedOnlyForConversationsTheViewerCanSeeOnPostgres(t *testing.T) {
	f := newRunFixture(t)
	sales, support := uuid.NewString(), uuid.NewString()
	at := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	inSales := f.record(f.whatsappEntry(sales), agentrun.SourceConversation, at)
	f.record(f.whatsappEntry(support), agentrun.SourceConversation, at.Add(time.Minute))
	simulated := f.record("", agentrun.SourceSimulator, at.Add(2*time.Minute))
	repo := NewRepository(f.db)

	scope := conversation.VisibilityScope{ReadScope: conversation.ReadScope{DepartmentIDs: []string{sales}, RestrictDepartments: true}}
	page, err := repo.List(f.workspace, f.agent, agentrun.Filter{}, scope, shared.Pagination{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalItems != 2 || len(page.Items) != 2 || page.Items[0].ID != simulated || page.Items[1].ID != inSales {
		t.Fatalf("visible runs %+v", page.Items)
	}
	all, err := repo.List(f.workspace, f.agent, agentrun.Filter{ToolInstanceID: "tool-1", ErrorKind: "timeout", Source: agentrun.SourceConversation}, conversation.VisibilityScope{}, shared.Pagination{Page: 1, PageSize: 20})
	if err != nil || all.TotalItems != 2 {
		t.Fatalf("filtered %+v %v", all, err)
	}
	detail, err := repo.Find(f.workspace, f.agent, inSales)
	if err != nil || len(detail.Spans) != 1 || detail.Spans[0].HTTP[0].Status != 504 || detail.Spans[0].ModelResult == "" || detail.Run.UserMessage != "Cadê o pedido 42?" || detail.Run.Reply != "Já foi enviado." {
		t.Fatalf("detail %+v %v", detail, err)
	}
	if _, err := repo.Find(uuid.NewString(), f.agent, inSales); err != agentrun.ErrRunNotFound {
		t.Fatalf("another workspace reads the run: %v", err)
	}
}

func TestRetentionPrunesPayloadsAtThirtyDaysAndRowsAtOneHundredEightyOnPostgres(t *testing.T) {
	f := newRunFixture(t)
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	ancient := f.record("", agentrun.SourceSimulator, now.Add(-200*24*time.Hour))
	old := f.record("", agentrun.SourceSimulator, now.Add(-40*24*time.Hour))
	recent := f.record("", agentrun.SourceSimulator, now.Add(-time.Hour))
	repo := NewRepository(f.db)

	if _, err := repo.PrunePayloads(now.Add(-agentrun.PayloadRetention), 5000); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DeleteStartedBefore(now.Add(-agentrun.RowRetention), 5000); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Find(f.workspace, f.agent, ancient); err != agentrun.ErrRunNotFound {
		t.Fatalf("a run older than 180 days survived: %v", err)
	}
	pruned, err := repo.Find(f.workspace, f.agent, old)
	if err != nil || len(pruned.Spans) != 1 || pruned.Spans[0].HTTP != nil || pruned.Spans[0].ModelResult != "" || pruned.Spans[0].Args != nil || pruned.Spans[0].ErrorKind != "timeout" || pruned.Run.UserMessage != "" || pruned.Run.Reply != "" {
		t.Fatalf("payloads older than 30 days must go and the outline stay: %+v %v", pruned, err)
	}
	kept, err := repo.Find(f.workspace, f.agent, recent)
	if err != nil || kept.Spans[0].HTTP == nil {
		t.Fatalf("recent payloads must stay: %+v %v", kept, err)
	}
}
