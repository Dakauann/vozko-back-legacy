package workflow_repository

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"vozko/domain/shared"
	"vozko/domain/workflow"
	"vozko/infra/database/schema"
)

func TestSentPromptsReadOnePageOfSentMenusAfterTheCursor(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(`FROM workflow_run_logs l JOIN workflow_runs r ON r.id = l.run_id`).
		WithArgs("unofficial_whatsapp", "action_send_interactive", "action_send_whatsapp_button", workflow.SentPromptCursorStart, 50).
		WillReturnRows(sqlmock.NewRows([]string{"step_id", "workflow_id", "entry_id", "entry_type", "provider_message_id", "config"}).
			AddRow("step-1", "wf-1", "conv-1", "unofficial_whatsapp", "3EB0MENU", `{"body":"MENU INICIAL"}`))

	prompts, err := NewSentPromptRepository(db).SentPrompts(context.Background(), shared.EntryTypeUnofficialWhatsApp,
		workflow.InteractivePromptNodeTypes(), workflow.SentPromptCursorStart, 50)
	if err != nil {
		t.Fatal(err)
	}
	want := []workflow.SentPrompt{{
		StepID: "step-1", WorkflowID: "wf-1", EntryID: "conv-1", EntryType: shared.EntryTypeUnofficialWhatsApp,
		ProviderMessageID: "3EB0MENU", Config: map[string]interface{}{"body": "MENU INICIAL"},
	}}
	if !reflect.DeepEqual(prompts, want) {
		t.Fatalf("prompts %+v, want %+v", prompts, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSentPromptsComeFromSentMenusOfThatChannelAgainstPostgres(t *testing.T) {
	tx := postgresSchemaTx(t, &schema.WorkflowRunSchema{}, &schema.WorkflowRunLogSchema{})
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	run := func(entryType string) string {
		id := uuid.NewString()
		row := schema.WorkflowRunSchema{ID: id, WorkflowID: uuid.NewString(), WorkspaceID: uuid.NewString(), EntryID: uuid.NewString(), EntryType: entryType, State: "{}", StartedAt: at}
		if err := tx.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	step := func(runID, nodeType string, input, output *string) string {
		id := uuid.NewString()
		row := schema.WorkflowRunLogSchema{ID: id, RunID: runID, NodeID: "ask", NodeType: nodeType, Status: "success", Input: input, Output: output, ExecutedAt: at}
		if err := tx.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	text := func(s string) *string { return &s }
	menuInput, sent := text(`{"interactive_type":"list","body":"MENU INICIAL"}`), text(`{"message_id":"3EB0MENU","sent":true}`)

	unofficial := run("unofficial_whatsapp")
	wanted := step(unofficial, "action_send_interactive", menuInput, sent)
	step(unofficial, "action_send_interactive", nil, sent)
	step(unofficial, "action_send_interactive", menuInput, text(`{"message_id":"","sent":false}`))
	step(unofficial, "action_send_text", menuInput, sent)
	step(run("instagram"), "action_send_interactive", menuInput, sent)

	prompts, err := NewSentPromptRepository(tx).SentPrompts(context.Background(), shared.EntryTypeUnofficialWhatsApp,
		workflow.InteractivePromptNodeTypes(), workflow.SentPromptCursorStart, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 1 || prompts[0].StepID != wanted || prompts[0].ProviderMessageID != "3EB0MENU" || prompts[0].Config["body"] != "MENU INICIAL" {
		t.Fatalf("prompts %+v, want only the sent menu step %s", prompts, wanted)
	}
	after, err := NewSentPromptRepository(tx).SentPrompts(context.Background(), shared.EntryTypeUnofficialWhatsApp,
		workflow.InteractivePromptNodeTypes(), wanted, 10)
	if err != nil || len(after) != 0 {
		t.Fatalf("after the cursor got %+v, %v; want nothing", after, err)
	}
}
