package googlesheet

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"

	"vozko/domain/googleaccount"
	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

const healthProblemRows = 50

func (m *Manager) ListRows(ctx context.Context, workspaceID, id string, pagination shared.Pagination) (*shared.PaginatedResult[RowView], error) {
	sheet, err := m.sheets.FindInWorkspace(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	pagination = shared.NormalizePagination(pagination)
	page, err := m.rows.ListRows(ctx, workspaceID, id, pagination)
	if err != nil {
		return nil, err
	}
	for _, row := range page.Items {
		row.Cells = sheet.DisplayCells(row.Cells)
	}
	views, err := m.names.views(ctx, workspaceID, page.Items)
	if err != nil {
		return nil, err
	}
	return &shared.PaginatedResult[RowView]{Items: views, Page: page.Page, PageSize: page.PageSize, TotalItems: page.TotalItems, TotalPages: page.TotalPages}, nil
}

type ManagerDeps struct {
	Sheets  googlesheet.SheetRepository
	Rows    googlesheet.RowRepository
	API     googlesheet.SheetsAPI
	Errors  googlesheet.ErrorClassifier
	Access  googleaccount.AccessSource
	Limiter googlesheet.Limiter
	Marks   Marker
	Alerts  Alerts
	Nudger  Nudger
	Names   OriginNames
	Edits   googlesheet.EditSessionRepository
	Usage   CampaignUsage
}

type CampaignUsage interface {
	CampaignsUsing(ctx context.Context, workspaceID, spreadsheetID string) ([]string, error)
}

type Manager struct {
	now     Clock
	seed    func() int64
	sheets  googlesheet.SheetRepository
	rows    googlesheet.RowRepository
	api     googlesheet.SheetsAPI
	errs    googlesheet.ErrorClassifier
	access  googleaccount.AccessSource
	limiter googlesheet.Limiter
	marks   Marker
	alerts  Alerts
	nudger  Nudger
	names   OriginNames
	edits   editGate
	usage   CampaignUsage
}

func NewManager(d ManagerDeps) *Manager {
	if d.Sheets == nil || d.Rows == nil || d.API == nil || d.Errors == nil || d.Access == nil || d.Limiter == nil || d.Marks == nil || d.Alerts == nil || d.Nudger == nil || !d.Names.complete() || d.Edits == nil || d.Usage == nil {
		panic("googlesheet: the manager requires sheets, rows, the sheets api, an error classifier, google access, a limiter, marks, alerts, a nudger, the agent, workflow and people names, the edit sessions and the campaign usage")
	}
	return &Manager{
		sheets: d.Sheets, rows: d.Rows, api: d.API, errs: d.Errors, access: d.Access,
		limiter: d.Limiter, marks: d.Marks, alerts: d.Alerts, nudger: d.Nudger, names: d.Names, edits: editGate{sessions: d.Edits}, usage: d.Usage, now: utcNow, seed: rand.Int64,
	}
}

type CreateInput struct {
	WorkspaceID   string
	UserID        string
	SpreadsheetID string
	FileTitle     string
	Title         string
	TabName       string
	TabPeriod     googlesheet.TabPeriod
	TabPattern    string
	Style         googlesheet.StylePreset
	Description   string
	TimeZone      string
	Columns       []googlesheet.ColumnInput
	Unique        googlesheet.UniqueInput
}

func (m *Manager) Create(ctx context.Context, in CreateInput) (*googlesheet.Sheet, error) {
	access, err := m.access.Access(ctx, in.WorkspaceID, googleaccount.ProductSheets)
	if err != nil {
		return nil, err
	}
	sheet, _, err := m.create(ctx, access, in)
	return sheet, err
}

type undo func(ctx context.Context)

func (m *Manager) create(ctx context.Context, access googleaccount.Access, in CreateInput) (*googlesheet.Sheet, undo, error) {
	sheet, err := googlesheet.NewSheet(googlesheet.NewSheetInput{
		WorkspaceID: in.WorkspaceID,
		AccountID:   access.AccountID,
		Title:       in.Title,
		TabName:     in.TabName,
		TabPeriod:   in.TabPeriod,
		TabPattern:  in.TabPattern,
		Style:       in.Style,
		Description: in.Description,
		TimeZone:    in.TimeZone,
		CreatedBy:   in.UserID,
		Columns:     in.Columns,
		Unique:      in.Unique,
	})
	if err != nil {
		return nil, nil, err
	}
	if err := sheet.OpenPeriod(m.now()); err != nil {
		return nil, nil, err
	}
	newFile := in.SpreadsheetID == ""
	var fileTitle string
	if newFile {
		if fileTitle, err = googlesheet.CheckFileTitle(in.FileTitle); err != nil {
			return nil, nil, err
		}
	}
	var google undo
	if newFile {
		google, err = m.newSpreadsheet(ctx, access, sheet, fileTitle)
	} else {
		google, err = m.newTab(ctx, access, sheet, in.SpreadsheetID)
	}
	if err != nil {
		return nil, nil, err
	}
	if err := m.sheets.Create(ctx, sheet); err != nil {
		google(ctx)
		return nil, nil, err
	}
	if newFile {
		if err := m.sheets.SaveFileTitle(ctx, sheet.WorkspaceID, sheet.SpreadsheetID, fileTitle); err != nil {
			if err := m.sheets.Delete(ctx, sheet.WorkspaceID, sheet.ID); err != nil {
				log.Printf("[googlesheet] sheet %s could not be removed after its file name failed to save: %v", sheet.ID, err)
			}
			google(ctx)
			return nil, nil, err
		}
	}
	return sheet, func(ctx context.Context) {
		if err := m.sheets.Delete(ctx, sheet.WorkspaceID, sheet.ID); err != nil {
			log.Printf("[googlesheet] sheet %s could not be removed while undoing a workbook: %v", sheet.ID, err)
		}
		google(ctx)
	}, nil
}

func (m *Manager) newSpreadsheet(ctx context.Context, access googleaccount.Access, sheet *googlesheet.Sheet, fileTitle string) (undo, error) {
	if err := m.checkLinks(ctx, sheet); err != nil {
		return nil, err
	}
	if err := m.takeWrite(ctx, access); err != nil {
		return nil, err
	}
	created, err := m.api.CreateSpreadsheet(ctx, access.Token, sheet.Spec(fileTitle))
	if err != nil {
		if created.SpreadsheetID != "" {
			m.trash(ctx, access, created.SpreadsheetID)
		}
		return nil, m.googleRefusal(err)
	}
	sheet.SpreadsheetID, sheet.TabID = created.SpreadsheetID, created.TabID
	return func(ctx context.Context) { m.trash(ctx, access, created.SpreadsheetID) }, nil
}

func (m *Manager) newTab(ctx context.Context, access googleaccount.Access, sheet *googlesheet.Sheet, spreadsheetID string) (undo, error) {
	hosts, err := m.sheets.InSpreadsheet(ctx, sheet.WorkspaceID, spreadsheetID)
	if err != nil {
		return nil, err
	}
	if len(hosts) == 0 {
		return nil, googlesheet.ErrSpreadsheetNotFound
	}
	if hosts[0].AccountID != access.AccountID {
		return nil, googlesheet.ErrAccountChanged
	}
	if err := m.editFree(ctx, sheet.WorkspaceID, spreadsheetID); err != nil {
		return nil, err
	}
	sheet.SpreadsheetID = spreadsheetID
	if err := m.checkLinks(ctx, sheet); err != nil {
		return nil, err
	}
	if err := m.takeRead(ctx, access); err != nil {
		return nil, err
	}
	workbook, err := m.api.Workbook(ctx, access.Token, spreadsheetID)
	if err != nil {
		return nil, m.googleRefusal(err)
	}
	if _, taken := workbook.Titled(sheet.TabName); taken {
		return nil, fmt.Errorf("%w: %q", googlesheet.ErrTabNameTaken, sheet.TabName)
	}
	if workbook.NearlyFull(googlesheet.NewTabRows * (len(sheet.Columns) + 1)) {
		return nil, fmt.Errorf("%w: %s", googlesheet.ErrSheetNotActive, googlesheet.ReasonSheetFull)
	}
	if err := m.takeWrite(ctx, access); err != nil {
		return nil, err
	}
	tabID := workbook.FreeTabID(m.seed())
	if err := m.api.AddTab(ctx, access.Token, spreadsheetID, googlesheet.TabSpec{ID: tabID, Title: sheet.TabName, Layout: sheet.Layout(0)}); err != nil {
		return nil, m.googleRefusal(err)
	}
	sheet.TabID = tabID
	return func(ctx context.Context) {
		if err := m.api.DeleteTab(ctx, access.Token, spreadsheetID, tabID); err != nil {
			log.Printf("[googlesheet] tab %d of %s was created but not recorded and could not be removed: %v", tabID, spreadsheetID, err)
		}
	}, nil
}

func (m *Manager) checkLinks(ctx context.Context, sheet *googlesheet.Sheet, only ...googlesheet.Column) error {
	columns := only
	if len(columns) == 0 {
		columns = sheet.LinkColumns()
	}
	for _, col := range columns {
		target, err := m.sheets.FindInWorkspace(ctx, sheet.WorkspaceID, col.LinkSheetID)
		if errors.Is(err, googlesheet.ErrSheetNotFound) {
			target = nil
		} else if err != nil {
			return err
		}
		if err := googlesheet.CheckLinkTarget(sheet, col, target); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) trash(ctx context.Context, access googleaccount.Access, spreadsheetID string) {
	if err := m.api.TrashSpreadsheet(ctx, access.Token, spreadsheetID); err != nil {
		log.Printf("[googlesheet] spreadsheet %s was created but not recorded and could not be moved to the trash: %v", spreadsheetID, err)
	}
}

func (m *Manager) Get(ctx context.Context, workspaceID, id string) (*googlesheet.Sheet, error) {
	return m.sheets.FindInWorkspace(ctx, workspaceID, id)
}

func (m *Manager) List(ctx context.Context, in googlesheet.ListInput) (*shared.PaginatedResult[*googlesheet.Sheet], error) {
	return m.sheets.List(ctx, in)
}

func (m *Manager) ListSpreadsheets(ctx context.Context, in googlesheet.ListInput) (*shared.PaginatedResult[googlesheet.Spreadsheet], error) {
	return m.sheets.ListSpreadsheets(ctx, in)
}

type DetailsInput struct {
	WorkspaceID string
	ID          string
	Version     int
	Title       *string
	Description *string
	Unique      *googlesheet.UniqueRule
}

func (m *Manager) UpdateDetails(ctx context.Context, in DetailsInput) (*googlesheet.Sheet, error) {
	sheet, err := m.sheets.FindInWorkspace(ctx, in.WorkspaceID, in.ID)
	if err != nil {
		return nil, err
	}
	if in.Version != sheet.Version {
		return nil, googlesheet.ErrStaleSheet
	}
	if in.Title != nil || in.Description != nil || in.Unique == nil {
		if err := sheet.Rename(in.Title, in.Description); err != nil {
			return nil, err
		}
	}
	if in.Unique != nil {
		if err := m.changeUnique(ctx, sheet, *in.Unique); err != nil {
			return nil, err
		}
	}
	if err := m.sheets.Update(ctx, sheet, in.Version); err != nil {
		return nil, err
	}
	return sheet, nil
}

type ColumnsInput struct {
	WorkspaceID string
	ID          string
	Version     int
	Edit        googlesheet.ColumnsEdit
}

func (m *Manager) EditColumns(ctx context.Context, in ColumnsInput) (*googlesheet.Sheet, error) {
	sheet, err := m.sheets.FindInWorkspace(ctx, in.WorkspaceID, in.ID)
	if err != nil {
		return nil, err
	}
	if in.Version != sheet.Version {
		return nil, googlesheet.ErrStaleSheet
	}
	before := len(sheet.Columns)
	if err := sheet.EditColumns(in.Edit); err != nil {
		return nil, err
	}
	var added []googlesheet.Column
	for _, col := range sheet.Columns[before:] {
		if col.Source == googlesheet.SourceExportLink {
			added = append(added, col)
		}
	}
	if len(added) > 0 {
		if err := m.checkLinks(ctx, sheet, added...); err != nil {
			return nil, err
		}
	}
	return m.saveLayout(ctx, sheet, in.Version, before)
}

type LayoutInput struct {
	WorkspaceID string
	ID          string
	Version     int
	Style       *googlesheet.StylePreset
	TabPattern  *string
}

func (m *Manager) UpdateLayout(ctx context.Context, in LayoutInput) (*googlesheet.Sheet, error) {
	sheet, err := m.sheets.FindInWorkspace(ctx, in.WorkspaceID, in.ID)
	if err != nil {
		return nil, err
	}
	if in.Version != sheet.Version {
		return nil, googlesheet.ErrStaleSheet
	}
	if err := sheet.ChangeLayout(in.Style, in.TabPattern); err != nil {
		return nil, err
	}
	return m.saveLayout(ctx, sheet, in.Version, len(sheet.Columns))
}

func (m *Manager) saveLayout(ctx context.Context, sheet *googlesheet.Sheet, version, sizeFrom int) (*googlesheet.Sheet, error) {
	if err := m.editFree(ctx, sheet.WorkspaceID, sheet.SpreadsheetID); err != nil {
		return nil, err
	}
	access, err := m.sheetAccess(ctx, sheet)
	if err != nil {
		return nil, err
	}
	if err := m.takeWrite(ctx, access); err != nil {
		return nil, err
	}
	if err := m.api.ApplyLayout(ctx, access.Token, sheet.Target(), sheet.Layout(sizeFrom)); err != nil {
		return nil, m.googleRefusal(err)
	}
	if err := m.sheets.Update(ctx, sheet, version); err != nil {
		return nil, err
	}
	m.forgetTabCheck(sheet.ID)
	return sheet, nil
}

func (m *Manager) Delete(ctx context.Context, workspaceID, id string) error {
	sheet, err := m.sheets.FindInWorkspace(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	campaigns, err := m.usage.CampaignsUsing(ctx, workspaceID, sheet.SpreadsheetID)
	if err != nil {
		return err
	}
	if len(campaigns) > 0 {
		return &googlesheet.InUseError{Campaigns: campaigns}
	}
	if err := m.sheets.Delete(ctx, workspaceID, id); err != nil {
		return err
	}
	return m.rows.DeleteUnwritten(ctx, id)
}

type HealthView struct {
	Sheet   *googlesheet.Sheet
	Health  googlesheet.Health
	Editing *EditView
}

func (m *Manager) Health(ctx context.Context, workspaceID, id string) (*HealthView, error) {
	sheet, err := m.sheets.FindInWorkspace(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	health, err := m.rows.Health(ctx, sheet.ID, healthProblemRows)
	if err != nil {
		return nil, err
	}
	editing, err := m.editing(ctx, sheet.WorkspaceID, sheet.SpreadsheetID)
	if err != nil {
		return nil, err
	}
	return &HealthView{Sheet: sheet, Health: health, Editing: editing}, nil
}

func (m *Manager) RetryProblems(ctx context.Context, workspaceID, id string) (int, error) {
	sheet, err := m.sheets.FindInWorkspace(ctx, workspaceID, id)
	if err != nil {
		return 0, err
	}
	if err := sheet.Writable(); err != nil {
		return 0, err
	}
	requeued, err := m.rows.RequeueProblems(ctx, sheet.ID, m.now())
	if err != nil {
		return 0, err
	}
	if requeued > 0 {
		m.nudger.Nudge(sheet.ID)
	}
	return requeued, nil
}

type ReactivateInput struct {
	WorkspaceID  string
	ID           string
	RepairHeader bool
}

func (m *Manager) Reactivate(ctx context.Context, in ReactivateInput) (*googlesheet.Sheet, error) {
	sheet, err := m.sheets.FindInWorkspace(ctx, in.WorkspaceID, in.ID)
	if err != nil {
		return nil, err
	}
	if in.RepairHeader {
		if err := m.editFree(ctx, sheet.WorkspaceID, sheet.SpreadsheetID); err != nil {
			return nil, err
		}
	}
	access, err := m.sheetAccess(ctx, sheet)
	if err != nil {
		return nil, err
	}
	if err := m.takeRead(ctx, access); err != nil {
		return nil, err
	}
	workbook, err := m.api.Workbook(ctx, access.Token, sheet.SpreadsheetID)
	if err != nil {
		return nil, m.googleRefusal(err)
	}
	tab, ok := workbook.Tab(sheet.TabID)
	if !ok {
		return nil, &GoogleRefusal{Failure: googlesheet.Failure{Kind: googlesheet.FailureNotFound, Detail: tabGoneDetail}, Err: fmt.Errorf("googlesheet: tab %d of %s is gone", sheet.TabID, sheet.SpreadsheetID)}
	}
	renamed := tab.Title != sheet.TabName
	sheet.TabName = tab.Title
	if workbook.NearlyFull(0) {
		return nil, fmt.Errorf("%w: %s", googlesheet.ErrSheetNotActive, googlesheet.ReasonSheetFull)
	}
	if err := m.settleHeaders(ctx, access, sheet, workbook, in.RepairHeader); err != nil {
		return nil, err
	}
	sheet.Activate()
	if renamed {
		if err := m.sheets.UpdateTabName(ctx, sheet.ID, sheet.TabName); err != nil {
			return nil, err
		}
	}
	if err := m.sheets.SaveStatus(ctx, sheet); err != nil {
		return nil, err
	}
	m.forgetTabCheck(sheet.ID)
	if err := m.alerts.Resolved(ctx, sheet); err != nil {
		log.Printf("[googlesheet] attention alert of %s not resolved: %v", sheet.ID, err)
	}
	m.nudger.Nudge(sheet.ID)
	return sheet, nil
}

const tabGoneDetail = "a aba da planilha foi removida no Google"

func (m *Manager) sheetAccess(ctx context.Context, sheet *googlesheet.Sheet) (googleaccount.Access, error) {
	access, err := m.access.Access(ctx, sheet.WorkspaceID, googleaccount.ProductSheets)
	if err != nil {
		return googleaccount.Access{}, err
	}
	if access.AccountID != sheet.AccountID {
		return googleaccount.Access{}, googlesheet.ErrAccountChanged
	}
	return access, nil
}

func (m *Manager) takeWrite(ctx context.Context, access googleaccount.Access) error {
	granted, err := m.limiter.TakeWrite(ctx, access.Email)
	if err != nil {
		return fmt.Errorf("%w: %v", googlesheet.ErrGoogleBusy, err)
	}
	if !granted {
		return googlesheet.ErrGoogleBusy
	}
	return nil
}

func (m *Manager) takeRead(ctx context.Context, access googleaccount.Access) error {
	granted, err := m.limiter.TakeRead(ctx, access.Email)
	if err != nil {
		return fmt.Errorf("%w: %v", googlesheet.ErrGoogleBusy, err)
	}
	if !granted {
		return googlesheet.ErrGoogleBusy
	}
	return nil
}

func (m *Manager) forgetTabCheck(sheetID string) {
	if err := m.marks.Del(tabCheckKey(sheetID)); err != nil {
		log.Printf("[googlesheet] tab check mark of %s not cleared: %v", sheetID, err)
	}
}

type GoogleRefusal struct {
	Failure googlesheet.Failure
	Err     error
}

func (e *GoogleRefusal) Error() string { return "googlesheet: google refused: " + e.Err.Error() }

func (e *GoogleRefusal) Unwrap() error { return e.Err }

func (m *Manager) googleRefusal(err error) error {
	var refusal *GoogleRefusal
	if errors.As(err, &refusal) {
		return err
	}
	return &GoogleRefusal{Failure: m.errs.Classify(err), Err: err}
}
