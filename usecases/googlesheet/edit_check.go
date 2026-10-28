package googlesheet

import (
	"context"
	"errors"

	"vozko/domain/googleaccount"
	"vozko/domain/googlesheet"
)

const (
	uncheckedAccount = "a conta Google conectada não pôde ler o arquivo"
	uncheckedGoogle  = "o Google não respondeu à leitura do arquivo"
	uncheckedTab     = "o Google não respondeu à leitura da aba"
)

func (m *Manager) inspectFile(ctx context.Context, workspaceID, spreadsheetID string) (googlesheet.EditCheck, error) {
	sheets, err := m.sheets.InSpreadsheet(ctx, workspaceID, spreadsheetID)
	if err != nil {
		return googlesheet.EditCheck{}, err
	}
	if len(sheets) == 0 {
		return googlesheet.NewEditCheck(nil), nil
	}
	access, err := m.sheetAccess(ctx, sheets[0])
	if errors.Is(err, googlesheet.ErrAccountChanged) || errors.Is(err, googleaccount.ErrNotConnected) {
		return googlesheet.UncheckedEdit(uncheckedAccount), nil
	}
	if err != nil {
		return googlesheet.EditCheck{}, err
	}
	if err := m.takeRead(ctx, access); err != nil {
		return googlesheet.UncheckedEdit(uncheckedGoogle), nil
	}
	workbook, err := m.api.Workbook(ctx, access.Token, spreadsheetID)
	if err != nil {
		return googlesheet.UncheckedEdit(uncheckedGoogle + ": " + m.errs.Classify(err).Detail), nil
	}
	reads := m.tabReads(access)
	var tabs []googlesheet.TabCheck
	for _, sheet := range sheets {
		checked, err := m.checkSheet(ctx, reads, sheet, workbook)
		if err != nil {
			return googlesheet.EditCheck{}, err
		}
		tabs = append(tabs, checked...)
	}
	return googlesheet.NewEditCheck(tabs), nil
}

func (m *Manager) checkSheet(ctx context.Context, reads tabReads, sheet *googlesheet.Sheet, workbook googlesheet.Workbook) ([]googlesheet.TabCheck, error) {
	info, live := workbook.Tab(sheet.TabID)
	if !live {
		gone := googlesheet.TabCheck{SheetID: sheet.ID, TabID: sheet.TabID, TabName: sheet.TabName, Reason: googlesheet.ReasonSpreadsheetGone, Detail: tabGoneDetail}
		return []googlesheet.TabCheck{gone}, raiseAttention(ctx, m.sheets, m.alerts, sheet, gone.Reason, gone.Detail)
	}
	current := googlesheet.TabCheck{SheetID: sheet.ID, TabID: info.ID, TabName: info.Title}
	if info.Title != sheet.TabName {
		current.RenamedFrom = sheet.TabName
		if err := m.sheets.UpdateTabName(ctx, sheet.ID, info.Title); err != nil {
			return nil, err
		}
		sheet.TabName = info.Title
	}
	targets, err := m.sheetTabs(ctx, sheet, workbook)
	if err != nil {
		return nil, err
	}
	checks := make([]googlesheet.TabCheck, len(targets))
	var broken *googlesheet.TabCheck
	for i, target := range targets {
		checks[i] = googlesheet.TabCheck{SheetID: sheet.ID, TabID: target.TabID, TabName: target.TabName}
		if i == 0 {
			checks[i] = current
		}
		inspection, _, err := reads.inspect(ctx, sheet, target)
		switch {
		case err != nil:
			checks[i].Unchecked = uncheckedTab
		case !inspection.place.InPlace():
			checks[i].Reason, checks[i].Detail = inspection.place.Reason, tabDetail(sheet, target, inspection.place.Detail)
			if broken == nil {
				broken = &checks[i]
			}
		default:
			checks[i].RefWarnings = inspection.snapshot.RefWarnings()
		}
	}
	if broken != nil {
		return checks, raiseAttention(ctx, m.sheets, m.alerts, sheet, broken.Reason, broken.Detail)
	}
	return checks, nil
}

func (m *Manager) resumeFineTabs(check googlesheet.EditCheck) {
	fine := map[string]bool{}
	var order []string
	for _, tab := range check.Tabs {
		if _, seen := fine[tab.SheetID]; !seen {
			order = append(order, tab.SheetID)
			fine[tab.SheetID] = true
		}
		if tab.Reason != "" || tab.Unchecked != "" {
			fine[tab.SheetID] = false
		}
	}
	for _, sheetID := range order {
		if fine[sheetID] {
			m.forgetTabCheck(sheetID)
			m.nudger.Nudge(sheetID)
		}
	}
}
