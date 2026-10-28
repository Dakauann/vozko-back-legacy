package googlesheet

import (
	"context"
	"errors"

	"vozko/domain/googleaccount"
	"vozko/domain/googlesheet"
	"vozko/domain/sheetfit"
)

type FitFilesInput struct {
	WorkspaceID        string
	ModelSpreadsheetID string
	Uses               []sheetfit.Use
}

type FileFit = sheetfit.Candidate

func (m *Manager) FitWorkspace(ctx context.Context, workspaceID string) (sheetfit.Workspace, error) {
	sheets, err := m.sheets.ListAll(ctx, workspaceID)
	if err != nil {
		return sheetfit.Workspace{}, err
	}
	files := googlesheet.GroupBySpreadsheet(sheets)
	if err := m.nameFiles(ctx, workspaceID, files); err != nil {
		return sheetfit.Workspace{}, err
	}
	out := sheetfit.Workspace{Files: files}
	access, err := m.access.Access(ctx, workspaceID, googleaccount.ProductSheets)
	switch {
	case err == nil:
		out.Account = access.AccountID
	case !errors.Is(err, googleaccount.ErrNotConnected):
		return sheetfit.Workspace{}, err
	}
	return out, nil
}

func (m *Manager) FitFiles(ctx context.Context, in FitFilesInput) ([]FileFit, error) {
	ws, err := m.FitWorkspace(ctx, in.WorkspaceID)
	if err != nil {
		return nil, err
	}
	model, err := modelOf(ws.Files, in.ModelSpreadsheetID, in.Uses)
	if err != nil {
		return nil, err
	}
	if ws.Account == "" {
		return nil, googleaccount.ErrNotConnected
	}
	return sheetfit.Candidates(ws, model)
}

func modelOf(files []googlesheet.Spreadsheet, spreadsheetID string, uses []sheetfit.Use) (sheetfit.Model, error) {
	for _, file := range files {
		if file.ID != spreadsheetID {
			continue
		}
		model := sheetfit.Model{Sheets: file.Sheets, Uses: uses}
		if err := model.Check(); err != nil {
			return sheetfit.Model{}, err
		}
		return model, nil
	}
	return sheetfit.Model{}, googlesheet.ErrSpreadsheetNotFound
}

type CopyModelInput struct {
	WorkspaceID        string
	UserID             string
	ModelSpreadsheetID string
	FileTitle          string
}

type ModelCopy struct {
	File googlesheet.Spreadsheet
	Tabs map[string]string
}

func (m *Manager) CopyModel(ctx context.Context, in CopyModelInput) (*ModelCopy, error) {
	title, err := googlesheet.CheckFileTitle(in.FileTitle)
	if err != nil {
		return nil, err
	}
	hosts, err := m.sheets.InSpreadsheet(ctx, in.WorkspaceID, in.ModelSpreadsheetID)
	if err != nil {
		return nil, err
	}
	if len(hosts) == 0 {
		return nil, googlesheet.ErrSpreadsheetNotFound
	}
	ordered, err := googlesheet.CopyOrder(hosts)
	if err != nil {
		return nil, err
	}
	workbook := WorkbookInput{WorkspaceID: in.WorkspaceID, UserID: in.UserID, FileTitle: title}
	for _, sheet := range ordered {
		workbook.Sheets = append(workbook.Sheets, copyOf(sheet))
	}
	created, err := m.CreateWorkbook(ctx, workbook)
	if err != nil {
		return nil, err
	}
	out := &ModelCopy{File: googlesheet.GroupBySpreadsheet(created)[0], Tabs: map[string]string{}}
	out.File.Name = title
	for i, sheet := range ordered {
		out.Tabs[sheet.ID] = created[i].ID
	}
	return out, nil
}

func copyOf(sheet *googlesheet.Sheet) WorkbookSheet {
	copied := sheet.Copy()
	tabName := sheet.TabName
	if sheet.Periodic() {
		tabName = ""
	}
	return WorkbookSheet{
		Ref: sheet.ID,
		Input: CreateInput{
			Title: sheet.Title, TabName: tabName, TabPeriod: sheet.TabPeriod, TabPattern: sheet.TabPattern, Style: sheet.Style,
			Description: sheet.Description, TimeZone: sheet.TimeZone, Columns: copied.Columns, Unique: copied.Unique,
		},
		LinkRefs: copied.LinkRefs,
	}
}
