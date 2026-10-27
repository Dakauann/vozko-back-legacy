package googlesheet

import (
	"context"
	"log"
	"strings"

	"vozko/domain/googlesheet"
)

type LinkAnchors interface {
	Anchor(ctx context.Context, sheetID, entryID, entryType string) (googlesheet.ExportAnchor, bool, error)
}

type Intake struct {
	sheets   googlesheet.SheetRepository
	rows     googlesheet.RowRepository
	resolver googlesheet.SystemValueResolver
	anchors  LinkAnchors
	nudger   Nudger
	now      Clock
}

func NewIntake(sheets googlesheet.SheetRepository, rows googlesheet.RowRepository, resolver googlesheet.SystemValueResolver, anchors LinkAnchors, nudger Nudger) *Intake {
	if sheets == nil || rows == nil || resolver == nil || anchors == nil || nudger == nil {
		panic("googlesheet: intake requires sheets, rows, a system value resolver, export anchors and a nudger")
	}
	return &Intake{sheets: sheets, rows: rows, resolver: resolver, anchors: anchors, nudger: nudger, now: utcNow}
}

type EnqueueInput struct {
	WorkspaceID    string
	SheetID        string
	Values         map[string]any
	IdempotencyKey string
	Origin         googlesheet.Origin
}

type EnqueueResult struct {
	RowID     string
	Duplicate bool
	Sheet     *googlesheet.Sheet
	Cells     googlesheet.Cells
}

func (in *Intake) Enqueue(ctx context.Context, input EnqueueInput) (*EnqueueResult, error) {
	sheet, cells, err := in.accept(ctx, input.WorkspaceID, input.SheetID, input.Values, input.Origin)
	if err != nil {
		return nil, err
	}
	row, err := googlesheet.NewRow(sheet, cells, input.IdempotencyKey, input.Origin, in.now())
	if err != nil {
		return nil, err
	}
	stored, duplicate, err := in.rows.Insert(ctx, row)
	if err != nil {
		return nil, err
	}
	if !duplicate {
		in.nudger.Nudge(sheet.ID)
	}
	return &EnqueueResult{RowID: stored.ID, Duplicate: duplicate, Sheet: sheet, Cells: stored.Cells}, nil
}

func (in *Intake) Preview(ctx context.Context, workspaceID, sheetID string, values map[string]any, origin googlesheet.Origin) (*googlesheet.Sheet, googlesheet.Cells, error) {
	return in.accept(ctx, workspaceID, sheetID, values, origin)
}

func (in *Intake) accept(ctx context.Context, workspaceID, sheetID string, values map[string]any, origin googlesheet.Origin) (*googlesheet.Sheet, googlesheet.Cells, error) {
	sheet, err := in.writableSheet(ctx, workspaceID, sheetID)
	if err != nil {
		return nil, nil, err
	}
	system, err := in.systemValues(ctx, sheet, workspaceID, origin)
	if err != nil {
		return nil, nil, err
	}
	cells, err := sheet.Accept(values, system, in.now())
	if err != nil {
		return nil, nil, err
	}
	return sheet, cells, nil
}

func (in *Intake) writableSheet(ctx context.Context, workspaceID, sheetID string) (*googlesheet.Sheet, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(sheetID) == "" {
		return nil, googlesheet.ErrSheetNotFound
	}
	sheet, err := in.sheets.FindInWorkspace(ctx, workspaceID, sheetID)
	if err != nil {
		return nil, err
	}
	if err := sheet.Writable(); err != nil {
		return nil, err
	}
	return sheet, nil
}

func (in *Intake) systemValues(ctx context.Context, sheet *googlesheet.Sheet, workspaceID string, origin googlesheet.Origin) (googlesheet.SystemValues, error) {
	system := googlesheet.SystemValues{}
	if needsContext(sheet) {
		resolved, err := in.resolver.Resolve(ctx, workspaceID, origin)
		if err != nil {
			log.Printf("[googlesheet] system values of %s unreadable: %v", sheet.ID, err)
			return nil, err
		}
		system = resolved
	}
	return in.withAnchors(ctx, sheet, system, origin)
}

func (in *Intake) withAnchors(ctx context.Context, sheet *googlesheet.Sheet, system googlesheet.SystemValues, origin googlesheet.Origin) (googlesheet.SystemValues, error) {
	if strings.TrimSpace(origin.EntryID) == "" || strings.TrimSpace(origin.EntryType) == "" {
		return system, nil
	}
	for _, col := range sheet.LinkColumns() {
		anchor, found, err := in.anchors.Anchor(ctx, col.LinkSheetID, origin.EntryID, origin.EntryType)
		if err != nil {
			return nil, err
		}
		if found {
			system = system.WithLink(col.LinkSheetID, anchor)
		}
	}
	return system, nil
}

func needsContext(sheet *googlesheet.Sheet) bool {
	for _, col := range sheet.Columns {
		if !col.Archived && col.Source.IsSystem() && col.Source != googlesheet.SourceWrittenAt && col.Source != googlesheet.SourceExportLink && !col.Source.PerMessage() {
			return true
		}
	}
	return false
}
