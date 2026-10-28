package googlesheets

import (
	"context"
	"testing"

	"vozko/domain/googlesheet"
)

const (
	movedRef = "6f1c2d3e-4a5b-4c6d-8e7f-90a1b2c3d4e5"
	otherRef = "7a2b3c4d-5e6f-4a7b-9c8d-0e1f2a3b4c5d"
)

func TestDeleteRowRemovesExactlyTheRowTheSnapshotLocatedAfterASort(t *testing.T) {
	client, g := newGoogleGrid(t, paymentsTab([]any{otherRef, "Bia"}, []any{"", "linha digitada"}, []any{movedRef, "Ana"}))
	row, err := snapshotOf(t, client, paymentsTarget()).Locate(movedRef)
	if err != nil || row != 4 {
		t.Fatalf("located row %d: %v", row, err)
	}
	if err := client.DeleteRow(context.Background(), "token", paymentsTarget(), row); err != nil {
		t.Fatalf("DeleteRow: %v", err)
	}
	got := g.column("Pagamentos · outubro 2026")
	if len(got) != 3 || got[0] != googlesheet.RefHeader || got[1] != otherRef || got[2] != "" {
		t.Fatalf("column A = %v, want the header, the other row and the human row", got)
	}
	if rows := g.tabs["Pagamentos · outubro 2026"]; rows.grid != googlesheet.NewTabRows-1 {
		t.Fatalf("grid = %d, the row must be deleted, not cleared", rows.grid)
	}
	requests := insertKinds(t, g)
	if len(requests) != 1 || requests[0]["deleteDimension"] == nil {
		t.Fatalf("requests = %v, want one deleteDimension", requests)
	}
}

func TestDeleteRowNeverTouchesTheHeader(t *testing.T) {
	client, g := newGoogleGrid(t, paymentsTab([]any{movedRef, "Ana"}))
	for _, row := range []int{1, 0, -3} {
		if err := client.DeleteRow(context.Background(), "token", paymentsTarget(), row); err == nil {
			t.Fatalf("row %d was accepted", row)
		}
	}
	if len(g.batches) != 0 {
		t.Fatalf("google was called: %v", g.batches)
	}
}
