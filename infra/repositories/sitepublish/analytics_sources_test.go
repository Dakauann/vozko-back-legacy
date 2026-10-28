package sitepublish_repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"vozko/domain/sitepublish"
)

func TestAnalyticsSourcesUpsertReplacesTheDayCounts(t *testing.T) {
	if !strings.Contains(upsertSourcesSQL, "ON CONFLICT (project_id, day, kind, value) DO UPDATE SET views = EXCLUDED.views, submits = EXCLUDED.submits") {
		t.Fatalf("statement = %s", upsertSourcesSQL)
	}
	db, mock := mockDB(t)
	day := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(sqlPattern(upsertSourcesSQL)).
		WithArgs(projectID, day, "utm", "instagram", int64(12), int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err := NewAnalytics(db).UpsertSources(context.Background(), []sitepublish.SourceStat{{ProjectID: projectID, Day: day, Kind: sitepublish.SourceUTM, Value: "instagram", Views: 12, Submits: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := NewAnalytics(db).UpsertSources(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyticsSourcesListTheProjectRange(t *testing.T) {
	db, mock := mockDB(t)
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT \* FROM "site_analytics_sources" WHERE project_id = \$1 AND day >= \$2 AND day <= \$3 ORDER BY day, kind, value`).
		WithArgs(projectID, from, to).
		WillReturnRows(sqlmock.NewRows([]string{"project_id", "day", "kind", "value", "views", "submits"}).AddRow(projectID, from, "referrer", "https://www.google.com", 7, 1))
	rows, err := NewAnalytics(db).ListSources(context.Background(), projectID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0] != (sitepublish.SourceStat{ProjectID: projectID, Day: from, Kind: sitepublish.SourceReferrer, Value: "https://www.google.com", Views: 7, Submits: 1}) {
		t.Fatalf("got %+v", rows)
	}
}
