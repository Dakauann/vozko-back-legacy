package sitepublish_repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestTheLastRolledDayIsTheNewestStoredDay(t *testing.T) {
	db, mock := mockDB(t)
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(sqlPattern(lastDaySQL)).WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(day))
	got, found, err := NewAnalytics(db).LastDay(context.Background())
	if err != nil || !found || !got.Equal(day) {
		t.Fatalf("got %s %v %v", got, found, err)
	}
	mock.ExpectQuery(sqlPattern(lastDaySQL)).WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil))
	if _, found, err := NewAnalytics(db).LastDay(context.Background()); err != nil || found {
		t.Fatalf("an empty table has no last day: %v %v", found, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
