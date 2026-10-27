package googlesheet

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

const anchorRowID = "2a4c9b0e-7d3f-4c1e-8b6a-1f2e3d4c5b6a"

func timelineColumns() []googlesheet.ColumnInput {
	return []googlesheet.ColumnInput{
		{Header: "Quando", Type: googlesheet.TypeDateTime, Source: googlesheet.SourceMessageSentAt},
		{Header: "Mensagem", Type: googlesheet.TypeLongText, Source: googlesheet.SourceMessageText},
	}
}

func paymentColumns(target string) []googlesheet.ColumnInput {
	return []googlesheet.ColumnInput{
		{Header: "Valor", Type: googlesheet.TypeMoney, Required: true},
		{Header: "Baixa", Type: googlesheet.TypeChoice, Options: []string{"SIM", "NÃO"}, Colors: map[string]googlesheet.OptionColor{"SIM": googlesheet.ColorGreen}},
		{Header: "Negociação", Type: googlesheet.TypeLink, Source: googlesheet.SourceExportLink, LinkSheetID: target},
	}
}

func (w *world) storeSheet(t *testing.T, in googlesheet.NewSheetInput, spreadsheetID string, tabID int64) *googlesheet.Sheet {
	t.Helper()
	in.WorkspaceID, in.AccountID = testWorkspace, "acct-1"
	s, err := googlesheet.NewSheet(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.OpenPeriod(w.now); err != nil {
		t.Fatal(err)
	}
	s.SpreadsheetID, s.TabID = spreadsheetID, tabID
	if err := w.sheets.Create(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (w *world) storeTimeline(t *testing.T) *googlesheet.Sheet {
	t.Helper()
	timeline := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Negociações", Columns: timelineColumns()}, "sp-1", 31)
	w.api.workbook.Tabs = append(w.api.workbook.Tabs, googlesheet.TabInfo{ID: 31, Title: "Negociações", Rows: 100, Columns: 3})
	return timeline
}

func TestCreateAddsATabToASpreadsheetTheWorkspaceAlreadyHas(t *testing.T) {
	w := newWorld(t)
	created, err := w.manager.Create(context.Background(), CreateInput{
		WorkspaceID: testWorkspace, SpreadsheetID: "sp-1", Title: "Pagamentos",
		Columns: []googlesheet.ColumnInput{{Header: "Valor", Type: googlesheet.TypeMoney}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(w.api.added) != 1 || w.api.added[0].Title != "Pagamentos" || w.api.added[0].ID == 0 {
		t.Fatalf("added = %+v", w.api.added)
	}
	if created.SpreadsheetID != "sp-1" || created.TabID != w.api.added[0].ID || created.Style != googlesheet.DefaultStyle {
		t.Fatalf("created = %+v", created)
	}
	if len(w.api.trashed) != 0 {
		t.Fatal("a tab create touched the file")
	}
}

func TestCreateInAnExistingSpreadsheetRefusesWhatItCannotDo(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*world) CreateInput
		want  error
	}{
		{"unknown file", func(w *world) CreateInput {
			return CreateInput{WorkspaceID: testWorkspace, SpreadsheetID: "sp-unknown", Title: "X", Columns: []googlesheet.ColumnInput{{Header: "A", Type: googlesheet.TypeText}}}
		}, googlesheet.ErrSpreadsheetNotFound},
		{"file of another account", func(w *world) CreateInput {
			w.access.access.AccountID = "acct-2"
			return CreateInput{WorkspaceID: testWorkspace, SpreadsheetID: "sp-1", Title: "X", Columns: []googlesheet.ColumnInput{{Header: "A", Type: googlesheet.TypeText}}}
		}, googlesheet.ErrAccountChanged},
		{"title taken", func(w *world) CreateInput {
			return CreateInput{WorkspaceID: testWorkspace, SpreadsheetID: "sp-1", Title: "pedidos", Columns: []googlesheet.ColumnInput{{Header: "A", Type: googlesheet.TypeText}}}
		}, googlesheet.ErrTabNameTaken},
		{"full file", func(w *world) CreateInput {
			w.api.workbook.Tabs[0].Rows = 960_000
			return CreateInput{WorkspaceID: testWorkspace, SpreadsheetID: "sp-1", Title: "X", Columns: []googlesheet.ColumnInput{{Header: "A", Type: googlesheet.TypeText}}}
		}, googlesheet.ErrSheetNotActive},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld(t)
			_, err := w.manager.Create(context.Background(), c.setup(w))
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if len(w.api.added) != 0 {
				t.Fatal("a refused create added a tab")
			}
		})
	}
}

func TestCreateDeletesOnlyTheTabItMadeWhenItCannotBeRecorded(t *testing.T) {
	w := newWorld(t)
	w.sheets.fail = errors.New("db down")
	_, err := w.manager.Create(context.Background(), CreateInput{
		WorkspaceID: testWorkspace, SpreadsheetID: "sp-1", Title: "Pagamentos", Columns: []googlesheet.ColumnInput{{Header: "Valor", Type: googlesheet.TypeMoney}},
	})
	if err == nil || len(w.api.deleted) != 1 || w.api.deleted[0] != w.api.added[0].ID || len(w.api.trashed) != 0 {
		t.Fatalf("err %v, deleted %v, trashed %v", err, w.api.deleted, w.api.trashed)
	}
}

func TestAPeriodicSheetStartsOnTheTabOfTheCurrentPeriod(t *testing.T) {
	w := newWorld(t)
	w.api.created = googlesheet.CreatedSpreadsheet{SpreadsheetID: "sp-new", TabID: 0}
	created, err := w.manager.Create(context.Background(), CreateInput{
		WorkspaceID: testWorkspace, Title: "Pagamentos", TabPeriod: googlesheet.PeriodMonth, TabPattern: "Pagamentos · {mes} {ano}",
		Columns: []googlesheet.ColumnInput{{Header: "Valor", Type: googlesheet.TypeMoney}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.TabName != "Pagamentos · outubro 2026" || created.TabPeriodKey != "2026-10" {
		t.Fatalf("created tab %q period %q", created.TabName, created.TabPeriodKey)
	}
	if tab, ok, _ := w.sheets.FindTab(context.Background(), created.ID, "2026-10"); !ok || tab.Title != created.TabName {
		t.Fatalf("first period tab not recorded: %+v %v", tab, ok)
	}
}

func TestALinkColumnMustPointAtATimelineInTheSameFile(t *testing.T) {
	w := newWorld(t)
	timeline := w.storeTimeline(t)
	elsewhere := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Outra", Columns: timelineColumns()}, "sp-9", 0)
	ctx := context.Background()
	if _, err := w.manager.Create(ctx, CreateInput{WorkspaceID: testWorkspace, Title: "Pagamentos", Columns: paymentColumns(timeline.ID)}); !errors.Is(err, googlesheet.ErrLinkTargetInvalid) {
		t.Fatalf("a link in a new file = %v", err)
	}
	if _, err := w.manager.Create(ctx, CreateInput{WorkspaceID: testWorkspace, SpreadsheetID: "sp-1", Title: "Pagamentos", Columns: paymentColumns(elsewhere.ID)}); !errors.Is(err, googlesheet.ErrLinkTargetInvalid) {
		t.Fatalf("a link to another file = %v", err)
	}
	created, err := w.manager.Create(ctx, CreateInput{WorkspaceID: testWorkspace, SpreadsheetID: "sp-1", Title: "Pagamentos", Columns: paymentColumns(timeline.ID)})
	if err != nil {
		t.Fatalf("a valid link was refused: %v", err)
	}
	_, err = w.manager.EditColumns(ctx, ColumnsInput{WorkspaceID: testWorkspace, ID: created.ID, Version: created.Version, Edit: googlesheet.ColumnsEdit{
		Add: []googlesheet.ColumnInput{{Header: "Outra conversa", Type: googlesheet.TypeLink, Source: googlesheet.SourceExportLink, LinkSheetID: elsewhere.ID}},
	}})
	if !errors.Is(err, googlesheet.ErrLinkTargetInvalid) {
		t.Fatalf("an added link to another file = %v", err)
	}
}

func TestCreateWorkbookWiresTheLinkToTheSiblingItJustMade(t *testing.T) {
	w := newWorld(t)
	w.api.created = googlesheet.CreatedSpreadsheet{SpreadsheetID: "sp-new", TabID: 0}
	w.api.workbook = googlesheet.Workbook{Tabs: []googlesheet.TabInfo{{ID: 0, Title: "Negociações", Rows: 100, Columns: 3}}}
	payments := WorkbookSheet{Ref: "pagamentos", Input: CreateInput{Title: "Pagamentos", TabPeriod: googlesheet.PeriodMonth, Columns: paymentColumns("")}}
	payments.LinkRefs = map[int]string{2: "negociacoes"}
	sheets, err := w.manager.CreateWorkbook(context.Background(), WorkbookInput{
		WorkspaceID: testWorkspace,
		Sheets: []WorkbookSheet{
			{Ref: "negociacoes", Input: CreateInput{Title: "Negociações", Columns: timelineColumns()}},
			payments,
		},
	})
	if err != nil {
		t.Fatalf("CreateWorkbook: %v", err)
	}
	if len(sheets) != 2 || sheets[0].SpreadsheetID != "sp-new" || sheets[1].SpreadsheetID != "sp-new" {
		t.Fatalf("sheets = %+v", sheets)
	}
	link, _ := sheets[1].Column("negociacao")
	if link.LinkSheetID != sheets[0].ID {
		t.Fatalf("link points at %q, want %q", link.LinkSheetID, sheets[0].ID)
	}
	if len(w.api.added) != 1 || !strings.HasPrefix(w.api.added[0].Title, "Pagamentos · outubro 2026") {
		t.Fatalf("added = %+v", w.api.added)
	}
}

func TestCreateWorkbookUndoesEverySheetWhenALaterOneFails(t *testing.T) {
	w := newWorld(t)
	w.api.created = googlesheet.CreatedSpreadsheet{SpreadsheetID: "sp-new", TabID: 0}
	w.api.workbook = googlesheet.Workbook{Tabs: []googlesheet.TabInfo{{ID: 0, Title: "Negociações", Rows: 100, Columns: 3}}}
	w.api.addErr = errTransient
	_, err := w.manager.CreateWorkbook(context.Background(), WorkbookInput{
		WorkspaceID: testWorkspace,
		Sheets: []WorkbookSheet{
			{Ref: "negociacoes", Input: CreateInput{Title: "Negociações", Columns: timelineColumns()}},
			{Ref: "pagamentos", Input: CreateInput{Title: "Pagamentos", Columns: []googlesheet.ColumnInput{{Header: "Valor", Type: googlesheet.TypeMoney}}}},
		},
	})
	var wbErr *WorkbookError
	if !errors.As(err, &wbErr) || wbErr.Sheet != 1 {
		t.Fatalf("err = %v, want a WorkbookError for sheet 1", err)
	}
	if len(w.api.trashed) != 1 || w.api.trashed[0] != "sp-new" || len(w.sheets.deleted) != 1 {
		t.Fatalf("trashed %v deleted %v: the new file and its first sheet must go", w.api.trashed, w.sheets.deleted)
	}
	_, err = w.manager.CreateWorkbook(context.Background(), WorkbookInput{
		WorkspaceID: testWorkspace,
		Sheets:      []WorkbookSheet{{Ref: "a", Input: CreateInput{Title: "A", Columns: paymentColumns("")}, LinkRefs: map[int]string{2: "later"}}},
	})
	if !errors.As(err, &wbErr) || !errors.Is(err, googlesheet.ErrLinkTargetInvalid) {
		t.Fatalf("a link to an unknown sibling = %v", err)
	}
}

func TestUpdateLayoutRestylesTheCurrentTab(t *testing.T) {
	w := newWorld(t)
	style := googlesheet.StylePurple
	if _, err := w.manager.UpdateLayout(context.Background(), LayoutInput{WorkspaceID: testWorkspace, ID: w.sheet.ID, Version: 9, Style: &style}); !errors.Is(err, googlesheet.ErrStaleSheet) {
		t.Fatalf("stale = %v", err)
	}
	updated, err := w.manager.UpdateLayout(context.Background(), LayoutInput{WorkspaceID: testWorkspace, ID: w.sheet.ID, Version: 1, Style: &style})
	if err != nil || updated.Style != style || updated.Version != 2 {
		t.Fatalf("UpdateLayout = %+v, %v", updated, err)
	}
	last := w.api.layouts[len(w.api.layouts)-1]
	if last.layout.Style != style || last.target.TabID != w.sheet.TabID || last.layout.SizeFrom != len(w.sheet.Columns) {
		t.Fatalf("layout applied = %+v", last)
	}
}

func TestWorkbooksGroupTheSheetsOfEachFile(t *testing.T) {
	w := newWorld(t)
	timeline := w.storeTimeline(t)
	w.storeSheet(t, googlesheet.NewSheetInput{Title: "Outra", Columns: timelineColumns()}, "sp-9", 0)
	books, err := w.manager.Workbooks(context.Background(), testWorkspace)
	if err != nil || len(books) != 2 {
		t.Fatalf("Workbooks = %+v, %v", books, err)
	}
	first := books[0]
	if first.SpreadsheetID != "sp-1" || len(first.Sheets) != 2 || !first.Usable || first.Title != "Pedidos" {
		t.Fatalf("first = %+v", first)
	}
	if first.Sheets[1].Sheet.ID != timeline.ID {
		t.Fatalf("sheets out of order: %+v", first.Sheets)
	}
	w.access.access.AccountID = "acct-2"
	books, _ = w.manager.Workbooks(context.Background(), testWorkspace)
	if books[0].Usable {
		t.Fatal("a file of another account is offered for new tabs")
	}
}

func TestEnqueueFillsTheLinkFromTheLatestExport(t *testing.T) {
	w := newWorld(t)
	timeline := w.storeTimeline(t)
	payments := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Pagamentos", Columns: paymentColumns(timeline.ID)}, "sp-1", 41)
	origin := googlesheet.Origin{Kind: googlesheet.OriginWorkflow, RunID: "run-1", EntryID: "conv-1", EntryType: "whatsapp"}
	in := EnqueueInput{WorkspaceID: testWorkspace, SheetID: payments.ID, Values: map[string]any{"valor": "10"}, IdempotencyKey: "k1", Origin: origin}
	if _, err := w.intake.Enqueue(context.Background(), in); !errors.Is(err, googlesheet.ErrLinkExportMissing) {
		t.Fatalf("no export yet = %v", err)
	}
	w.exports.anchors[exportKey(timeline.ID, "conv-1", "whatsapp")] = googlesheet.ExportAnchor{RowID: anchorRowID}
	res, err := w.intake.Enqueue(context.Background(), in)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	ref, err := googlesheet.ParseLinkRef(res.Cells["negociacao"].(string))
	if err != nil || ref.RowID != anchorRowID || ref.SheetID != timeline.ID {
		t.Fatalf("link cell = %v, %v", res.Cells["negociacao"], err)
	}
}

func TestWriterPutsTheLinkFormulaPointingAtTheTimelineTab(t *testing.T) {
	w := newWorld(t)
	timeline := w.storeTimeline(t)
	payments := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Pagamentos", Columns: paymentColumns(timeline.ID)}, "sp-1", 41)
	w.api.headers[41] = toAny(payments.HeaderRow())
	w.api.workbook.Tabs = append(w.api.workbook.Tabs, googlesheet.TabInfo{ID: 41, Title: "Pagamentos", Rows: 100, Columns: 4})
	w.exports.anchors[exportKey(timeline.ID, "conv-1", "whatsapp")] = googlesheet.ExportAnchor{RowID: anchorRowID}
	origin := googlesheet.Origin{Kind: googlesheet.OriginWorkflow, RunID: "run-1", EntryID: "conv-1", EntryType: "whatsapp"}
	if _, err := w.intake.Enqueue(context.Background(), EnqueueInput{WorkspaceID: testWorkspace, SheetID: payments.ID, Values: map[string]any{"valor": "10"}, IdempotencyKey: "k1", Origin: origin}); err != nil {
		t.Fatal(err)
	}
	if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	if len(w.api.appends) != 1 || w.api.appends[0].tab != 41 {
		t.Fatalf("appends = %+v", w.api.appends)
	}
	formula, ok := w.api.appends[0].rows[0][3].(googlesheet.Formula)
	want, _ := googlesheet.LinkFormula(googlesheet.TabRef{ID: 31, Title: "Negociações"}, anchorRowID, googlesheet.DefaultLinkLabel)
	if !ok || formula != want {
		t.Fatalf("link cell = %#v, want %s", w.api.appends[0].rows[0][3], want)
	}
}

func TestALinkToAPeriodTabThatDoesNotExistYetWaitsThenFails(t *testing.T) {
	w := newWorld(t)
	timeline := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Negociações", TabPeriod: googlesheet.PeriodMonth, Columns: timelineColumns()}, "sp-1", 31)
	payments := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Pagamentos", Columns: paymentColumns(timeline.ID)}, "sp-1", 41)
	w.api.headers[41] = toAny(payments.HeaderRow())
	w.api.workbook.Tabs = append(w.api.workbook.Tabs, googlesheet.TabInfo{ID: 41, Title: "Pagamentos", Rows: 100, Columns: 4})
	w.exports.anchors[exportKey(timeline.ID, "conv-1", "whatsapp")] = googlesheet.ExportAnchor{RowID: anchorRowID, Period: "2026-11"}
	origin := googlesheet.Origin{Kind: googlesheet.OriginWorkflow, RunID: "run-1", EntryID: "conv-1", EntryType: "whatsapp"}
	if _, err := w.intake.Enqueue(context.Background(), EnqueueInput{WorkspaceID: testWorkspace, SheetID: payments.ID, Values: map[string]any{"valor": "10"}, IdempotencyKey: "k1", Origin: origin}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < googlesheet.MaxTransientAttempts; i++ {
		if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			pending := w.rows.byStatus(payments.ID, googlesheet.RowPending)
			if len(pending) != 1 || pending[0].ErrorCode != googlesheet.CodeWriteLinkPending {
				t.Fatalf("first try = %+v", pending)
			}
		}
		w.now = w.now.Add(2 * googlesheet.MaxBackoff)
	}
	if len(w.api.appends) != 0 {
		t.Fatal("a row was written with a link that points nowhere")
	}
	failed := w.rows.byStatus(payments.ID, googlesheet.RowFailed)
	if len(failed) != 1 || failed[0].ErrorCode != googlesheet.CodeWriteLinkTargetMissing {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestALinkToADeletedTimelineFailsAtOnce(t *testing.T) {
	w := newWorld(t)
	timeline := w.storeTimeline(t)
	payments := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Pagamentos", Columns: paymentColumns(timeline.ID)}, "sp-1", 41)
	w.api.headers[41] = toAny(payments.HeaderRow())
	w.api.workbook.Tabs = append(w.api.workbook.Tabs, googlesheet.TabInfo{ID: 41, Title: "Pagamentos", Rows: 100, Columns: 4})
	w.exports.anchors[exportKey(timeline.ID, "conv-1", "whatsapp")] = googlesheet.ExportAnchor{RowID: anchorRowID}
	origin := googlesheet.Origin{Kind: googlesheet.OriginWorkflow, RunID: "run-1", EntryID: "conv-1", EntryType: "whatsapp"}
	if _, err := w.intake.Enqueue(context.Background(), EnqueueInput{WorkspaceID: testWorkspace, SheetID: payments.ID, Values: map[string]any{"valor": "10"}, IdempotencyKey: "k1", Origin: origin}); err != nil {
		t.Fatal(err)
	}
	delete(w.sheets.sheets, timeline.ID)
	if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	failed := w.rows.byStatus(payments.ID, googlesheet.RowFailed)
	if len(failed) != 1 || failed[0].ErrorCode != googlesheet.CodeWriteLinkTargetMissing || len(w.api.appends) != 0 {
		t.Fatalf("failed = %+v, appends %d", failed, len(w.api.appends))
	}
}

func (w *world) periodicPayments(t *testing.T) *googlesheet.Sheet {
	t.Helper()
	s := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Pagamentos", TabPeriod: googlesheet.PeriodMonth, Columns: []googlesheet.ColumnInput{{Header: "Valor", Type: googlesheet.TypeMoney}}}, "sp-1", 51)
	w.api.workbook.Tabs = append(w.api.workbook.Tabs, googlesheet.TabInfo{ID: 51, Title: s.TabName, Rows: 100, Columns: 2})
	w.api.headers[51] = toAny(s.HeaderRow())
	return s
}

func (w *world) enqueueOn(t *testing.T, sheet *googlesheet.Sheet, key string, values map[string]any) {
	t.Helper()
	if _, err := w.intake.Enqueue(context.Background(), EnqueueInput{WorkspaceID: testWorkspace, SheetID: sheet.ID, Values: values, IdempotencyKey: key, Origin: googlesheet.Origin{Kind: googlesheet.OriginManual}}); err != nil {
		t.Fatal(err)
	}
	w.now = w.now.Add(time.Millisecond)
}

func TestTheFirstRowOfANewMonthOpensItsTab(t *testing.T) {
	w := newWorld(t)
	s := w.periodicPayments(t)
	w.enqueueOn(t, s, "oct", map[string]any{"valor": 1})
	w.now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	w.enqueueOn(t, s, "nov", map[string]any{"valor": 2})
	if err := w.writer.WriteSheet(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if len(w.api.appends) != 2 || w.api.appends[0].tab != 51 {
		t.Fatalf("appends = %+v, want October in its own tab and November in a new one", w.api.appends)
	}
	if len(w.api.added) != 1 || w.api.added[0].Title != "Pagamentos · novembro 2026" || w.api.appends[1].tab != w.api.added[0].ID {
		t.Fatalf("added = %+v, appends = %+v", w.api.added, w.api.appends)
	}
	stored, _ := w.sheets.Get(context.Background(), s.ID)
	if stored.TabID != w.api.added[0].ID || stored.TabPeriodKey != "2026-11" {
		t.Fatalf("sheet still points at %d %q", stored.TabID, stored.TabPeriodKey)
	}
	w.enqueueOn(t, s, "nov2", map[string]any{"valor": 3})
	if err := w.writer.WriteSheet(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if len(w.api.added) != 1 {
		t.Fatal("the November tab was added twice")
	}
}

func TestAPeriodTabLeftByACrashIsAdoptedNotDuplicated(t *testing.T) {
	w := newWorld(t)
	s := w.periodicPayments(t)
	w.api.workbook.Tabs = append(w.api.workbook.Tabs, googlesheet.TabInfo{ID: 77, Title: "Pagamentos · novembro 2026", Rows: 100, Columns: 2})
	w.api.headers[77] = []any{}
	w.now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	w.enqueueOn(t, s, "nov", map[string]any{"valor": 2})
	if err := w.writer.WriteSheet(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if len(w.api.added) != 0 || len(w.api.appends) != 1 || w.api.appends[0].tab != 77 {
		t.Fatalf("added %+v appends %+v", w.api.added, w.api.appends)
	}
	if last := w.api.layouts[len(w.api.layouts)-1]; last.target.TabID != 77 {
		t.Fatalf("the adopted blank tab was not laid out: %+v", last)
	}
}

func TestAForeignTabWithTheSameNameStopsTheSheet(t *testing.T) {
	w := newWorld(t)
	s := w.periodicPayments(t)
	w.api.workbook.Tabs = append(w.api.workbook.Tabs, googlesheet.TabInfo{ID: 77, Title: "Pagamentos · novembro 2026", Rows: 100, Columns: 2})
	w.api.headers[77] = []any{"Data", "Total"}
	w.now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	w.enqueueOn(t, s, "nov", map[string]any{"valor": 2})
	if err := w.writer.WriteSheet(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	stored, _ := w.sheets.Get(context.Background(), s.ID)
	if len(w.api.appends) != 0 || stored.Reason != googlesheet.ReasonHeaderMismatch {
		t.Fatalf("appends %d, sheet %+v", len(w.api.appends), stored)
	}
	if len(w.rows.byStatus(s.ID, googlesheet.RowPending)) != 1 {
		t.Fatal("the row was not kept")
	}
}

func TestTheRoomCheckCountsEveryTabOfTheFile(t *testing.T) {
	w := newWorld(t)
	w.api.workbook.Tabs = append(w.api.workbook.Tabs, googlesheet.TabInfo{ID: 99, Title: "Enorme", Rows: 950_000, Columns: 10})
	w.enqueue(t, "k1", map[string]any{"nome": "a"})
	w.write(t)
	if s := w.stored(t); s.Reason != googlesheet.ReasonSheetFull || len(w.api.appends) != 0 {
		t.Fatalf("sheet = %+v, appends %d", s, len(w.api.appends))
	}
}

func TestListRowsShowsTheLinkLabel(t *testing.T) {
	w := newWorld(t)
	timeline := w.storeTimeline(t)
	payments := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Pagamentos", Columns: paymentColumns(timeline.ID)}, "sp-1", 41)
	w.exports.anchors[exportKey(timeline.ID, "conv-1", "whatsapp")] = googlesheet.ExportAnchor{RowID: anchorRowID}
	origin := googlesheet.Origin{Kind: googlesheet.OriginWorkflow, RunID: "run-1", EntryID: "conv-1", EntryType: "whatsapp"}
	if _, err := w.intake.Enqueue(context.Background(), EnqueueInput{WorkspaceID: testWorkspace, SheetID: payments.ID, Values: map[string]any{"valor": "10"}, IdempotencyKey: "k1", Origin: origin}); err != nil {
		t.Fatal(err)
	}
	page, err := w.manager.ListRows(context.Background(), testWorkspace, payments.ID, shared.Pagination{Page: 1, PageSize: 10})
	if err != nil || page.Items[0].Cells["negociacao"] != googlesheet.DefaultLinkLabel {
		t.Fatalf("rows = %+v, %v", page, err)
	}
}

func TestReactivateRefusesWhenTheTabIsGone(t *testing.T) {
	w := newWorld(t)
	w.api.workbook.Tabs = nil
	_, err := w.manager.Reactivate(context.Background(), ReactivateInput{WorkspaceID: testWorkspace, ID: w.sheet.ID})
	var refusal *GoogleRefusal
	if !errors.As(err, &refusal) || refusal.Failure.Kind != googlesheet.FailureNotFound {
		t.Fatalf("err = %v", err)
	}
}
