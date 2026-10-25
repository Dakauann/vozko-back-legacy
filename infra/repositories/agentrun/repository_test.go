package agentrun_repository

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"vozko/domain/agentrun"
	"vozko/domain/ai"
	aa "vozko/domain/ai_attendance"
	"vozko/domain/conversation"
)

func mockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true, WithoutReturning: true}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

func TestARunAndItsSpansAreWrittenInOneTransaction(t *testing.T) {
	db, mock := mockDB(t)
	at := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	cost := int64(700)
	run := &agentrun.Run{
		ID: "6f1c2a52-0b5e-4bb4-9d0f-7d1a3a9c2b10", WorkspaceID: "ws-1", AgentID: "ag-1", AgentVersion: 3, EntryID: "simulacao", Source: agentrun.SourceConversation,
		Model: "m", Status: agentrun.StatusOK, CostMicros: &cost, ModelCalls: []ai.ModelCall{{Index: 0, Model: "m"}}, StartedAt: at,
	}
	spans := []aa.ToolCall{{ID: "c-1", WorkspaceID: "ws-1", AgentID: "ag-1", ToolName: "consultar", Outcome: aa.ToolCallOK, AgentRunID: run.ID, CreatedAt: at}}
	mock.ExpectBegin()
	mock.ExpectExec(`^INSERT INTO "agent_runs"`).WithArgs(
		run.ID, "ws-1", "ag-1", 3, nil, "", "conversation", "m", "ok", "", "", int64(0), 0, 0, 0, 0, &cost, 0, 0, 0, `[{"index":0,"model":"m","latencyMs":0,"inputTokens":0,"outputTokens":0,"cachedTokens":0,"reasoningTokens":0,"costMicros":0}]`, `[]`, "", "", at,
	).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`^INSERT INTO "ai_tool_calls"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := NewRepository(db).Record(run, spans); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := NewRepository(db).Record(nil, nil); err != nil {
		t.Fatalf("nothing to record writes nothing: %v", err)
	}
}

func TestTheListAppliesEveryFilterAndTheConversationRuleWithMatchingPlaceholders(t *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	filter := agentrun.Filter{From: from, To: from.Add(time.Hour), Source: agentrun.SourceConversation, Status: agentrun.StatusToolFailed, ToolInstanceID: "tool-1", ErrorKind: "timeout", EntryID: "6f1c2a52-0b5e-4bb4-9d0f-7d1a3a9c2b10", HandledFailure: true}
	scope := conversation.VisibilityScope{ReadScope: conversation.ReadScope{DepartmentIDs: []string{"d-1"}, RestrictDepartments: true}}
	where, args := listConditions("ws-1", "ag-1", filter, scope)
	if strings.Count(where, "?") != len(args) {
		t.Fatalf("placeholders %d args %d in %s", strings.Count(where, "?"), len(args), where)
	}
	for _, want := range []string{"agent_runs.started_at >= ?", "agent_runs.started_at <= ?", "agent_runs.source = ?", "agent_runs.status = ?", "agent_runs.entry_id = ?", "agent_runs.handled_failures > 0", "c.tool_instance_id = ?", "c.error_kind = ?", "agent_runs.entry_id IS NULL"} {
		if !strings.Contains(where, want) {
			t.Fatalf("missing %q in %s", want, where)
		}
	}
	open, openArgs := listConditions("ws-1", "ag-1", agentrun.Filter{ErrorKind: agentrun.ErrorKindModel}, conversation.VisibilityScope{})
	if strings.Contains(open, "EXISTS") || strings.Contains(open, "IS NULL") || !strings.Contains(open, "agent_runs.error_kind = ?") || len(openArgs) != 3 {
		t.Fatalf("an unrestricted model error filter: %s %v", open, openArgs)
	}
}

func TestRetentionStatementsAreBoundedBatches(t *testing.T) {
	for _, statement := range []string{pruneSpanPayloadsSQL, pruneRunTextsSQL, deleteRunsBeforeSQL} {
		if strings.Count(statement, "?") != 2 || !strings.Contains(statement, "LIMIT ?") {
			t.Fatalf("not a bounded batch: %s", statement)
		}
	}
	db, mock := mockDB(t)
	cutoff := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE ai_tool_calls SET args = NULL`)).WithArgs(cutoff, 5000).WillReturnResult(sqlmock.NewResult(0, 7))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE agent_runs SET user_message = ''`)).WithArgs(cutoff, 5000).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM agent_runs`)).WithArgs(cutoff, 5000).WillReturnResult(sqlmock.NewResult(0, 3))
	repo := NewRepository(db)
	if n, err := repo.PrunePayloads(cutoff, 5000); err != nil || n != 9 {
		t.Fatalf("pruned %d %v", n, err)
	}
	if n, err := repo.DeleteStartedBefore(cutoff, 5000); err != nil || n != 3 {
		t.Fatalf("deleted %d %v", n, err)
	}
}

func TestAnUnknownRunIDIsNotFoundWithoutAQuery(t *testing.T) {
	db, _ := mockDB(t)
	if _, err := NewRepository(db).Find("ws-1", "ag-1", "nope"); err != agentrun.ErrRunNotFound {
		t.Fatalf("err %v", err)
	}
}
