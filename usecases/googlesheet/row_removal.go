package googlesheet

import (
	"context"
	"fmt"

	"vozko/domain/cache"
	"vozko/domain/googleaccount"
	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

type RowRemoverDeps struct {
	Manager  *Manager
	Removals googlesheet.RowRemovalRepository
	Leases   cache.Locker
}

type RowRemover struct {
	m        *Manager
	removals googlesheet.RowRemovalRepository
	leases   cache.Locker
}

func NewRowRemover(d RowRemoverDeps) *RowRemover {
	if d.Manager == nil || d.Removals == nil || d.Leases == nil {
		panic("googlesheet: the row remover requires the manager, the removal records and sheet leases")
	}
	return &RowRemover{m: d.Manager, removals: d.Removals, leases: d.Leases}
}

type RemoveRowInput struct {
	WorkspaceID string
	SheetID     string
	RowID       string
	ActorID     string
	Reason      string
}

type RemovalView struct {
	Removal   *googlesheet.RowRemoval
	Origin    OriginView
	RemovedBy *NamedRef
}

func (r *RowRemover) Cancel(ctx context.Context, in RemoveRowInput) (*googlesheet.RowRemoval, error) {
	_, row, err := r.request(ctx, in, googlesheet.RemovalCancel)
	if err != nil {
		return nil, err
	}
	removal, err := googlesheet.NewRowRemoval(row, googlesheet.RemovalCancel, in.ActorID, in.Reason, 0, r.m.now())
	if err != nil {
		return nil, err
	}
	if err := r.removals.CancelQueued(ctx, removal); err != nil {
		return nil, err
	}
	return removal, nil
}

func (r *RowRemover) Remove(ctx context.Context, in RemoveRowInput) (*googlesheet.RowRemoval, error) {
	if _, _, err := r.request(ctx, in, googlesheet.RemovalDelete); err != nil {
		return nil, err
	}
	release, busy, err := acquireSheet(ctx, r.leases, in.SheetID)
	if busy {
		return nil, googlesheet.ErrSheetBusy
	}
	if err != nil {
		return nil, err
	}
	defer release()
	sheet, row, err := r.request(ctx, in, googlesheet.RemovalDelete)
	if err != nil {
		return nil, err
	}
	if err := sheet.Writable(); err != nil {
		return nil, err
	}
	access, err := r.m.sheetAccess(ctx, sheet)
	if err != nil {
		return nil, err
	}
	target, number, err := r.locate(ctx, access, sheet, row)
	if err != nil {
		return nil, err
	}
	removal, err := googlesheet.NewRowRemoval(row, googlesheet.RemovalDelete, in.ActorID, in.Reason, number, r.m.now())
	if err != nil {
		return nil, err
	}
	if err := r.m.takeWrite(ctx, access); err != nil {
		return nil, err
	}
	if err := r.m.api.DeleteRow(ctx, access.Token, target, number); err != nil {
		return nil, r.m.googleRefusal(err)
	}
	if err := r.removals.RecordDeleted(ctx, removal); err != nil {
		return nil, fmt.Errorf("googlesheet: row %s was deleted in google but its removal was not recorded: %w", row.ID, err)
	}
	return removal, nil
}

func (r *RowRemover) request(ctx context.Context, in RemoveRowInput, kind googlesheet.RemovalKind) (*googlesheet.Sheet, *googlesheet.Row, error) {
	if _, _, err := googlesheet.CheckRemovalRequest(in.ActorID, in.Reason); err != nil {
		return nil, nil, err
	}
	sheet, err := r.m.sheets.FindInWorkspace(ctx, in.WorkspaceID, in.SheetID)
	if err != nil {
		return nil, nil, err
	}
	if !googlesheet.IsRowRef(in.RowID) {
		return nil, nil, googlesheet.ErrRowNotFound
	}
	row, err := r.removals.FindRow(ctx, sheet.WorkspaceID, sheet.ID, in.RowID)
	if err != nil {
		return nil, nil, err
	}
	if err := row.CheckRemoval(kind); err != nil {
		return nil, nil, err
	}
	return sheet, row, nil
}

func (r *RowRemover) locate(ctx context.Context, access googleaccount.Access, sheet *googlesheet.Sheet, row *googlesheet.Row) (googlesheet.Target, int, error) {
	target, known, err := periodTarget(ctx, r.m.sheets, sheet, row.PeriodKey)
	if err != nil {
		return googlesheet.Target{}, 0, err
	}
	if !known {
		return googlesheet.Target{}, 0, googlesheet.ErrRowNotInSheet
	}
	inspection, _, err := r.m.tabReads(access).inspect(ctx, sheet, target)
	if err != nil {
		return googlesheet.Target{}, 0, err
	}
	if !inspection.place.InPlace() {
		return googlesheet.Target{}, 0, r.m.holdForHeader(ctx, sheet, target, inspection.place)
	}
	number, err := inspection.snapshot.Locate(row.ID)
	return target, number, err
}

func (r *RowRemover) Removals(ctx context.Context, workspaceID, sheetID string, pagination shared.Pagination) (*shared.PaginatedResult[RemovalView], error) {
	sheet, err := r.m.sheets.FindInWorkspace(ctx, workspaceID, sheetID)
	if err != nil {
		return nil, err
	}
	pagination = shared.NormalizePagination(pagination)
	page, err := r.removals.ListRemovals(ctx, sheet.WorkspaceID, sheet.ID, pagination)
	if err != nil {
		return nil, err
	}
	describer := r.m.names.describer(sheet.WorkspaceID)
	views := make([]RemovalView, len(page.Items))
	for i, removal := range page.Items {
		removal.Cells = sheet.DisplayCells(removal.Cells)
		origin, err := describer.describe(ctx, removal.Origin)
		if err != nil {
			return nil, err
		}
		removedBy, err := describer.people.ref(ctx, sheet.WorkspaceID, removal.RemovedBy)
		if err != nil {
			return nil, err
		}
		views[i] = RemovalView{Removal: removal, Origin: origin, RemovedBy: removedBy}
	}
	return &shared.PaginatedResult[RemovalView]{Items: views, Page: page.Page, PageSize: page.PageSize, TotalItems: page.TotalItems, TotalPages: page.TotalPages}, nil
}
