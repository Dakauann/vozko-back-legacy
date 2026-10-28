package googlesheet

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"vozko/domain/googleaccount"
	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

type memorySheets struct {
	mu      sync.Mutex
	sheets  map[string]*googlesheet.Sheet
	tabs    map[string]map[string]googlesheet.Tab
	titles  map[string]string
	deleted []string
	fail    error
}

func newMemorySheets() *memorySheets {
	return &memorySheets{sheets: map[string]*googlesheet.Sheet{}, tabs: map[string]map[string]googlesheet.Tab{}, titles: map[string]string{}}
}

func (m *memorySheets) SaveFileTitle(_ context.Context, workspaceID, spreadsheetID, title string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.titles[workspaceID+"/"+spreadsheetID] = title
	return nil
}

func (m *memorySheets) FileTitles(_ context.Context, workspaceID string, spreadsheetIDs []string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]string{}
	for _, id := range spreadsheetIDs {
		if title, ok := m.titles[workspaceID+"/"+id]; ok {
			out[id] = title
		}
	}
	return out, nil
}

func (m *memorySheets) saveTab(id string, tab googlesheet.Tab) {
	if m.tabs[id] == nil {
		m.tabs[id] = map[string]googlesheet.Tab{}
	}
	if _, ok := m.tabs[id][tab.Period]; !ok {
		m.tabs[id][tab.Period] = tab
	}
}

func (m *memorySheets) OpenTab(_ context.Context, id string, tab googlesheet.Tab) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sheets[id]
	if !ok {
		return googlesheet.ErrSheetNotFound
	}
	m.saveTab(id, tab)
	if s.TabPeriodKey < tab.Period {
		s.TabID, s.TabName, s.TabPeriodKey = tab.ID, tab.Title, tab.Period
	}
	return nil
}

func (m *memorySheets) FindTab(_ context.Context, sheetID, period string) (googlesheet.Tab, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tab, ok := m.tabs[sheetID][period]
	return tab, ok, nil
}

func (m *memorySheets) Tabs(_ context.Context, sheetID string) ([]googlesheet.Tab, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []googlesheet.Tab
	for _, tab := range m.tabs[sheetID] {
		out = append(out, tab)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Period < out[j].Period })
	return out, nil
}

