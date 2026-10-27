package googlesheet

import (
	"context"
	"fmt"
	"strings"

	"vozko/domain/googlesheet"
	"vozko/domain/shared/deps"
)

type ExportsDeps struct {
	Intake  *Intake
	Exports googlesheet.ExportRepository
}

type Exports struct {
	intake  *Intake
	exports googlesheet.ExportRepository
}

func NewExports(d ExportsDeps) (*Exports, error) {
	if err := deps.Require("google sheet message exports", map[string]any{"intake": d.Intake, "exports": d.Exports}); err != nil {
		return nil, err
	}
	return &Exports{intake: d.Intake, exports: d.Exports}, nil
}

type ExportTarget struct {
	WorkspaceID string
	SheetID     string
	Origin      googlesheet.Origin
}

func (t ExportTarget) onConversation() bool {
	return strings.TrimSpace(t.Origin.EntryID) != "" && strings.TrimSpace(t.Origin.EntryType) != ""
}

type ExportInput struct {
	Target   ExportTarget
	Values   map[string]any
	Messages []googlesheet.ExportedMessage
	Through  googlesheet.ExportPosition
}

type ExportResult struct {
	Sheet      *googlesheet.Sheet
	Exported   int
	Duplicates int
}

func (x *Exports) Start(ctx context.Context, target ExportTarget) (*googlesheet.Sheet, googlesheet.ExportPosition, error) {
	sheet, err := x.exportingSheet(ctx, target)
	if err != nil {
		return nil, googlesheet.ExportPosition{}, err
	}
	position, err := x.exports.Position(ctx, sheet.ID, target.Origin.EntryID, target.Origin.EntryType)
	if err != nil {
		return nil, googlesheet.ExportPosition{}, err
	}
	return sheet, position, nil
}

func (x *Exports) Record(ctx context.Context, in ExportInput) (*ExportResult, error) {
	sheet, err := x.exportingSheet(ctx, in.Target)
	if err != nil {
		return nil, err
	}
	rows, err := x.rowsFor(ctx, sheet, in)
	if err != nil {
		return nil, err
	}
	inserted, err := x.exports.Record(ctx, googlesheet.MessageExport{
		WorkspaceID: sheet.WorkspaceID, SheetID: sheet.ID,
		EntryID: in.Target.Origin.EntryID, EntryType: in.Target.Origin.EntryType,
		Rows: rows, Through: in.Through,
	})
	if err != nil {
		return nil, err
	}
	if inserted > 0 {
		x.intake.nudger.Nudge(sheet.ID)
	}
	return &ExportResult{Sheet: sheet, Exported: inserted, Duplicates: len(rows) - inserted}, nil
}

func (x *Exports) exportingSheet(ctx context.Context, target ExportTarget) (*googlesheet.Sheet, error) {
	if !target.onConversation() {
		return nil, googlesheet.ErrNeedsConversation
	}
	sheet, err := x.intake.writableSheet(ctx, target.WorkspaceID, target.SheetID)
	if err != nil {
		return nil, err
	}
	if !sheet.HasMessageColumns() {
		return nil, googlesheet.ErrNoMessageColumns
	}
	return sheet, nil
}

func (x *Exports) rowsFor(ctx context.Context, sheet *googlesheet.Sheet, in ExportInput) ([]*googlesheet.Row, error) {
	if len(in.Messages) == 0 {
		return nil, nil
	}
	conversationValues, err := x.intake.systemValues(ctx, sheet, in.Target.WorkspaceID, in.Target.Origin)
	if err != nil {
		return nil, err
	}
	now := x.intake.now()
	accepted := make([]googlesheet.MessageRow, 0, len(in.Messages))
	for _, message := range in.Messages {
		cells, err := sheet.Accept(in.Values, sheet.WithMessage(conversationValues, message), now)
		if err != nil {
			return nil, fmt.Errorf("mensagem %s: %w", message.ID, err)
		}
		accepted = append(accepted, googlesheet.MessageRow{MessageID: message.ID, Cells: cells})
	}
	return googlesheet.NewMessageRows(sheet, accepted, in.Target.Origin, now)
}
