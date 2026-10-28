package googlesheet

import (
	"context"
	"fmt"
	"testing"

	"vozko/domain/googlesheet"
)

const brokenAnchorRow = "b8dabd6c-409a-437d-90f4-c0d8193e5cc3"

func (w *world) linkedPayments(t *testing.T) (*googlesheet.Sheet, *googlesheet.Sheet) {
	t.Helper()
	timeline := w.storeTimeline(t)
	payments := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Pagamentos", Columns: paymentColumns(timeline.ID)}, "sp-1", 41)
	w.api.headers[41] = toAny(payments.HeaderRow())
	w.api.workbook.Tabs = append(w.api.workbook.Tabs, googlesheet.TabInfo{ID: 41, Title: "Pagamentos", Rows: 100, Columns: 4})
	w.exports.anchors[exportKey(timeline.ID, "conv-1", "whatsapp")] = googlesheet.ExportAnchor{RowID: anchorRowID}
	return timeline, payments
}

func (w *world) enqueuePayment(t *testing.T, payments *googlesheet.Sheet, key string) {
	t.Helper()
	origin := googlesheet.Origin{Kind: googlesheet.OriginWorkflow, RunID: "run-" + key, EntryID: "conv-1", EntryType: "whatsapp"}
	in := EnqueueInput{WorkspaceID: testWorkspace, SheetID: payments.ID, Values: map[string]any{"valor": "10"}, IdempotencyKey: key, Origin: origin}
	if _, err := w.intake.Enqueue(context.Background(), in); err != nil {
		t.Fatal(err)
	}
}

func timelineLink(t *testing.T, sep googlesheet.Separator) googlesheet.Formula {
	t.Helper()
	formula, err := googlesheet.LinkFormula(googlesheet.TabRef{ID: 31, Title: "Negociações"}, anchorRowID, googlesheet.DefaultLinkLabel, sep)
	if err != nil {
		t.Fatal(err)
	}
	return formula
}

func TestWriterBuildsTheLinkInTheLocaleTheFileHasNow(t *testing.T) {
	for locale, sep := range map[string]googlesheet.Separator{"pt_BR": googlesheet.SeparatorSemicolon, "en_US": googlesheet.SeparatorComma} {
		t.Run(locale, func(t *testing.T) {
			w := newWorld(t)
			w.api.locale = locale
			_, payments := w.linkedPayments(t)
			w.enqueuePayment(t, payments, "k1")
			if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
				t.Fatal(err)
			}
			if len(w.api.appends) != 1 {
				t.Fatalf("appends = %+v", w.api.appends)
			}
			if got := w.api.appends[0].rows[0][3]; got != timelineLink(t, sep) {
				t.Fatalf("link cell = %#v, want %s", got, timelineLink(t, sep))
			}
		})
	}
}

func TestWriterReadsTheLocaleOncePerRunEvenWhenTheTabCheckIsNotDue(t *testing.T) {
	w := newWorld(t)
	_, payments := w.linkedPayments(t)
	w.marks.keys[tabCheckKey(payments.ID)] = true
	w.enqueuePayment(t, payments, "k1")
	w.enqueuePayment(t, payments, "k2")
	w.writer.cfg.BatchRows = 1
	if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	if len(w.api.appends) != 2 || w.api.workbookCalls != 1 {
		t.Fatalf("appends %d workbook reads %d, want 2 and 1", len(w.api.appends), w.api.workbookCalls)
	}
}

func TestASheetWithoutLinksKeepsTheTabCheckCadence(t *testing.T) {
	w := newWorld(t)
	w.marks.keys[tabCheckKey(w.sheet.ID)] = true
	w.enqueue(t, "k1", map[string]any{"nome": "Ana"})
	if err := w.writer.WriteSheet(context.Background(), w.sheet.ID); err != nil {
		t.Fatal(err)
	}
	if w.api.workbookCalls != 0 || w.api.linkReads != 0 {
		t.Fatalf("workbook reads %d link reads %d for a sheet without links", w.api.workbookCalls, w.api.linkReads)
	}
}

func TestAnUnsupportedLocaleFailsTheLinkRowInsteadOfWritingABrokenFormula(t *testing.T) {
	for _, locale := range []string{"", "xx_YY"} {
		t.Run(fmt.Sprintf("%q", locale), func(t *testing.T) {
			w := newWorld(t)
			w.api.locale = locale
			_, payments := w.linkedPayments(t)
			w.enqueuePayment(t, payments, "k1")
			if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
				t.Fatal(err)
			}
			failed := w.rows.byStatus(payments.ID, googlesheet.RowFailed)
			if len(failed) != 1 || failed[0].ErrorCode != googlesheet.CodeWriteLocaleUnsupported || len(w.api.appends) != 0 {
				t.Fatalf("failed = %+v appends %d", failed, len(w.api.appends))
			}
			if len(w.api.formulaWrites) != 0 {
				t.Fatalf("a repair ran without a known separator: %+v", w.api.formulaWrites)
			}
		})
	}
}