func (m *memorySheets) InSpreadsheet(ctx context.Context, workspaceID, spreadsheetID string) ([]*googlesheet.Sheet, error) {
	all, _ := m.ListAll(ctx, workspaceID)
	var out []*googlesheet.Sheet
	for _, s := range all {
		if s.SpreadsheetID == spreadsheetID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *memorySheets) ListAll(_ context.Context, workspaceID string) ([]*googlesheet.Sheet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*googlesheet.Sheet
	for _, s := range m.sheets {
		if s.WorkspaceID == workspaceID {
			out = append(out, copySheet(s))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func copySheet(s *googlesheet.Sheet) *googlesheet.Sheet {
	c := *s
	c.Columns = append([]googlesheet.Column(nil), s.Columns...)
	return &c
}

func (m *memorySheets) Create(_ context.Context, s *googlesheet.Sheet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Date(2026, 1, 1, 0, 0, len(m.sheets), 0, time.UTC)
	}
	m.sheets[s.ID] = copySheet(s)
	if s.Periodic() {
		m.saveTab(s.ID, googlesheet.Tab{Period: s.TabPeriodKey, ID: s.TabID, Title: s.TabName})
	}
	return nil
}

func (m *memorySheets) Update(_ context.Context, s *googlesheet.Sheet, expected int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.sheets[s.ID]
	if !ok || stored.Version != expected {
		return googlesheet.ErrStaleSheet
	}
	m.sheets[s.ID] = copySheet(s)
	return nil
}

func (m *memorySheets) SaveStatus(_ context.Context, s *googlesheet.Sheet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.sheets[s.ID]
	if !ok {
		return googlesheet.ErrSheetNotFound
	}
	stored.Status, stored.Reason, stored.ReasonDetail = s.Status, s.Reason, s.ReasonDetail
	return nil
}

func (m *memorySheets) UpdateTabName(_ context.Context, id, tab string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sheets[id].TabName = tab
	return nil
}

func (m *memorySheets) Get(_ context.Context, id string) (*googlesheet.Sheet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sheets[id]
	if !ok {
		return nil, googlesheet.ErrSheetNotFound
	}
	return copySheet(s), nil
}

func (m *memorySheets) FindInWorkspace(ctx context.Context, ws, id string) (*googlesheet.Sheet, error) {
	s, err := m.Get(ctx, id)
	if err != nil || s.WorkspaceID != ws {
		return nil, googlesheet.ErrSheetNotFound
	}
	return s, nil
}

func (m *memorySheets) List(context.Context, googlesheet.ListInput) (*shared.PaginatedResult[*googlesheet.Sheet], error) {
	return nil, errors.New("not used")
}

func (m *memorySheets) ListSpreadsheets(context.Context, googlesheet.ListInput) (*shared.PaginatedResult[googlesheet.Spreadsheet], error) {
	return nil, errors.New("not used")
}

func (m *memorySheets) Delete(_ context.Context, ws, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sheets, id)
	m.deleted = append(m.deleted, id)
	return nil
}

func (m *memorySheets) TouchWritten(_ context.Context, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sheets[id]; ok {
		s.LastWrittenAt = &at
	}
	return nil
}

type memoryRows struct {
	mu     sync.Mutex
	rows   map[string]*googlesheet.Row
	claims map[string]string
	seq    int
}

func newMemoryRows() *memoryRows {
	return &memoryRows{rows: map[string]*googlesheet.Row{}, claims: map[string]string{}}
}

func claimSlot(sheetID string, key googlesheet.UniqueKey) string {
	return sheetID + "|" + key.Period + "|" + key.Hash
}

func (m *memoryRows) Insert(ctx context.Context, row *googlesheet.Row) (*googlesheet.Row, bool, error) {
	inserted, err := m.InsertUnique(ctx, row, googlesheet.UniqueKey{})
	return inserted.Row, inserted.Duplicate, err
}

func (m *memoryRows) InsertUnique(_ context.Context, row *googlesheet.Row, unique googlesheet.UniqueKey) (googlesheet.Inserted, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.rows {
		if existing.SheetID == row.SheetID && existing.IdempotencyKey == row.IdempotencyKey {
			c := *existing
			return googlesheet.Inserted{Row: &c, Duplicate: true}, nil
		}
	}
	if holder, taken := m.claims[claimSlot(row.SheetID, unique)]; taken && !unique.Empty() {
		return googlesheet.Inserted{RepeatOf: holder}, nil
	}
	m.seq++
	row.ID = fmt.Sprintf("00000000-0000-4000-8000-%012d", m.seq)
	row.CreatedAt = row.CreatedAt.Add(time.Duration(m.seq) * time.Microsecond)
	c := *row
	m.rows[row.ID] = &c
	if !unique.Empty() {
		m.claims[claimSlot(row.SheetID, unique)] = row.ID
	}
	return googlesheet.Inserted{Row: row}, nil
}

func (m *memoryRows) LedgerPage(_ context.Context, sheetID string, after googlesheet.LedgerCursor, limit int) ([]*googlesheet.Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var page []*googlesheet.Row
	for _, r := range m.rows {
		if r.SheetID == sheetID {
			c := *r
			page = append(page, &c)
		}
	}
	sort.Slice(page, func(i, j int) bool { return page[i].CreatedAt.Before(page[j].CreatedAt) })
	start := 0
	for start < len(page) && after.ID != "" && !page[start].CreatedAt.After(after.CreatedAt) {
		start++
	}
	page = page[start:]
	if len(page) > limit {
		page = page[:limit]
	}
	return page, nil
}

func (m *memoryRows) ClaimUnique(_ context.Context, claims []googlesheet.UniqueClaim) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	claimed := 0
	for _, claim := range claims {
		slot := claimSlot(claim.Row.SheetID, claim.Key)
		if _, taken := m.claims[slot]; !taken {
			m.claims[slot] = claim.Row.ID
			claimed++
		}
	}
	return claimed, nil
}

func (m *memoryRows) ordered(sheetID string, statuses ...googlesheet.RowStatus) []*googlesheet.Row {
	var out []*googlesheet.Row
	for _, r := range m.rows {
		if r.SheetID != sheetID {
			continue
		}
		for _, s := range statuses {
			if r.Status == s {
				out = append(out, r)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (m *memoryRows) PendingSheets(_ context.Context, readyAt time.Time, limit int) ([]googlesheet.PendingSheet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]bool{}
	var out []googlesheet.PendingSheet
	for _, r := range m.rows {
		if seen[r.SheetID] || r.Status != googlesheet.RowPending {
			continue
		}
		seen[r.SheetID] = true
		head := m.ordered(r.SheetID, googlesheet.RowPending)[0]
		if !head.NextAttemptAt.After(readyAt) {
			out = append(out, googlesheet.PendingSheet{SheetID: r.SheetID, OldestPendingAt: head.CreatedAt})
		}
	}
	return out, nil
}

func (m *memoryRows) Claim(_ context.Context, sheetID, batchID string, limit int, now time.Time) ([]*googlesheet.Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pending := m.ordered(sheetID, googlesheet.RowPending)
	if len(pending) == 0 || pending[0].NextAttemptAt.After(now) {
		return nil, nil
	}
	if len(pending) > limit {
		pending = pending[:limit]
	}
	out := make([]*googlesheet.Row, len(pending))
	for i, r := range pending {
		r.Status, r.BatchID, r.Attempts = googlesheet.RowInFlight, batchID, r.Attempts+1
		claimedAt := now
		r.WrittenAt = nil
		r.NextAttemptAt = claimedAt
		c := *r
		out[i] = &c
	}
	return out, nil
}

func (m *memoryRows) set(ids []string, change func(*googlesheet.Row)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if r, ok := m.rows[id]; ok {
			change(r)
		}
	}
}

func (m *memoryRows) MarkWritten(_ context.Context, rows []googlesheet.WrittenRow, at time.Time) error {
	for _, w := range rows {
		number := w.RowNumber
		m.set([]string{w.ID}, func(r *googlesheet.Row) {
			r.Status, r.RowNumber, r.WrittenAt, r.ErrorCode = googlesheet.RowWritten, number, &at, ""
		})
	}
	return nil
}

func (m *memoryRows) Release(_ context.Context, ids []string, next time.Time, code string) error {
	m.set(ids, func(r *googlesheet.Row) { r.Status, r.NextAttemptAt, r.ErrorCode = googlesheet.RowPending, next, code })
	return nil
}

func (m *memoryRows) Unclaim(_ context.Context, ids []string) error {
	m.set(ids, func(r *googlesheet.Row) {
		r.Status = googlesheet.RowPending
		if r.Attempts > 0 {
			r.Attempts--
		}
	})
	return nil
}

func (m *memoryRows) Hold(_ context.Context, ids []string, code, detail string) error {
	m.set(ids, func(r *googlesheet.Row) { r.Status, r.ErrorCode, r.ErrorDetail = googlesheet.RowHeld, code, detail })
	return nil
}

func (m *memoryRows) Fail(_ context.Context, id, code, detail string) error {
	m.set([]string{id}, func(r *googlesheet.Row) { r.Status, r.ErrorCode, r.ErrorDetail = googlesheet.RowFailed, code, detail })
	return nil
}

func (m *memoryRows) RequeueProblems(_ context.Context, sheetID string, now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.ordered(sheetID, googlesheet.RowHeld, googlesheet.RowFailed) {
		r.Status, r.Attempts, r.NextAttemptAt = googlesheet.RowPending, 0, now
		n++
	}
	return n, nil
}

func (m *memoryRows) StuckInFlight(_ context.Context, sheetID string, before time.Time) ([]*googlesheet.Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*googlesheet.Row
	for _, r := range m.ordered(sheetID, googlesheet.RowInFlight) {
		if r.NextAttemptAt.Before(before) {
			c := *r
			out = append(out, &c)
		}
	}
	return out, nil
}

func (m *memoryRows) PurgeWritten(context.Context, time.Time, int) (int, error) { return 0, nil }

func (m *memoryRows) DeleteForLead(context.Context, string, string) (int, error) { return 0, nil }

func (m *memoryRows) DeleteUnwritten(_ context.Context, sheetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, r := range m.rows {
		if r.SheetID == sheetID && r.Status != googlesheet.RowWritten {
			delete(m.rows, id)
			for slot, holder := range m.claims {
				if holder == id {
					delete(m.claims, slot)
				}
			}
		}
	}
	return nil
}

func (m *memoryRows) Health(_ context.Context, sheetID string, _ int) (googlesheet.Health, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := googlesheet.Health{}
	for _, r := range m.rows {
		if r.SheetID != sheetID {
			continue
		}
		switch r.Status {
		case googlesheet.RowPending:
			h.Pending++
		case googlesheet.RowInFlight:
			h.InFlight++
		case googlesheet.RowHeld:
			h.Held++
		case googlesheet.RowFailed:
			h.Failed++
		case googlesheet.RowWritten:
			h.Written++
		}
	}
	return h, nil
}

func (m *memoryRows) byStatus(sheetID string, status googlesheet.RowStatus) []*googlesheet.Row {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ordered(sheetID, status)
}

type appendCall struct {
	tab  int64
	at   int
	refs []string
	rows [][]any
}

type appliedLayout struct {
	target googlesheet.Target
	layout googlesheet.Layout
}

type fakeAPI struct {
	mu            sync.Mutex
	header        []any
	headers       map[int64][]any
	body          map[int64][][]any
	appends       []appendCall
	appendErrs    []error
	rejectRef     map[string]bool
	repairs       []googlesheet.HeaderRepair
	repairIgnored bool
	workbook      googlesheet.Workbook
	layouts       []appliedLayout
	added         []googlesheet.TabSpec
	addErr        error
	deleted       []int64
	deletedRows   []int
	deleteErr     error
	readErr       error
	createErr     error
	created       googlesheet.CreatedSpreadsheet
	specs         []googlesheet.SpreadsheetSpec
	renamed       []string
	renameErr     error
	trashed       []string
	applied       int
	readCalls     int
	workbookCalls int
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		rejectRef: map[string]bool{}, headers: map[int64][]any{}, body: map[int64][][]any{},
		workbook: googlesheet.Workbook{Tabs: []googlesheet.TabInfo{{ID: 0, Title: "Pedidos", Rows: 1000, Columns: 10}}},
	}
}

func (f *fakeAPI) grid(tab int64) [][]any {
	first, ok := f.headers[tab]
	if !ok {
		first = f.header
	}
	return append([][]any{first}, f.body[tab]...)
}

func (f *fakeAPI) store(tab int64, grid [][]any) {
	f.headers[tab] = grid[0]
	f.body[tab] = grid[1:]
}

func (f *fakeAPI) addRows(tab int64, rows ...[]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.body[tab] = append(f.body[tab], rows...)
}

func (f *fakeAPI) setHeader(tab int64, header []any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.headers[tab] = header
}

func (f *fakeAPI) CreateSpreadsheet(_ context.Context, _ string, spec googlesheet.SpreadsheetSpec) (googlesheet.CreatedSpreadsheet, error) {
	f.specs = append(f.specs, spec)
	return f.created, f.createErr
}

func (f *fakeAPI) RenameSpreadsheet(_ context.Context, _, spreadsheetID, title string) error {
	if f.renameErr != nil {
		return f.renameErr
	}
	f.renamed = append(f.renamed, spreadsheetID+"="+title)
	return nil
}

func (f *fakeAPI) TrashSpreadsheet(_ context.Context, _, id string) error {
	f.trashed = append(f.trashed, id)
	return nil
}

func (f *fakeAPI) ApplyLayout(_ context.Context, _ string, target googlesheet.Target, layout googlesheet.Layout) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied++
	f.layouts = append(f.layouts, appliedLayout{target: target, layout: layout})
	f.headers[target.TabID] = layoutHeader(layout)
	return nil
}

func (f *fakeAPI) RepairHeader(_ context.Context, _ string, target googlesheet.Target, repair googlesheet.HeaderRepair, layout googlesheet.Layout) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.repairs = append(f.repairs, repair)
	if f.repairIgnored {
		return nil
	}
	grid := f.grid(target.TabID)
	switch {
	case repair.MoveFrom > 1:
		header := grid[repair.MoveFrom-1]
		rest := append(append([][]any{}, grid[:repair.MoveFrom-1]...), grid[repair.MoveFrom:]...)
		grid = append([][]any{header}, rest...)
	case repair.Insert:
		grid = append([][]any{{}}, grid...)
	}
	grid[0] = layoutHeader(layout)
	f.store(target.TabID, grid)
	f.layouts = append(f.layouts, appliedLayout{target: target, layout: layout})
	return nil
}

