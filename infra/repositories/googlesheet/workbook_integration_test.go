package googlesheet_repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

func monthlySheet(t *testing.T, repo googlesheet.SheetRepository, workspaceID, spreadsheetID string, at time.Time) *googlesheet.Sheet {
	t.Helper()
	s, err := googlesheet.NewSheet(googlesheet.NewSheetInput{
		WorkspaceID: workspaceID, AccountID: uuid.NewString(), Title: "Pagamentos",
		TabPeriod: googlesheet.PeriodMonth, TabPattern: "Pagamentos · {mes} {ano}", Style: googlesheet.StyleGreen,
		Columns: []googlesheet.ColumnInput{{Header: "Valor", Type: googlesheet.TypeMoney}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.OpenPeriod(at); err != nil {
		t.Fatal(err)
	}
	s.SpreadsheetID, s.TabID = spreadsheetID, 5
	if err := repo.Create(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAPeriodicSheetKeepsEveryTabAndMovesToTheNewest(t *testing.T) {
	db := integrationDB(t)
	repo := NewSheetRepository(db)
	ctx := context.Background()
	s := monthlySheet(t, repo, uuid.NewString(), "file-1", time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))

	got, err := repo.Get(ctx, s.ID)
	if err != nil || got.TabPeriod != googlesheet.PeriodMonth || got.TabPattern != "Pagamentos · {mes} {ano}" || got.Style != googlesheet.StyleGreen || got.TabPeriodKey != "2026-10" {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
	first, found, err := repo.FindTab(ctx, s.ID, "2026-10")
	if err != nil || !found || first.ID != 5 || first.Title != "Pagamentos · outubro 2026" {
		t.Fatalf("first tab = %+v %v %v", first, found, err)
	}
	if _, found, _ := repo.FindTab(ctx, s.ID, "2026-11"); found {
		t.Fatal("a tab that was never made was found")
	}
	if err := repo.OpenTab(ctx, s.ID, googlesheet.Tab{Period: "2026-11", ID: 9, Title: "Pagamentos · novembro 2026"}); err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	got, _ = repo.Get(ctx, s.ID)
	if got.TabID != 9 || got.TabName != "Pagamentos · novembro 2026" || got.TabPeriodKey != "2026-11" {
		t.Fatalf("current tab = %d %q %q", got.TabID, got.TabName, got.TabPeriodKey)
	}
	if err := repo.OpenTab(ctx, s.ID, googlesheet.Tab{Period: "2026-09", ID: 3, Title: "Pagamentos · setembro 2026"}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(ctx, s.ID)
	if got.TabID != 9 {
		t.Fatalf("an older period moved the current tab to %d", got.TabID)
	}
	if err := repo.OpenTab(ctx, s.ID, googlesheet.Tab{Period: "2026-11", ID: 9, Title: "Pagamentos · novembro 2026"}); err != nil {
		t.Fatalf("opening the same tab twice must be harmless: %v", err)
	}
	tabs, err := repo.Tabs(ctx, s.ID)
	if err != nil || len(tabs) != 3 || tabs[0].Period != "2026-09" || tabs[2].Period != "2026-11" {
		t.Fatalf("tabs = %+v, %v", tabs, err)
	}
	if err := repo.UpdateTabName(ctx, s.ID, "Novembro pago"); err != nil {
		t.Fatal(err)
	}
	renamed, _, _ := repo.FindTab(ctx, s.ID, "2026-11")
	if renamed.Title != "Novembro pago" {
		t.Fatalf("rename on Google did not reach the period tab: %+v", renamed)
	}
	style := googlesheet.StyleBlue
	pattern := "Pagos {mm}-{ano}"
	if err := got.ChangeLayout(&style, &pattern); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, got, got.Version-1); err != nil {
		t.Fatalf("Update: %v", err)
	}
	again, _ := repo.Get(ctx, s.ID)
	if again.Style != googlesheet.StyleBlue || again.TabPattern != pattern {
		t.Fatalf("layout not saved: %q %q", again.Style, again.TabPattern)
	}
}

func TestSheetsOfOneSpreadsheetAreFoundTogether(t *testing.T) {
	db := integrationDB(t)
	repo := NewSheetRepository(db)
	ctx := context.Background()
	ws := uuid.NewString()
	a := monthlySheet(t, repo, ws, "file-1", time.Now())
	b, _ := googlesheet.NewSheet(googlesheet.NewSheetInput{
		WorkspaceID: ws, AccountID: a.AccountID, Title: "Negociações",
		Columns: []googlesheet.ColumnInput{{Header: "Texto", Type: googlesheet.TypeLongText, Source: googlesheet.SourceMessageText}},
	})
	b.SpreadsheetID, b.TabID = "file-1", 0
	if err := repo.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	monthlySheet(t, repo, ws, "file-2", time.Now())
	monthlySheet(t, repo, uuid.NewString(), "file-1", time.Now())

	inFile, err := repo.InSpreadsheet(ctx, ws, "file-1")
	if err != nil || len(inFile) != 2 {
		t.Fatalf("InSpreadsheet = %d, %v", len(inFile), err)
	}
	all, err := repo.ListAll(ctx, ws)
	if err != nil || len(all) != 3 {
		t.Fatalf("ListAll = %d, %v", len(all), err)
	}
	if all[0].ID != a.ID {
		t.Fatalf("ListAll must be oldest first, got %s", all[0].Title)
	}
	page, err := repo.List(ctx, googlesheet.ListInput{WorkspaceID: ws, TakesInput: true, Pagination: shared.Pagination{Page: 1, PageSize: 10}})
	if err != nil || page.TotalItems != 2 {
		t.Fatalf("sheets that take input = %d, %v; the timeline has no input column", page.TotalItems, err)
	}
	for _, s := range page.Items {
		if s.ID == b.ID {
			t.Fatal("the timeline was listed as taking input")
		}
	}
}

func TestRowsKeepThePeriodTheyWereMadeIn(t *testing.T) {
	db := integrationDB(t)
	sheets, rows := NewSheetRepository(db), NewRowRepository(db)
	ctx := context.Background()
	s := monthlySheet(t, sheets, uuid.NewString(), "file-1", time.Now())
	at := time.Date(2026, 11, 1, 2, 59, 0, 0, time.UTC)
	row, err := googlesheet.NewRow(s, googlesheet.Cells{"valor": 10.0}, "k1", googlesheet.Origin{Kind: googlesheet.OriginManual}, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := rows.Insert(ctx, row); err != nil {
		t.Fatal(err)
	}
	claimed, err := rows.Claim(ctx, s.ID, uuid.NewString(), 10, at.Add(time.Second))
	if err != nil || len(claimed) != 1 || claimed[0].PeriodKey != "2026-10" {
		t.Fatalf("claimed = %+v, %v", claimed, err)
	}
}

func TestTheExportAnchorIsTheFirstRowOfTheLatestBlock(t *testing.T) {
	db := integrationDB(t)
	sheets, exports, rows := NewSheetRepository(db), NewExportRepository(db), NewRowRepository(db)
	ctx := context.Background()
	s := storedSheet(t, sheets)
	entryID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if _, found, err := exports.Anchor(ctx, s.ID, entryID, "whatsapp"); err != nil || found {
		t.Fatalf("a conversation never exported has an anchor: %v %v", found, err)
	}
	first := exportOf(t, s, entryID, []string{"m1", "m2"}, googlesheet.ExportPosition{At: now.Add(-time.Hour), MessageID: "m2"}, now)
	if _, err := exports.Record(ctx, first); err != nil {
		t.Fatal(err)
	}
	claimed, _ := rows.Claim(ctx, s.ID, uuid.NewString(), 10, now.Add(time.Second))
	anchor, found, err := exports.Anchor(ctx, s.ID, entryID, "whatsapp")
	if err != nil || !found || anchor.RowID != claimed[0].ID {
		t.Fatalf("anchor = %+v %v %v, want the first row %s", anchor, found, err, claimed[0].ID)
	}
	empty := googlesheet.MessageExport{WorkspaceID: s.WorkspaceID, SheetID: s.ID, EntryID: entryID, EntryType: "whatsapp", Through: googlesheet.ExportPosition{At: now.Add(-30 * time.Minute), MessageID: "m3"}}
	if _, err := exports.Record(ctx, empty); err != nil {
		t.Fatal(err)
	}
	if kept, _, _ := exports.Anchor(ctx, s.ID, entryID, "whatsapp"); kept.RowID != anchor.RowID {
		t.Fatalf("an export without rows moved the anchor to %+v", kept)
	}
	later := exportOf(t, s, entryID, []string{"m4", "m5"}, googlesheet.ExportPosition{At: now.Add(-time.Minute), MessageID: "m5"}, now.Add(time.Minute))
	if _, err := exports.Record(ctx, later); err != nil {
		t.Fatal(err)
	}
	moved, _, _ := exports.Anchor(ctx, s.ID, entryID, "whatsapp")
	var m4 string
	if err := db.Raw("SELECT id FROM google_sheet_rows WHERE sheet_id = ? AND idempotency_key = ?", s.ID, googlesheet.KeyForMessage("whatsapp", entryID, "m4")).Row().Scan(&m4); err != nil {
		t.Fatal(err)
	}
	if moved.RowID != m4 {
		t.Fatalf("anchor = %s, want the first row of the new block %s", moved.RowID, m4)
	}
}

func tabOfFile(t *testing.T, db *gorm.DB, workspaceID, spreadsheetID, title string, tabID int64, born time.Time) *googlesheet.Sheet {
	t.Helper()
	s, err := googlesheet.NewSheet(googlesheet.NewSheetInput{
		WorkspaceID: workspaceID, AccountID: uuid.NewString(), Title: title,
		Columns: []googlesheet.ColumnInput{{Header: "Valor", Type: googlesheet.TypeMoney}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.SpreadsheetID, s.TabID = spreadsheetID, tabID
	if err := NewSheetRepository(db).Create(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE google_sheets SET created_at = ? WHERE id = ?", born, s.ID).Error; err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSpreadsheetsArePagedByFileWithEveryTabOfEachFile(t *testing.T) {
	db := integrationDB(t)
	repo := NewSheetRepository(db)
	ctx := context.Background()
	ws := uuid.NewString()
	day := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tabOfFile(t, db, ws, "file-3", "Antiga", 0, day)
	negotiations := tabOfFile(t, db, ws, "file-1", "Negociações", 0, day.Add(time.Hour))
	tabOfFile(t, db, ws, "file-2", "Pedidos", 0, day.Add(2*time.Hour))
	payments := tabOfFile(t, db, ws, "file-1", "Pagamentos", 7, day.Add(3*time.Hour))
	tabOfFile(t, db, uuid.NewString(), "file-1", "Outro workspace", 0, day.Add(4*time.Hour))

	first, err := repo.ListSpreadsheets(ctx, googlesheet.ListInput{WorkspaceID: ws, Pagination: shared.Pagination{Page: 1, PageSize: 2}})
	if err != nil {
		t.Fatalf("ListSpreadsheets: %v", err)
	}
	if first.TotalItems != 3 || first.TotalPages != 2 || len(first.Items) != 2 {
		t.Fatalf("page = %d items of %d in %d pages", len(first.Items), first.TotalItems, first.TotalPages)
	}
	if first.Items[0].ID != "file-1" || first.Items[1].ID != "file-2" {
		t.Fatalf("files must come newest first: %s, %s", first.Items[0].ID, first.Items[1].ID)
	}
	tabs := first.Items[0].Sheets
	if len(tabs) != 2 || tabs[0].ID != negotiations.ID || tabs[1].ID != payments.ID {
		t.Fatalf("tabs of file-1 = %+v", tabs)
	}
	second, err := repo.ListSpreadsheets(ctx, googlesheet.ListInput{WorkspaceID: ws, Pagination: shared.Pagination{Page: 2, PageSize: 2}})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != "file-3" {
		t.Fatalf("second page = %+v, %v", second, err)
	}
}

func TestASpreadsheetSearchFindsTheFileAndKeepsAllOfItsTabs(t *testing.T) {
	db := integrationDB(t)
	repo := NewSheetRepository(db)
	ctx := context.Background()
	ws := uuid.NewString()
	day := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tabOfFile(t, db, ws, "file-1", "Pagamentos", 0, day)
	negotiations := tabOfFile(t, db, ws, "file-1", "Negociações", 7, day.Add(time.Hour))
	tabOfFile(t, db, ws, "file-2", "Pedidos", 0, day.Add(2*time.Hour))
	search := func(text string) *shared.PaginatedResult[googlesheet.Spreadsheet] {
		t.Helper()
		page, err := repo.ListSpreadsheets(ctx, googlesheet.ListInput{WorkspaceID: ws, Search: text, Pagination: shared.Pagination{Page: 1, PageSize: 10}})
		if err != nil {
			t.Fatalf("ListSpreadsheets(%q): %v", text, err)
		}
		return page
	}

	found := search("negocia")
	if found.TotalItems != 1 || len(found.Items) != 1 || found.Items[0].ID != "file-1" {
		t.Fatalf("search = %+v", found)
	}
	if len(found.Items[0].Sheets) != 2 {
		t.Fatalf("a file found by one tab lost its other tabs: %d", len(found.Items[0].Sheets))
	}
	if err := repo.Delete(ctx, ws, negotiations.ID); err != nil {
		t.Fatal(err)
	}
	if gone := search("negocia"); gone.TotalItems != 0 || len(gone.Items) != 0 {
		t.Fatalf("a removed tab is still found: %+v", gone)
	}
	rest := search("")
	if rest.TotalItems != 2 || rest.Items[1].ID != "file-1" || len(rest.Items[1].Sheets) != 1 {
		t.Fatalf("files after removing a tab = %+v", rest)
	}
}

func TestSpreadsheetsCanBeNarrowedToOneFile(t *testing.T) {
	db := integrationDB(t)
	repo := NewSheetRepository(db)
	ctx := context.Background()
	ws := uuid.NewString()
	day := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tabOfFile(t, db, ws, "file-1", "Negociações", 0, day)
	tabOfFile(t, db, ws, "file-1", "Pagamentos", 7, day.Add(time.Hour))
	tabOfFile(t, db, ws, "file-2", "Pedidos", 0, day.Add(2*time.Hour))

	page, err := repo.ListSpreadsheets(ctx, googlesheet.ListInput{WorkspaceID: ws, SpreadsheetID: "file-1", Pagination: shared.Pagination{Page: 1, PageSize: 10}})
	if err != nil || page.TotalItems != 1 || len(page.Items) != 1 || page.Items[0].ID != "file-1" || len(page.Items[0].Sheets) != 2 {
		t.Fatalf("one file = %+v, %v", page, err)
	}
	other, err := repo.ListSpreadsheets(ctx, googlesheet.ListInput{WorkspaceID: uuid.NewString(), SpreadsheetID: "file-1", Pagination: shared.Pagination{Page: 1, PageSize: 10}})
	if err != nil || other.TotalItems != 0 {
		t.Fatalf("another workspace saw the file: %+v, %v", other, err)
	}
}

func TestSpreadsheetsCanBeNarrowedToFilesWithAMessageTab(t *testing.T) {
	db := integrationDB(t)
	repo := NewSheetRepository(db)
	ctx := context.Background()
	ws := uuid.NewString()
	timeline, err := googlesheet.NewSheet(googlesheet.NewSheetInput{
		WorkspaceID: ws, AccountID: uuid.NewString(), Title: "Negociações",
		Columns: []googlesheet.ColumnInput{{Header: "Texto", Type: googlesheet.TypeLongText, Source: googlesheet.SourceMessageText}},
	})
	if err != nil {
		t.Fatal(err)
	}
	timeline.SpreadsheetID, timeline.TabID = "file-1", 0
	if err := repo.Create(ctx, timeline); err != nil {
		t.Fatal(err)
	}
	tabOfFile(t, db, ws, "file-1", "Pagamentos", 7, time.Now())
	tabOfFile(t, db, ws, "file-2", "Pedidos", 0, time.Now())

	page, err := repo.ListSpreadsheets(ctx, googlesheet.ListInput{WorkspaceID: ws, TakesMessages: true, Pagination: shared.Pagination{Page: 1, PageSize: 10}})
	if err != nil || page.TotalItems != 1 || page.Items[0].ID != "file-1" || len(page.Items[0].Sheets) != 2 {
		t.Fatalf("files with a message tab = %+v, %v", page, err)
	}
}
