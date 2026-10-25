package attendance_repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"

	"vozko/domain/attendance"
)

const scopeBuildStatements = 8

func countContaining(statements []string, fragment string) int {
	count := 0
	for _, statement := range statements {
		if strings.Contains(statement, fragment) {
			count++
		}
	}
	return count
}

func expectExecs(mock sqlmock.Sqlmock, n int) {
	for range n {
		mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
	}
}

func TestASharedScopeIsBuiltOnceForEveryReadOfTheRequest(t *testing.T) {
	db, mock, captured := newCapturingDB(t)
	mock.ExpectBegin()
	expectExecs(mock, 3+scopeBuildStatements)
	expectExecs(mock, 3)
	expectExecs(mock, 3)
	expectExecs(mock, 3)
	mock.ExpectCommit()

	repo := &repository{db: db}
	filter := activityWindow()
	reads := 0
	err := repo.ShareScope(context.Background(), func(ctx context.Context) error {
		for range 2 {
			if err := repo.withScope(ctx, "ws-1", filter, func(*gorm.DB) error { reads++; return nil }); err != nil {
				return err
			}
		}
		return repo.analyticsContext(ctx, func(*gorm.DB) error { reads++; return nil })
	})
	if err != nil {
		t.Fatalf("ShareScope() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("one transaction, one build: %v", err)
	}
	statements := captured.all()
	if reads != 3 {
		t.Fatalf("reads = %d, want 3", reads)
	}
	if got := countContaining(statements, "CREATE TEMP TABLE "+scopeEntriesTable+" "); got != 1 {
		t.Fatalf("the scope was built %d times in one request, want 1", got)
	}
	if got := countContaining(statements, "SET LOCAL statement_timeout = '"+analyticsStatementTimeout+"'"); got != 4 {
		t.Fatalf("statement timeout set %d times, want once per read and once for the build", got)
	}
	if got := countContaining(statements, "SET LOCAL jit = off"); got != 4 {
		t.Fatalf("jit = off set %d times, want once per read and once for the build", got)
	}
}

func TestAnotherScopeInsideASharedRequestGetsItsOwnTransaction(t *testing.T) {
	db, mock, captured := newCapturingDB(t)
	mock.ExpectBegin()
	expectExecs(mock, 3+scopeBuildStatements)
	expectExecs(mock, 3)
	mock.ExpectBegin()
	expectExecs(mock, 2+scopeBuildStatements)
	mock.ExpectCommit()
	mock.ExpectCommit()

	repo := &repository{db: db}
	first := activityWindow()
	other := activityWindow()
	other.DepartmentID = "dept-1"
	err := repo.ShareScope(context.Background(), func(ctx context.Context) error {
		if err := repo.withScope(ctx, "ws-1", first, func(*gorm.DB) error { return nil }); err != nil {
			return err
		}
		return repo.withScope(ctx, "ws-1", other, func(*gorm.DB) error { return nil })
	})
	if err != nil {
		t.Fatalf("ShareScope() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("a different scope must not reuse the shared temp tables: %v", err)
	}
	if got := countContaining(captured.all(), "CREATE TEMP TABLE "+scopeEntriesTable+" "); got != 2 {
		t.Fatalf("scopes built = %d, want 2", got)
	}
}

func TestAFailedReadInASharedRequestIsReturnedAndRolledBack(t *testing.T) {
	db, mock, _ := newCapturingDB(t)
	mock.ExpectBegin()
	expectExecs(mock, 3+scopeBuildStatements)
	expectExecs(mock, 3)
	expectExecs(mock, 1)
	mock.ExpectRollback()

	boom := errors.New("read failed")
	repo := &repository{db: db}
	err := repo.ShareScope(context.Background(), func(ctx context.Context) error {
		return repo.withScope(ctx, "ws-1", activityWindow(), func(*gorm.DB) error { return boom })
	})
	if !errors.Is(err, boom) {
		t.Fatalf("ShareScope() error = %v, want the read failure", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the shared transaction must roll back after a failure: %v", err)
	}
}

func TestAReadAfterTheSharedRequestEndedRunsAlone(t *testing.T) {
	db, mock, _ := newCapturingDB(t)
	mock.ExpectBegin()
	expectExecs(mock, 2)
	mock.ExpectCommit()

	repo := &repository{db: db}
	var leaked context.Context
	if err := repo.ShareScope(context.Background(), func(ctx context.Context) error {
		leaked = ctx
		return nil
	}); err != nil {
		t.Fatalf("ShareScope() error = %v", err)
	}
	if err := repo.analyticsContext(leaked, func(*gorm.DB) error { return nil }); err != nil {
		t.Fatalf("analyticsContext() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("a read after the request ended must use its own transaction: %v", err)
	}
}

func TestNestedSharesReuseTheOuterOne(t *testing.T) {
	db, mock, _ := newCapturingDB(t)
	mock.ExpectBegin()
	expectExecs(mock, 3)
	expectExecs(mock, 3)
	mock.ExpectCommit()

	repo := &repository{db: db}
	err := repo.ShareScope(context.Background(), func(ctx context.Context) error {
		if err := repo.analyticsContext(ctx, func(*gorm.DB) error { return nil }); err != nil {
			return err
		}
		return repo.ShareScope(ctx, func(ctx context.Context) error {
			return repo.analyticsContext(ctx, func(*gorm.DB) error { return nil })
		})
	})
	if err != nil {
		t.Fatalf("ShareScope() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("a nested share must not open a second transaction: %v", err)
	}
}

func TestTrendRunsInTheAnalyticsSession(t *testing.T) {
	db, mock, captured := newCapturingDB(t)
	mock.ExpectBegin()
	expectExecs(mock, 2)
	mock.ExpectQuery("").WillReturnRows(sqlmock.NewRows([]string{"bucket", "engaged", "finished", "created", "pending"}))
	mock.ExpectQuery("").WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(0))
	mock.ExpectCommit()

	repo := &repository{db: db}
	to := time.Date(2026, 9, 24, 23, 59, 59, 0, time.UTC)
	if _, err := repo.GetTrend(context.Background(), "ws-1", attendance.OverviewFilter{DateTo: &to}, 3, time.UTC); err != nil {
		t.Fatalf("GetTrend() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("trend must run inside one analytics transaction: %v", err)
	}
	joined := strings.Join(captured.all(), "\n")
	if !strings.Contains(joined, "SET LOCAL jit = off") || !strings.Contains(joined, "SET LOCAL statement_timeout = '"+analyticsStatementTimeout+"'") {
		t.Fatalf("trend ran without the analytics settings:\n%s", joined)
	}
}