func layoutHeader(layout googlesheet.Layout) []any {
	header := []any{googlesheet.RefHeader}
	for _, col := range layout.Columns {
		header = append(header, col.Header)
	}
	return header
}

func (f *fakeAPI) AddTab(_ context.Context, _, _ string, tab googlesheet.TabSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, tab)
	f.headers[tab.ID] = layoutHeader(tab.Layout)
	f.workbook.Tabs = append(f.workbook.Tabs, googlesheet.TabInfo{ID: tab.ID, Title: tab.Title, Rows: googlesheet.NewTabRows, Columns: len(tab.Layout.Columns) + 1})
	return nil
}

func (f *fakeAPI) DeleteTab(_ context.Context, _, _ string, tabID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, tabID)
	return nil
}

var (
	errInvalidRow  = errors.New("invalid row")
	errOverHeader  = errors.New("a write at row 1 pushes the header down")
	rowRangeFormat = regexp.MustCompile(`^(\d+):(\d+)$`)
)

func (f *fakeAPI) InsertRows(_ context.Context, _ string, target googlesheet.Target, at int, rows [][]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	refs := make([]string, len(rows))
	for i, row := range rows {
		refs[i] = row[0].(string)
	}
	f.appends = append(f.appends, appendCall{tab: target.TabID, at: at, refs: refs, rows: rows})
	if len(f.appendErrs) > 0 {
		err := f.appendErrs[0]
		f.appendErrs = f.appendErrs[1:]
		if err != nil {
			return err
		}
	}
	for _, ref := range refs {
		if f.rejectRef[ref] {
			return errInvalidRow
		}
	}
	if at < 2 {
		return errOverHeader
	}
	grid := f.grid(target.TabID)
	for len(grid) < at-1 {
		grid = append(grid, []any{})
	}
	grid = append(grid[:at-1], append(append([][]any{}, rows...), grid[at-1:]...)...)
	f.store(target.TabID, grid)
	return nil
}

