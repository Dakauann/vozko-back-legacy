package sitepublish_repository

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vozko/domain/sitepublish"
)

func mockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
	return db, mock
}

func sqlPattern(query string) string {
	return "^" + strings.ReplaceAll(regexp.QuoteMeta(query), `\?`, `\$[0-9]+`) + "$"
}

const projectID = "8f6c2a52-1b0f-4f7e-9a34-0d5f8f1e2c3b"

func TestActiveHostnamesReadsOnlyServingRows(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(sqlPattern(activeHostnamesSQL)).WithArgs(projectID, string(sitepublish.HostnameActive)).
		WillReturnRows(sqlmock.NewRows([]string{"hostname"}).AddRow("a.paginas.example").AddRow("www.cliente.com.br"))
	got, err := NewHostnames(db).ActiveHostnames(context.Background(), projectID)
	if err != nil || len(got) != 2 || got[1] != "www.cliente.com.br" {
		t.Fatalf("got %v %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestActiveHostnamesSurfacesStoreErrors(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(sqlPattern(activeHostnamesSQL)).WillReturnError(errors.New("down"))
	if _, err := NewHostnames(db).ActiveHostnames(context.Background(), projectID); err == nil {
		t.Fatal("a store error must refuse, never an empty list")
	}
}

func TestCreatingATakenHostnameRefusesWithTheDomainError(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "site_hostnames"`).WillReturnError(&pgconn.PgError{Code: "23505"})
	mock.ExpectRollback()
	err := NewHostnames(db).Create(context.Background(), sitepublish.HostnameRecord{
		ID: "11111111-1111-1111-1111-111111111111", WorkspaceID: projectID, ProjectID: projectID,
		Hostname: "a.paginas.example", Kind: sitepublish.HostnamePlatform, Status: sitepublish.HostnameActive,
	})
	if !errors.Is(err, sitepublish.ErrHostnameTaken) {
		t.Fatalf("got %v", err)
	}
}

func TestActivateSupersedesThePreviousDeploymentInOneTransaction(t *testing.T) {
	for _, fragment := range []string{"status = 'superseded'", "status = 'active'", "status IN ('ready','superseded')"} {
		if !strings.Contains(supersedeSQL+activateSQL, fragment) {
			t.Fatalf("statements miss %q", fragment)
		}
	}
	db, mock := mockDB(t)
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(sqlPattern(supersedeSQL)).WithArgs(projectID, "dep-2").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlPattern(activateSQL)).WithArgs("person:u1", at, "dep-2", projectID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := NewDeployments(db).Activate(context.Background(), projectID, "dep-2", "person:u1", at); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestActivateRefusesAnUnactivatableDeploymentAndRollsBack(t *testing.T) {
	db, mock := mockDB(t)
	at := time.Now()
	mock.ExpectBegin()
	mock.ExpectExec(sqlPattern(supersedeSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlPattern(activateSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	err := NewDeployments(db).Activate(context.Background(), projectID, "dep-9", "person:u1", at)
	if !errors.Is(err, sitepublish.ErrDeploymentTransition) {
		t.Fatalf("got %v", err)
	}
}

func TestAnalyticsUpsertReplacesTheDayCounts(t *testing.T) {
	if !strings.Contains(upsertDailySQL, "ON CONFLICT (project_id, day, route, variant) DO UPDATE SET views = EXCLUDED.views") {
		t.Fatalf("statement = %s", upsertDailySQL)
	}
	db, mock := mockDB(t)
	day := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(sqlPattern(upsertDailySQL)).
		WithArgs(projectID, day, "/", "control", int64(10), int64(7), int64(3), int64(4), int64(2), int64(1), int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err := NewAnalytics(db).Upsert(context.Background(), []sitepublish.DailyStat{{
		ProjectID: projectID, Day: day, Route: "/", Variant: "control", Views: 10, Visitors: 7, CTA: 3, FormViews: 4, FormStarts: 2, FormSubmits: 1, WhatsApp: 5,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSuspendedProjectChecksOpenAdminSuspensions(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(sqlPattern(suspendedSQL)).WithArgs(projectID, string(sitepublish.AbuseSuspended)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	suspended, err := NewAbuseCases(db).Suspended(context.Background(), projectID)
	if err != nil || !suspended {
		t.Fatalf("got %v %v", suspended, err)
	}
}

func TestHostnamesForSeveralProjectsComeFromOneWorkspaceScopedRead(t *testing.T) {
	db, mock := mockDB(t)
	other := "1d2c3b4a-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
	mock.ExpectQuery(`SELECT \* FROM "site_hostnames" WHERE workspace_id = \$1 AND project_id IN \(\$2,\$3\)`).WithArgs("ws-1", projectID, other).
		WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "project_id", "hostname", "kind", "status", "verification"}).
			AddRow("h1", "ws-1", projectID, "a.paginas.example", "platform", "active", "{}").
			AddRow("h2", "ws-1", other, "www.cliente.com.br", "custom", "active", "{}"))
	got, err := NewHostnames(db).ListByProjects(context.Background(), "ws-1", []string{projectID, other})
	if err != nil || len(got) != 2 || got[1].ProjectID != other {
		t.Fatalf("got %+v %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHostnamesForNoProjectsSkipTheRead(t *testing.T) {
	db, mock := mockDB(t)
	got, err := NewHostnames(db).ListByProjects(context.Background(), "ws-1", nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAHostnamePageWithoutALimitRefuses(t *testing.T) {
	db, _ := mockDB(t)
	page := sitepublish.HostnamePage{Statuses: sitepublish.ProviderlessStatuses(), Kind: sitepublish.HostnameCustom, HoldingProviderRecord: true}
	if _, err := NewHostnames(db).ListPage(context.Background(), page); err == nil {
		t.Fatal("an unbounded page is refused, never read whole")
	}
}
