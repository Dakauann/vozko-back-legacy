package googlesheet

import (
	"context"
	"time"

	"vozko/domain/shared"
)

type Target struct {
	SpreadsheetID string
	TabID         int64
	TabName       string
}

func (s *Sheet) Target() Target {
	return Target{SpreadsheetID: s.SpreadsheetID, TabID: s.TabID, TabName: s.TabName}
}

type SpreadsheetSpec struct {
	Title    string
	TabName  string
	Locale   string
	TimeZone string
	Layout   Layout
}

func (s *Sheet) Spec(fileTitle string) SpreadsheetSpec {
	return SpreadsheetSpec{Title: fileTitle, TabName: s.TabName, Locale: s.Locale, TimeZone: s.TimeZone, Layout: s.Layout(0)}
}

type TabSpec struct {
	ID     int64
	Title  string
	Layout Layout
}

type CreatedSpreadsheet struct {
	SpreadsheetID string
	TabID         int64
}

type SheetsAPI interface {
	CreateSpreadsheet(ctx context.Context, token string, spec SpreadsheetSpec) (CreatedSpreadsheet, error)
	TrashSpreadsheet(ctx context.Context, token, spreadsheetID string) error
	RenameSpreadsheet(ctx context.Context, token, spreadsheetID, title string) error
	AddTab(ctx context.Context, token, spreadsheetID string, tab TabSpec) error
	DeleteTab(ctx context.Context, token, spreadsheetID string, tabID int64) error
	ApplyLayout(ctx context.Context, token string, target Target, layout Layout) error
	RepairHeader(ctx context.Context, token string, target Target, repair HeaderRepair, layout Layout) error
	InsertRows(ctx context.Context, token string, target Target, at int, rows [][]any) error
	DeleteRow(ctx context.Context, token string, target Target, row int) error
	ReadRange(ctx context.Context, token string, target Target, a1 string) ([][]any, error)
	ReadRanges(ctx context.Context, token string, target Target, ranges []string) ([][][]any, error)
	Workbook(ctx context.Context, token, spreadsheetID string) (Workbook, error)
	LinkCells(ctx context.Context, token string, target Target, columns []int) ([]LinkCell, error)
	WriteFormulas(ctx context.Context, token string, target Target, cells []FormulaCell) error
}

type ErrorClassifier interface {
	Classify(err error) Failure
}

type ListInput struct {
	WorkspaceID   string
	SpreadsheetID string
	Search        string
	TakesInput    bool
	TakesMessages bool
	Pagination    shared.Pagination
}

type SheetRepository interface {
	Create(ctx context.Context, s *Sheet) error
	Update(ctx context.Context, s *Sheet, expectedVersion int) error
	SaveStatus(ctx context.Context, s *Sheet) error
	UpdateTabName(ctx context.Context, id, tabName string) error
	OpenTab(ctx context.Context, id string, tab Tab) error
	FindTab(ctx context.Context, sheetID, period string) (Tab, bool, error)
	Tabs(ctx context.Context, sheetID string) ([]Tab, error)
	InSpreadsheet(ctx context.Context, workspaceID, spreadsheetID string) ([]*Sheet, error)
	ListAll(ctx context.Context, workspaceID string) ([]*Sheet, error)
	Get(ctx context.Context, id string) (*Sheet, error)
	FindInWorkspace(ctx context.Context, workspaceID, id string) (*Sheet, error)
	List(ctx context.Context, in ListInput) (*shared.PaginatedResult[*Sheet], error)
	ListSpreadsheets(ctx context.Context, in ListInput) (*shared.PaginatedResult[Spreadsheet], error)
	SaveFileTitle(ctx context.Context, workspaceID, spreadsheetID, title string) error
	FileTitles(ctx context.Context, workspaceID string, spreadsheetIDs []string) (map[string]string, error)
	Delete(ctx context.Context, workspaceID, id string) error
	TouchWritten(ctx context.Context, id string, at time.Time) error
}

type PendingSheet struct {
	SheetID         string
	OldestPendingAt time.Time
}

type WrittenRow struct {
	ID        string
	RowNumber int
}

const HealthCountCap = 100_000

type Health struct {
	Pending         int
	InFlight        int
	Held            int
	Failed          int
	Written         int
	OldestPendingAt *time.Time
	Problems        []*Row
}

type RowRepository interface {
	ListRows(ctx context.Context, workspaceID, sheetID string, pagination shared.Pagination) (*shared.PaginatedResult[*Row], error)
	Insert(ctx context.Context, row *Row) (stored *Row, duplicate bool, err error)
	InsertUnique(ctx context.Context, row *Row, unique UniqueKey) (Inserted, error)
	LedgerPage(ctx context.Context, sheetID string, after LedgerCursor, limit int) ([]*Row, error)
	ClaimUnique(ctx context.Context, claims []UniqueClaim) (int, error)
	PendingSheets(ctx context.Context, readyAt time.Time, limit int) ([]PendingSheet, error)
	Claim(ctx context.Context, sheetID, batchID string, limit int, now time.Time) ([]*Row, error)
	MarkWritten(ctx context.Context, rows []WrittenRow, at time.Time) error
	Release(ctx context.Context, ids []string, nextAttemptAt time.Time, code string) error
	Unclaim(ctx context.Context, ids []string) error
	Hold(ctx context.Context, ids []string, code, detail string) error
	Fail(ctx context.Context, id, code, detail string) error
	RequeueProblems(ctx context.Context, sheetID string, now time.Time) (int, error)
	StuckInFlight(ctx context.Context, sheetID string, claimedBefore time.Time) ([]*Row, error)
	PurgeWritten(ctx context.Context, writtenBefore time.Time, limit int) (int, error)
	DeleteForLead(ctx context.Context, workspaceID, leadID string) (int, error)
	DeleteUnwritten(ctx context.Context, sheetID string) error
	Health(ctx context.Context, sheetID string, problems int) (Health, error)
}

type SystemValueResolver interface {
	Resolve(ctx context.Context, workspaceID string, origin Origin) (SystemValues, error)
}

type Limiter interface {
	TakeWrite(ctx context.Context, accountKey string) (granted bool, err error)
	TakeRead(ctx context.Context, accountKey string) (granted bool, err error)
	WritePressure(ctx context.Context) (float64, error)
}

type ExportRepository interface {
	Position(ctx context.Context, sheetID, entryID, entryType string) (ExportPosition, error)
	Record(ctx context.Context, export MessageExport) (inserted int, err error)
	Anchor(ctx context.Context, sheetID, entryID, entryType string) (ExportAnchor, bool, error)
}