func (f *fakeAPI) DeleteRow(_ context.Context, _ string, target googlesheet.Target, row int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletedRows = append(f.deletedRows, row)
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if row < 2 {
		return errOverHeader
	}
	grid := f.grid(target.TabID)
	if row > len(grid) {
		return nil
	}
	f.store(target.TabID, append(append([][]any{}, grid[:row-1]...), grid[row:]...))
	return nil
}

func trimmedRow(row []any) []any {
	end := len(row)
	for end > 0 && (row[end-1] == nil || row[end-1] == "") {
		end--
	}
	return row[:end]
}

func (f *fakeAPI) read(tab int64, a1 string) [][]any {
	grid := f.grid(tab)
	var out [][]any
	if a1 == googlesheet.RefColumnRange {
		for _, row := range grid {
			out = append(out, trimmedRow(append([]any{}, row[:min(len(row), 1)]...)))
		}
	} else if match := rowRangeFormat.FindStringSubmatch(a1); match != nil {
		row, _ := strconv.Atoi(match[1])
		if row <= len(grid) {
			out = [][]any{trimmedRow(grid[row-1])}
		}
	}
	end := len(out)
	for end > 0 && len(out[end-1]) == 0 {
		end--
	}
	return out[:end]
}

func (f *fakeAPI) ReadRange(_ context.Context, _ string, target googlesheet.Target, a1 string) ([][]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readCalls++
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.read(target.TabID, a1), nil
}

