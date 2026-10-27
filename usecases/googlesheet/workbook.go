package googlesheet

import (
	"context"
	"errors"
	"fmt"

	"vozko/domain/googleaccount"
	"vozko/domain/googlesheet"
)

type WorkbookSheet struct {
	Ref      string
	Input    CreateInput
	LinkRefs map[int]string
}

type WorkbookInput struct {
	WorkspaceID   string
	UserID        string
	SpreadsheetID string
	Sheets        []WorkbookSheet
}

type WorkbookError struct {
	Sheet int
	Err   error
}

func (e *WorkbookError) Error() string {
	return fmt.Sprintf("googlesheet: sheet %d of the workbook: %v", e.Sheet, e.Err)
}

func (e *WorkbookError) Unwrap() error { return e.Err }

func (m *Manager) CreateWorkbook(ctx context.Context, in WorkbookInput) ([]*googlesheet.Sheet, error) {
	if len(in.Sheets) == 0 {
		return nil, &WorkbookError{Sheet: 0, Err: &googlesheet.DefinitionError{Code: googlesheet.CodeNoColumns, Field: googlesheet.FieldColumns, Column: -1}}
	}
	access, err := m.access.Access(ctx, in.WorkspaceID, googleaccount.ProductSheets)
	if err != nil {
		return nil, err
	}
	spreadsheetID := in.SpreadsheetID
	made := map[string]string{}
	var created []*googlesheet.Sheet
	var undos []undo
	rollback := func() {
		for i := len(undos) - 1; i >= 0; i-- {
			undos[i](context.WithoutCancel(ctx))
		}
	}
	for i, item := range in.Sheets {
		input := item.Input
		input.WorkspaceID, input.UserID, input.SpreadsheetID = in.WorkspaceID, in.UserID, spreadsheetID
		columns, err := resolveLinkRefs(input.Columns, item.LinkRefs, made)
		if err != nil {
			rollback()
			return nil, &WorkbookError{Sheet: i, Err: err}
		}
		input.Columns = columns
		sheet, undoSheet, err := m.create(ctx, access, input)
		if err != nil {
			rollback()
			return nil, &WorkbookError{Sheet: i, Err: err}
		}
		if spreadsheetID == "" {
			spreadsheetID = sheet.SpreadsheetID
		}
		undos = append(undos, undoSheet)
		if item.Ref != "" {
			made[item.Ref] = sheet.ID
		}
		created = append(created, sheet)
	}
	return created, nil
}

func resolveLinkRefs(columns []googlesheet.ColumnInput, refs map[int]string, made map[string]string) ([]googlesheet.ColumnInput, error) {
	out := append([]googlesheet.ColumnInput(nil), columns...)
	for index, ref := range refs {
		if index < 0 || index >= len(out) {
			return nil, fmt.Errorf("%w: a coluna %d não existe", googlesheet.ErrLinkTargetInvalid, index)
		}
		id, ok := made[ref]
		if !ok {
			return nil, fmt.Errorf("%w: coluna %q: %q precisa vir antes nesta planilha", googlesheet.ErrLinkTargetInvalid, out[index].Header, ref)
		}
		out[index].LinkSheetID = id
	}
	return out, nil
}

type WorkbookTab struct {
	Sheet *googlesheet.Sheet
	Tabs  []googlesheet.Tab
}

type WorkbookView struct {
	SpreadsheetID string
	Title         string
	URL           string
	Usable        bool
	Sheets        []WorkbookTab
}

func (m *Manager) Workbooks(ctx context.Context, workspaceID string) ([]WorkbookView, error) {
	sheets, err := m.sheets.ListAll(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	account := ""
	access, err := m.access.Access(ctx, workspaceID, googleaccount.ProductSheets)
	switch {
	case err == nil:
		account = access.AccountID
	case !errors.Is(err, googleaccount.ErrNotConnected):
		return nil, err
	}
	var out []WorkbookView
	for _, file := range googlesheet.GroupBySpreadsheet(sheets) {
		view := WorkbookView{
			SpreadsheetID: file.ID,
			Title:         file.Title(),
			URL:           file.URL(),
			Usable:        account != "" && file.Oldest().AccountID == account,
		}
		for _, sheet := range file.Sheets {
			tabs, err := m.sheets.Tabs(ctx, sheet.ID)
			if err != nil {
				return nil, err
			}
			view.Sheets = append(view.Sheets, WorkbookTab{Sheet: sheet, Tabs: tabs})
		}
		out = append(out, view)
	}
	return out, nil
}

func (m *Manager) PreviewTabName(pattern, timeZone string) (string, string, error) {
	return googlesheet.PreviewTabTitle(pattern, timeZone, m.now())
}
