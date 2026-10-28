package conversation_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

func pageOriginMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

func TestAPageOriginClaimDoesNothingWhenOneExists(t *testing.T) {
	db, mock := pageOriginMock(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "conversation_page_origins" .* ON CONFLICT DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	claimed, err := NewPageOriginRepository(db).Claim(context.Background(), conversation.PageOrigin{EntryID: "e", EntryType: shared.EntryTypeWhatsApp, WorkspaceID: "w", ProjectID: "p", Reference: "LP7K2", ArrivedAt: time.Now()})
	if err != nil || claimed {
		t.Fatalf("claimed %v err %v", claimed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAnIncompletePageOriginIsNeverStored(t *testing.T) {
	db, mock := pageOriginMock(t)
	if _, err := NewPageOriginRepository(db).Claim(context.Background(), conversation.PageOrigin{EntryID: "e"}); err == nil {
		t.Fatal("an incomplete origin must refuse")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAnUnreadablePageOriginIsAnError(t *testing.T) {
	db, mock := pageOriginMock(t)
	mock.ExpectQuery(`SELECT .* FROM "conversation_page_origins"`).WillReturnError(errors.New("down"))
	if got, err := NewPageOriginRepository(db).Get(context.Background(), "e", shared.EntryTypeWhatsApp); err == nil || got != nil {
		t.Fatalf("got %+v %v", got, err)
	}
}
