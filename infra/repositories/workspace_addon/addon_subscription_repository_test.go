package workspace_addon_repository

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
		WithoutReturning:     true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock, sqlDB
}

func TestListReactivatableByWorkspace_FilterAndMapping(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := NewAddonSubscriptionRepository(db)

	since := time.Date(2026, 2, 23, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT \* FROM "workspace_addon_subscriptions".*cancelled_at IS NULL.*current_period_end >=`).
		WithArgs(
			"ws-1",
			string(workspace_plan.SubscriptionStatusActive),
			string(workspace_plan.SubscriptionStatusExpired),
			since,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "workspace_id", "addon_definition_id", "addon_key", "entitlement_kind",
			"quantity", "units_per_quantity", "billing_cycle", "status", "unit_price_micros",
			"current_period_start", "current_period_end", "cancelled_at",
		}).AddRow(
			"addon-1", "ws-1", "def-1", "whatsapp_channel", "whatsapp_business_phones",
			1, 1, "monthly", string(workspace_plan.SubscriptionStatusExpired), int64(25_000_000),
			periodEnd.AddDate(0, -1, 0), periodEnd, nil,
		))

	out, err := repo.ListReactivatableByWorkspace("ws-1", since)
	if err != nil {
		t.Fatalf("ListReactivatableByWorkspace: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one row mapped, got %d", len(out))
	}
	if out[0].ID != "addon-1" || out[0].Status != workspace_plan.SubscriptionStatusExpired {
		t.Fatalf("row mapped wrong: %+v", out[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("query did not match the expected revival filter: %v", err)
	}
}

func TestUnitsLockHoldsAnAdvisoryLockForTheWholeChange(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := NewAddonSubscriptionRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtextextended\(\$1::text, 0\)\)`).
		WithArgs("addon_units:ws-1:site_domains").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT \* FROM "workspace_addon_subscriptions" WHERE workspace_id = \$1 AND entitlement_kind = \$2 AND status = \$3`).
		WithArgs("ws-1", "site_domains", string(workspace_plan.SubscriptionStatusActive)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()

	err := repo.WithUnitsLock("ws-1", workspace_addon.EntitlementSiteDomains, func(locked workspace_addon.AddonSubscriptionRepository) error {
		_, err := locked.ListActiveByWorkspaceAndKind("ws-1", workspace_addon.EntitlementSiteDomains)
		return err
	})
	if err != nil {
		t.Fatalf("WithUnitsLock: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the reads must run inside the locked transaction: %v", err)
	}
}

func TestUnitsLockRollsBackWhenTheChangeFails(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := NewAddonSubscriptionRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	boom := errors.New("save failed")
	err := repo.WithUnitsLock("ws-1", workspace_addon.EntitlementSiteDomains, func(workspace_addon.AddonSubscriptionRepository) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("the failure must surface, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspacesWithAddonsDuePageByWorkspaceText(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := NewAddonSubscriptionRepository(db)
	through := time.Date(2026, 3, 23, 3, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT DISTINCT workspace_id::text FROM workspace_addon_subscriptions WHERE status = \$1 AND current_period_end <= \$2 AND workspace_id::text > \$3 ORDER BY 1 LIMIT \$4`).
		WithArgs(string(workspace_plan.SubscriptionStatusActive), through, "", 2).
		WillReturnRows(sqlmock.NewRows([]string{"workspace_id"}).AddRow("ws-1").AddRow("ws-2"))

	got, err := repo.ListWorkspaceIDsWithAddonsDue(through, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "ws-1" || got[1] != "ws-2" {
		t.Fatalf("got %v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
