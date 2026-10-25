package workspace_department_repository

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func mockedRepository(t *testing.T) (*repositoryImpl, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return &repositoryImpl{db: db}, mock
}

func hasDepartmentsPattern() string {
	return "^" + strings.ReplaceAll(regexp.QuoteMeta(hasDepartmentsSQL), `\?`, `\$[0-9]+`) + "$"
}

func TestHasDepartmentsBindsOnlyTheWorkspace(t *testing.T) {
	if got := strings.Count(hasDepartmentsSQL, "?"); got != 1 {
		t.Fatalf("placeholders = %d, want 1", got)
	}
}

func TestHasDepartmentsAnswersWhetherAnyLiveDepartmentExists(t *testing.T) {
	for _, exists := range []bool{true, false} {
		repo, mock := mockedRepository(t)
		mock.ExpectQuery(hasDepartmentsPattern()).
			WithArgs("ws-1").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))

		got, err := repo.HasDepartments("ws-1")
		if err != nil {
			t.Fatal(err)
		}
		if got != exists {
			t.Fatalf("HasDepartments = %v, want %v", got, exists)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHasDepartmentsReturnsTheQueryError(t *testing.T) {
	repo, mock := mockedRepository(t)
	boom := errors.New("connection reset")
	mock.ExpectQuery(hasDepartmentsPattern()).WithArgs("ws-1").WillReturnError(boom)

	got, err := repo.HasDepartments("ws-1")
	if !errors.Is(err, boom) || got {
		t.Fatalf("HasDepartments = %v, %v, want false and the query error", got, err)
	}
}
