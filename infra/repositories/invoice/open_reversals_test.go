package invoice_repository

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOpenReversalAsksForReversedInvoicesOfTheWorkspace(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(openReversalSQL, "?"); got != 2 {
		t.Fatalf("placeholders = %d, want 2", got)
	}
	mock.ExpectQuery("^"+strings.ReplaceAll(regexp.QuoteMeta(openReversalSQL), `\?`, `\$[0-9]+`)+"$").
		WithArgs("ws-1", "REVERSED").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	open, err := NewOpenReversals(db).HasOpenReversal(context.Background(), "ws-1")
	if err != nil || !open {
		t.Fatalf("open = %v, err = %v", open, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