func (f *fakeAPI) ReadRanges(_ context.Context, _ string, target googlesheet.Target, ranges []string) ([][][]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readCalls++
	if f.readErr != nil {
		return nil, f.readErr
	}
	out := make([][][]any, len(ranges))
	for i, a1 := range ranges {
		out[i] = f.read(target.TabID, a1)
	}
	return out, nil
}

func (f *fakeAPI) Workbook(context.Context, string, string) (googlesheet.Workbook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workbookCalls++
	return googlesheet.Workbook{Tabs: append([]googlesheet.TabInfo(nil), f.workbook.Tabs...)}, f.readErr
}

func (f *fakeAPI) written() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	tabs := make([]int64, 0, len(f.body))
	for tab := range f.body {
		tabs = append(tabs, tab)
	}
	sort.Slice(tabs, func(i, j int) bool { return tabs[i] < tabs[j] })
	var out []string
	for _, tab := range tabs {
		for _, row := range f.grid(tab) {
			if len(row) == 0 {
				continue
			}
			if ref, ok := row[0].(string); ok && ref != "" && ref != googlesheet.RefHeader {
				out = append(out, ref)
			}
		}
	}
	return out
}

var (
	errRateLimited = errors.New("429")
	errTransient   = errors.New("503")
	errForbidden   = errors.New("403")
	errAuth        = errors.New("401")
)

