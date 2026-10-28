package googlesheet

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/googlesheet"
)

func chatOrigin(entryID string) googlesheet.Origin {
	return googlesheet.Origin{Kind: googlesheet.OriginAgent, AgentID: "agent-1", EntryID: entryID, EntryType: "whatsapp"}
}

func (w *world) enqueueFrom(t *testing.T, key string, origin googlesheet.Origin, values map[string]any) (*EnqueueResult, error) {
	t.Helper()
	res, err := w.intake.Enqueue(context.Background(), EnqueueInput{
		WorkspaceID: testWorkspace, SheetID: w.sheet.ID, Values: values, IdempotencyKey: key, Origin: origin,
	})
	w.now = w.now.Add(time.Millisecond)
	return res, err
}

func (w *world) setUnique(t *testing.T, rule googlesheet.UniqueRule) *googlesheet.Sheet {
	t.Helper()
	current := w.stored(t)
	sheet, err := w.manager.UpdateDetails(context.Background(), DetailsInput{WorkspaceID: testWorkspace, ID: w.sheet.ID, Version: current.Version, Unique: &rule})
	if err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
	return sheet
}

func TestARepeatedRecordIsNotQueuedAgainAndPointsAtTheFirstRow(t *testing.T) {
	w := newWorld(t)
	w.setUnique(t, googlesheet.UniqueRule{Mode: googlesheet.UniqueConversation})
	first, err := w.enqueueFrom(t, "ai:conv-1:call-1", chatOrigin("conv-1"), map[string]any{"nome": "Ana", "valor": 10})
	if err != nil || first.Repeat != nil || first.Duplicate {
		t.Fatalf("first = %+v, %v", first, err)
	}
	repeat, err := w.enqueueFrom(t, "wf:run-2:node#0", chatOrigin("conv-1"), map[string]any{"nome": "Ana", "valor": 10})
	if err != nil || repeat.Repeat == nil || repeat.Duplicate || repeat.RowID != first.RowID {
		t.Fatalf("repeat = %+v, %v", repeat, err)
	}
	if repeat.Repeat.RowID != first.RowID || repeat.Repeat.Code != googlesheet.CodeAlreadyRecordedConversation || !errors.Is(repeat.Repeat, googlesheet.ErrAlreadyRecorded) {
		t.Fatalf("repeat refusal = %+v", repeat.Repeat)
	}
	replay, err := w.enqueueFrom(t, "ai:conv-1:call-1", chatOrigin("conv-1"), map[string]any{"nome": "Ana", "valor": 10})
	if err != nil || !replay.Duplicate || replay.Repeat != nil || replay.RowID != first.RowID {
		t.Fatalf("an idempotent replay stays a duplicate: %+v, %v", replay, err)
	}
	if len(w.rows.rows) != 1 || len(w.nudger.sheets) != 1 {
		t.Fatalf("rows = %d, nudges = %v", len(w.rows.rows), w.nudger.sheets)
	}
	other, err := w.enqueueFrom(t, "ai:conv-2:call-1", chatOrigin("conv-2"), map[string]any{"nome": "Bia", "valor": 10})
	if err != nil || other.Repeat != nil || other.RowID == first.RowID {
		t.Fatalf("another conversation = %+v, %v", other, err)
	}
}

func TestAConversationRuleRefusesWritesWithoutAConversationBeforeAnythingIsStored(t *testing.T) {
	w := newWorld(t)
	w.setUnique(t, googlesheet.UniqueRule{Mode: googlesheet.UniqueConversation})
	manual := googlesheet.Origin{Kind: googlesheet.OriginManual, ActorID: "user-1"}
	if _, err := w.enqueueFrom(t, "manual:user-1:k", manual, map[string]any{"nome": "Ana"}); !errors.Is(err, googlesheet.ErrUniqueNeedsConversation) {
		t.Fatalf("manual add = %v", err)
	}
	if _, _, err := w.intake.Preview(context.Background(), testWorkspace, w.sheet.ID, map[string]any{"nome": "Ana"}, manual); !errors.Is(err, googlesheet.ErrUniqueNeedsConversation) {
		t.Fatalf("preview = %v", err)
	}
	if len(w.rows.rows) != 0 {
		t.Fatalf("a refused write was stored: %d", len(w.rows.rows))
	}
}