func brokenCell(t *testing.T, row int) googlesheet.LinkCell {
	t.Helper()
	return googlesheet.LinkCell{Row: row, Column: 3, RowRef: fmt.Sprintf("00000000-0000-4000-8000-%012d", row), Formula: timelineLink(t, googlesheet.SeparatorComma), Failed: true}
}

func TestWriterRepairsTheLinksItWroteWithTheOtherSeparator(t *testing.T) {
	w := newWorld(t)
	_, payments := w.linkedPayments(t)
	healthy := googlesheet.LinkCell{Row: 3, Column: 3, RowRef: brokenAnchorRow, Formula: timelineLink(t, googlesheet.SeparatorSemicolon)}
	human := brokenCell(t, 5)
	human.RowRef = "linha do João"
	w.api.linkCells[41] = []googlesheet.LinkCell{brokenCell(t, 2), healthy, brokenCell(t, 4), human}
	w.enqueuePayment(t, payments, "k1")
	if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	if len(w.api.formulaWrites) != 1 || w.api.formulaWrites[0].tab != 41 {
		t.Fatalf("formula writes = %+v", w.api.formulaWrites)
	}
	fixed := w.api.formulaWrites[0].cells
	if len(fixed) != 2 || fixed[0].Row != 2 || fixed[1].Row != 4 || fixed[0].Column != 3 || fixed[0].Formula != timelineLink(t, googlesheet.SeparatorSemicolon) {
		t.Fatalf("fixed = %+v", fixed)
	}
	if len(w.api.appends) != 1 {
		t.Fatalf("the new row was not written after the repair: %+v", w.api.appends)
	}
	if w.api.linkCells[41][3].Formula != human.Formula {
		t.Fatal("a row that is not ours was rewritten")
	}
}

func TestTheRepairIsIdempotentAndScansATabOncePerLocale(t *testing.T) {
	w := newWorld(t)
	_, payments := w.linkedPayments(t)
	w.api.linkCells[41] = []googlesheet.LinkCell{brokenCell(t, 2)}
	w.enqueuePayment(t, payments, "k1")
	if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	reads := w.api.linkReads
	w.enqueuePayment(t, payments, "k2")
	if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	if w.api.linkReads != reads || len(w.api.formulaWrites) != 1 {
		t.Fatalf("link reads %d then %d, formula writes %d", reads, w.api.linkReads, len(w.api.formulaWrites))
	}
	w.marks.keys = map[string]bool{}
	w.enqueuePayment(t, payments, "k3")
	if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	if w.api.linkReads == reads || len(w.api.formulaWrites) != 1 {
		t.Fatalf("a rescan rewrote healthy links: reads %d writes %+v", w.api.linkReads, w.api.formulaWrites)
	}
	w.api.locale = "en_US"
	w.enqueuePayment(t, payments, "k4")
	if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	if w.api.linkReads != reads+2 {
		t.Fatalf("a locale change did not rescan the tab: reads %d", w.api.linkReads)
	}
}

func TestTheRepairIsBoundedPerRunAndCarriesOnInTheNext(t *testing.T) {
	w := newWorld(t)
	_, payments := w.linkedPayments(t)
	total := googlesheet.MaxLinkRepairs + 3
	for row := 2; row < total+2; row++ {
		w.api.linkCells[41] = append(w.api.linkCells[41], brokenCell(t, row))
	}
	w.enqueuePayment(t, payments, "k1")
	if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	if len(w.api.formulaWrites) != 1 || len(w.api.formulaWrites[0].cells) != googlesheet.MaxLinkRepairs {
		t.Fatalf("first run rewrote %d batches", len(w.api.formulaWrites))
	}
	w.enqueuePayment(t, payments, "k2")
	if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	if len(w.api.formulaWrites) != 2 || len(w.api.formulaWrites[1].cells) != 3 {
		t.Fatalf("second run = %+v", w.api.formulaWrites[1:])
	}
}

func TestARepairGoogleRefusesNeverBlocksTheRows(t *testing.T) {
	w := newWorld(t)
	_, payments := w.linkedPayments(t)
	w.api.linkCells[41] = []googlesheet.LinkCell{brokenCell(t, 2)}
	w.api.formulaErr = errTransient
	w.enqueuePayment(t, payments, "k1")
	if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	if len(w.api.appends) != 1 {
		t.Fatalf("appends = %+v", w.api.appends)
	}
	w.api.formulaErr = nil
	w.enqueuePayment(t, payments, "k2")
	if err := w.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	if len(w.api.formulaWrites) != 2 || w.api.linkCells[41][0].Failed {
		t.Fatalf("the refused repair was not retried: %+v", w.api.formulaWrites)
	}
}
