package balance_repository

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"vozko/domain/balance"
)

func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	mock.MatchExpectationsInOrder(false)
	db, err := gorm.Open(
		postgres.New(postgres.Config{
			Conn:                 sqlDB,
			PreferSimpleProtocol: true,
			WithoutReturning:     true,
		}),
		&gorm.Config{SkipDefaultTransaction: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	return db, mock, sqlDB
}

func TestDebitBalance_PersistsIsRefund(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	repo := NewRepository(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "balances"`).
		WithArgs("ws-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "amount", "currency"}).
			AddRow("bal-1", "ws-1", int64(10_000_000), "USD"))
	mock.ExpectExec(`INSERT INTO "balance_transactions".*"is_refund"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE "balances"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := repo.DebitBalance(balance.DebitBalanceInput{
		WorkspaceID:        "ws-1",
		Amount:             5_000_000,
		ServiceType:        balance.ServiceTopUp,
		IsRefund:           true,
		ExchangeRateMicros: 6_000_000,
	})
	if err != nil {
		t.Fatalf("DebitBalance error: %v", err)
	}
	if tx == nil || !tx.IsRefund {
		t.Fatalf("expected returned transaction with IsRefund=true, got %+v", tx)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestDebitBalance_DefaultsIsRefundFalse(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	repo := NewRepository(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "balances"`).
		WithArgs("ws-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "amount", "currency"}).
			AddRow("bal-1", "ws-1", int64(10_000_000), "USD"))
	mock.ExpectQuery(`SELECT \(SELECT COALESCE`).
		WillReturnRows(sqlmock.NewRows([]string{"campaign"}).AddRow("camp-1"))
	mock.ExpectExec(`INSERT INTO "balance_transactions".*"charge_campaign_id"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE "balances"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := repo.DebitBalance(balance.DebitBalanceInput{
		WorkspaceID:        "ws-1",
		Amount:             1_000_000,
		ServiceType:        balance.ServiceWhatsAppCampaign,
		ExchangeRateMicros: 6_000_000,
	})
	if err != nil {
		t.Fatalf("DebitBalance error: %v", err)
	}
	if tx == nil || tx.IsRefund {
		t.Fatalf("expected returned transaction with IsRefund=false, got %+v", tx)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestGetFullBalanceSummary_ServesCurrentRunningTotalsWithoutSummingTheLedger(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	stamp := time.Date(2026, 10, 24, 12, 0, 0, 123000, time.UTC)

	mock.ExpectQuery(`SELECT .* FROM "balances"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "amount", "currency", "updated_at", "total_money_credits", "total_money_debits", "money_totals_at"}).
			AddRow("bal-1", "ws-1", int64(700), "USD", stamp, int64(1_000), int64(300), stamp))

	summary, err := NewRepository(db).GetFullBalanceSummary("ws-1")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.TotalMoneyCredits != 1_000 || summary.TotalMoneyDebits != 300 || summary.Balance.Amount != 700 {
		t.Fatalf("summary = %+v", summary)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestGetFullBalanceSummary_SumsTheLedgerWhenTheTotalsAreNotCurrent(t *testing.T) {
	stamp := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for name, marker := range map[string]interface{}{"never counted": nil, "moved since": stamp.Add(-time.Second)} {
		t.Run(name, func(t *testing.T) {
			db, mock, sqlDB := newMockDB(t)
			defer sqlDB.Close()
			mock.ExpectQuery(`SELECT .* FROM "balances"`).
				WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "amount", "currency", "updated_at", "total_money_credits", "total_money_debits", "money_totals_at"}).
					AddRow("bal-1", "ws-1", int64(700), "USD", stamp, int64(1), int64(1), marker))
			mock.ExpectQuery(`SELECT resource_type, type, COALESCE\(SUM\(amount\), 0\) as total FROM "balance_transactions" WHERE balance_id = .* GROUP BY resource_type, type`).
				WillReturnRows(sqlmock.NewRows([]string{"resource_type", "type", "total"}).
					AddRow("money", "credit", int64(1_000)).AddRow("money", "debit", int64(300)))

			summary, err := NewRepository(db).GetFullBalanceSummary("ws-1")
			if err != nil {
				t.Fatalf("summary: %v", err)
			}
			if summary.TotalMoneyCredits != 1_000 || summary.TotalMoneyDebits != 300 {
				t.Fatalf("summary = %+v", summary)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sqlmock expectations: %v", err)
			}
		})
	}
}

func TestCampaignChargeQueriesBindOneArgumentPerPlaceholder(t *testing.T) {
	from, to := time.Now().Add(-time.Hour), time.Now()
	filter := balance.WhatsAppChargeFilter{WorkspaceID: "ws-1", CampaignType: "standard", DepartmentIDs: []string{"d-1", "d-2"}, From: &from, To: &to}
	for name, q := range map[string]chargeQuery{
		"by campaign":     chargesByCampaign(filter, "ws-1"),
		"by owner":        chargesByOwner(filter, "ws-1"),
		"whole workspace": chargesByOwner(balance.WhatsAppChargeFilter{WorkspaceID: "ws-1"}, "ws-1"),
	} {
		if got := strings.Count(q.sql, "?"); got != len(q.args) {
			t.Fatalf("%s: %d placeholders for %d arguments", name, got, len(q.args))
		}
	}
}
