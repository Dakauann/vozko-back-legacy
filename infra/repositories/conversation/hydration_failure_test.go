package conversation_repository

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

func TestSearchEntriesByFilter_RefusesWhenAPageCannotBeHydrated(t *testing.T) {
	db, mock, sqlDB := newStatusDB(t)
	defer sqlDB.Close()
	mock.MatchExpectationsInOrder(true)

	boom := errors.New("hydration failed")
	mock.ExpectQuery(`SELECT entry_id, entry_type, lead_name, lead_number, COUNT\(\*\) OVER \(\) AS page_total FROM entries_with_msg`).
		WillReturnRows(sqlmock.NewRows([]string{"entry_id", "entry_type", "lead_name", "lead_number", "page_total"}).
			AddRow("e-1", string(shared.EntryTypeWhatsApp), "Ana", "5511999999999", 1))
	mock.ExpectQuery(`.`).WillReturnError(boom)

	entries, total, err := (&repository{db: db}).SearchEntriesByFilter(selectionInput())
	if !errors.Is(err, boom) {
		t.Fatalf("SearchEntriesByFilter err = %v, want the hydration failure", err)
	}
	if entries != nil || total != 0 {
		t.Fatalf("a refused page returns nothing, got %d entries and total %d", len(entries), total)
	}
}

func TestSearchEntriesWithMessages_WorkspaceSearchRefusesWhenAPageCannotBeHydrated(t *testing.T) {
	db, mock, sqlDB := newStatusDB(t)
	defer sqlDB.Close()
	mock.MatchExpectationsInOrder(true)

	boom := errors.New("hydration failed")
	mock.ExpectQuery(`SELECT entry_id, entry_type, COUNT\(\*\) OVER \(\) AS page_total FROM entries_with_msg`).
		WillReturnRows(sqlmock.NewRows([]string{"entry_id", "entry_type", "page_total"}).
			AddRow("e-1", string(shared.EntryTypeWhatsApp), 1))
	mock.ExpectQuery(`.`).WillReturnError(boom)

	entries, total, err := (&repository{db: db}).SearchEntriesWithMessages(conversation.SearchEntriesInput{WorkspaceID: "ws-1"})
	if !errors.Is(err, boom) {
		t.Fatalf("SearchEntriesWithMessages err = %v, want the hydration failure", err)
	}
	if entries != nil || total != 0 {
		t.Fatalf("a refused page returns nothing, got %d entries and total %d", len(entries), total)
	}
}
