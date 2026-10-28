package googlesheet

import (
	"context"
	"fmt"

	"vozko/domain/googleaccount"
	"vozko/domain/googlesheet"
)

type tabReads struct {
	api    googlesheet.SheetsAPI
	token  string
	take   func(ctx context.Context) (bool, error)
	failed func(ctx context.Context, err error) error
}

type tabInspection struct {
	snapshot googlesheet.TabSnapshot
	place    googlesheet.HeaderPlace
}

func (r tabReads) read(ctx context.Context, target googlesheet.Target, ranges ...string) ([][][]any, bool, error) {
	if ok, err := r.take(ctx); !ok || err != nil {
		return nil, false, err
	}
	read, err := r.api.ReadRanges(ctx, r.token, target, ranges)
	if err != nil {
		return nil, false, r.failed(ctx, err)
	}
	return read, true, nil
}

func (r tabReads) inspect(ctx context.Context, sheet *googlesheet.Sheet, target googlesheet.Target) (tabInspection, bool, error) {
	read, ok, err := r.read(ctx, target, googlesheet.HeaderRange, googlesheet.RefColumnRange)
	if err != nil || !ok {
		return tabInspection{}, false, err
	}
	snapshot := googlesheet.TabSnapshot{Header: read[0], Refs: read[1]}
	var candidate [][]any
	if row, found := snapshot.HeaderCandidate(); found && googlesheet.HeaderDifference(sheet.HeaderRow(), snapshot.Header) != "" {
		lower, ok, err := r.read(ctx, target, googlesheet.RowRange(row))
		if err != nil || !ok {
			return tabInspection{}, false, err
		}
		candidate = lower[0]
	}
	return tabInspection{snapshot: snapshot, place: sheet.PlaceHeader(snapshot, candidate)}, true, nil
}

func periodTarget(ctx context.Context, sheets googlesheet.SheetRepository, sheet *googlesheet.Sheet, period string) (googlesheet.Target, bool, error) {
	if !sheet.Periodic() || period == sheet.TabPeriodKey {
		return sheet.Target(), true, nil
	}
	tab, found, err := sheets.FindTab(ctx, sheet.ID, period)
	if err != nil || !found {
		return googlesheet.Target{}, false, err
	}
	return googlesheet.Target{SpreadsheetID: sheet.SpreadsheetID, TabID: tab.ID, TabName: tab.Title}, true, nil
}

func tabDetail(sheet *googlesheet.Sheet, target googlesheet.Target, detail string) string {
	if target.TabID == sheet.TabID {
		return detail
	}
	return fmt.Sprintf("aba %q: %s", target.TabName, detail)
}

func (m *Manager) tabReads(access googleaccount.Access) tabReads {
	return tabReads{
		api: m.api, token: access.Token,
		take:   func(ctx context.Context) (bool, error) { return true, m.takeRead(ctx, access) },
		failed: func(_ context.Context, err error) error { return m.googleRefusal(err) },
	}
}

func (m *Manager) sheetTabs(ctx context.Context, sheet *googlesheet.Sheet, workbook googlesheet.Workbook) ([]googlesheet.Target, error) {
	return liveTabs(ctx, m.sheets, sheet, workbook)
}

func liveTabs(ctx context.Context, sheets googlesheet.SheetRepository, sheet *googlesheet.Sheet, workbook googlesheet.Workbook) ([]googlesheet.Target, error) {
	targets := []googlesheet.Target{sheet.Target()}
	if !sheet.Periodic() {
		return targets, nil
	}
	tabs, err := sheets.Tabs(ctx, sheet.ID)
	if err != nil {
		return nil, err
	}
	for _, tab := range tabs {
		info, live := workbook.Tab(tab.ID)
		if tab.ID == sheet.TabID || !live {
			continue
		}
		targets = append(targets, googlesheet.Target{SpreadsheetID: sheet.SpreadsheetID, TabID: info.ID, TabName: info.Title})
	}
	return targets, nil
}

func (m *Manager) settleHeaders(ctx context.Context, access googleaccount.Access, sheet *googlesheet.Sheet, workbook googlesheet.Workbook, repair bool) error {
	targets, err := m.sheetTabs(ctx, sheet, workbook)
	if err != nil {
		return err
	}
	reads := m.tabReads(access)
	for _, target := range targets {
		if err := m.settleHeader(ctx, access, reads, sheet, target, repair); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) settleHeader(ctx context.Context, access googleaccount.Access, reads tabReads, sheet *googlesheet.Sheet, target googlesheet.Target, repair bool) error {
	inspection, _, err := reads.inspect(ctx, sheet, target)
	if err != nil {
		return err
	}
	if inspection.place.InPlace() {
		return nil
	}
	if !repair {
		return m.holdForHeader(ctx, sheet, target, inspection.place)
	}
	if err := m.takeWrite(ctx, access); err != nil {
		return err
	}
	fix := sheet.RepairFor(inspection.snapshot, inspection.place)
	if err := m.api.RepairHeader(ctx, access.Token, target, fix, sheet.Layout(len(sheet.Columns))); err != nil {
		return m.googleRefusal(err)
	}
	again, _, err := reads.inspect(ctx, sheet, target)
	if err != nil {
		return err
	}
	if !again.place.InPlace() {
		return m.holdForHeader(ctx, sheet, target, again.place)
	}
	return nil
}

func (m *Manager) holdForHeader(ctx context.Context, sheet *googlesheet.Sheet, target googlesheet.Target, place googlesheet.HeaderPlace) error {
	detail := tabDetail(sheet, target, place.Detail)
	sheet.NeedsAttention(place.Reason, detail)
	if err := m.sheets.SaveStatus(ctx, sheet); err != nil {
		return err
	}
	return fmt.Errorf("%w: %s: %s", googlesheet.ErrSheetNotActive, place.Reason, detail)
}