type fakeClassifier struct{}

func (fakeClassifier) Classify(err error) googlesheet.Failure {
	switch {
	case errors.Is(err, errRateLimited):
		return googlesheet.Failure{Kind: googlesheet.FailureRateLimited, RetryAfter: 30 * time.Second}
	case errors.Is(err, errForbidden):
		return googlesheet.Failure{Kind: googlesheet.FailurePermission, Detail: "no access"}
	case errors.Is(err, errAuth):
		return googlesheet.Failure{Kind: googlesheet.FailureAuth}
	case errors.Is(err, errInvalidRow):
		return googlesheet.Failure{Kind: googlesheet.FailureInvalid, Detail: "bad value"}
	}
	return googlesheet.Failure{Kind: googlesheet.FailureTransient, Detail: "unavailable"}
}

type fakeAccess struct {
	access googleaccount.Access
	err    error
}

func (f *fakeAccess) Access(context.Context, string, googleaccount.Product) (googleaccount.Access, error) {
	return f.access, f.err
}

type fakeLimiter struct {
	mu       sync.Mutex
	writes   int
	reads    int
	pressure float64
	err      error
}

func (l *fakeLimiter) TakeWrite(context.Context, string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return false, l.err
	}
	if l.writes == 0 {
		return false, nil
	}
	l.writes--
	return true, nil
}

func (l *fakeLimiter) TakeRead(context.Context, string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return false, l.err
	}
	if l.reads == 0 {
		return false, nil
	}
	l.reads--
	return true, nil
}

func (l *fakeLimiter) WritePressure(context.Context) (float64, error) { return l.pressure, nil }

type fakeLeases struct {
	mu   sync.Mutex
	held map[string]bool
}

func (l *fakeLeases) Acquire(ctx context.Context, key string) (func() (bool, error), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held[key] {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	l.held[key] = true
	return func() (bool, error) {
		l.mu.Lock()
		defer l.mu.Unlock()
		delete(l.held, key)
		return true, nil
	}, nil
}

type fakeMarks struct {
	mu   sync.Mutex
	keys map[string]bool
}

func (m *fakeMarks) SetNX(key, _ string, _ time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keys[key] {
		return false, nil
	}
	m.keys[key] = true
	return true, nil
}

func (m *fakeMarks) Del(keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.keys, k)
	}
	return nil
}

type fakeAlerts struct {
	mu        sync.Mutex
	attention []googlesheet.AttentionReason
	resolved  int
}

func (a *fakeAlerts) NeedsAttention(_ context.Context, s *googlesheet.Sheet) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.attention = append(a.attention, s.Reason)
	return nil
}

func (a *fakeAlerts) Resolved(context.Context, *googlesheet.Sheet) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.resolved++
	return nil
}

type fakeMetrics struct{}

func (fakeMetrics) AddGoogleSheetsEvent(string, string)   {}
func (fakeMetrics) ObserveGoogleSheetsBatch(int, float64) {}

type recordingNudger struct {
	mu     sync.Mutex
	sheets []string
}

func (n *recordingNudger) Nudge(id string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sheets = append(n.sheets, id)
}

type fakeResolver struct {
	values googlesheet.SystemValues
	calls  int
}

func (r *fakeResolver) Resolve(context.Context, string, googlesheet.Origin) (googlesheet.SystemValues, error) {
	r.calls++
	return r.values, nil
}

func (m *memoryRows) ListRows(_ context.Context, workspaceID, sheetID string, p shared.Pagination) (*shared.PaginatedResult[*googlesheet.Row], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var rows []*googlesheet.Row
	for _, row := range m.rows {
		if row.WorkspaceID == workspaceID && row.SheetID == sheetID {
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	total := len(rows)
	start := min(p.Offset(), total)
	end := min(start+p.PageSize, total)
	return shared.NewPaginatedResult(rows[start:end], p, int64(total)), nil
}