func TestTurningTheRuleOnClaimsTheRowsStillInTheLedger(t *testing.T) {
	w := newWorld(t)
	oldest, err := w.enqueueFrom(t, "k1", chatOrigin("conv-1"), map[string]any{"nome": "Ana", "valor": 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.enqueueFrom(t, "k2", chatOrigin("conv-1"), map[string]any{"nome": "Ana", "valor": 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.enqueueFrom(t, "k3", googlesheet.Origin{Kind: googlesheet.OriginManual, ActorID: "u"}, map[string]any{"nome": "Caio"}); err != nil {
		t.Fatal(err)
	}
	sheet := w.setUnique(t, googlesheet.UniqueRule{Mode: googlesheet.UniqueConversation})
	if sheet.Unique.Mode != googlesheet.UniqueConversation || w.stored(t).Unique.Mode != googlesheet.UniqueConversation {
		t.Fatalf("rule not saved: %+v", sheet.Unique)
	}
	repeat, err := w.enqueueFrom(t, "k4", chatOrigin("conv-1"), map[string]any{"nome": "Ana", "valor": 10})
	if err != nil || repeat.Repeat == nil || repeat.RowID != oldest.RowID {
		t.Fatalf("a record already in the ledger must count: %+v, %v", repeat, err)
	}
	if fresh, err := w.enqueueFrom(t, "k5", chatOrigin("conv-3"), map[string]any{"nome": "Duda"}); err != nil || fresh.Repeat != nil {
		t.Fatalf("a new conversation = %+v, %v", fresh, err)
	}
}

func TestTheRuleIsCheckedAgainstTheSavedColumnsAndAStaleVersionIsRefused(t *testing.T) {
	w := newWorld(t)
	rule := googlesheet.UniqueRule{Mode: googlesheet.UniqueColumns, Columns: []string{"inexistente"}}
	_, err := w.manager.UpdateDetails(context.Background(), DetailsInput{WorkspaceID: testWorkspace, ID: w.sheet.ID, Version: 1, Unique: &rule})
	if googlesheet.ErrorCode(err) != googlesheet.CodeUniqueColumnUnknown {
		t.Fatalf("unknown column = %v", err)
	}
	none := googlesheet.UniqueRule{Mode: googlesheet.UniqueNone}
	if _, err := w.manager.UpdateDetails(context.Background(), DetailsInput{WorkspaceID: testWorkspace, ID: w.sheet.ID, Version: 7, Unique: &none}); !errors.Is(err, googlesheet.ErrStaleSheet) {
		t.Fatalf("stale = %v", err)
	}
	if w.stored(t).Version != 1 {
		t.Fatal("a refused change must not be saved")
	}
}

func TestCreateKeepsTheChosenRule(t *testing.T) {
	w := newWorld(t)
	sheet, err := w.manager.Create(context.Background(), CreateInput{
		WorkspaceID: testWorkspace, UserID: "user-1", FileTitle: "Cobrança", Title: "Pagamentos", TabName: "Pagamentos",
		Columns: []googlesheet.ColumnInput{{Header: "Matrícula", Type: googlesheet.TypeText}, {Header: "Valor", Type: googlesheet.TypeMoney}},
		Unique:  googlesheet.UniqueInput{Mode: googlesheet.UniqueColumns, Columns: []int{0}},
	})
	if err != nil || sheet.Unique.Mode != googlesheet.UniqueColumns || len(sheet.Unique.Columns) != 1 || sheet.Unique.Columns[0] != "matricula" {
		t.Fatalf("created = %+v, %v", sheet, err)
	}
}
