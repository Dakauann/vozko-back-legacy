package opportunity_repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"vozko/domain/opportunity"
	crmfiltersql "vozko/infra/repositories/crmfilter"
)

func TestTheDealsOfManyLeadsUseTheSameMembershipAsOne(t *testing.T) {
	single, _ := leadDealsMembership("d", "ws-1", "lead-1")
	many, args := leadSetDealsMembership("d", "ws-1", []string{"lead-1", "lead-2"})
	want := "d.id IN (SELECT od.id FROM opportunities od WHERE od.workspace_id = ? AND od.lead_id = ANY(?::uuid[]) AND od.deleted_at IS NULL" +
		" UNION SELECT oc.opportunity_id FROM opportunity_conversations oc JOIN " + crmfiltersql.LeadEntriesSource() +
		" ON lead_entries.entry_id = oc.entry_id AND lead_entries.entry_type = oc.entry_type WHERE lead_entries.lead_id = ANY(?::uuid[]))"
	if many != want || len(args) != 3 || placeholdersIn(many) != 3 {
		t.Fatalf("membership = %q args %v", many, args)
	}
	if regexp.MustCompile(`= ANY\(\?::uuid\[\]\)`).ReplaceAllString(many, "= ?") != single {
		t.Fatal("one lead and many leads must share the membership rule")
	}
}

func TestWonDealsOfLeadsTalliesWinsInTheRangeInsideTheScope(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := &repository{db: db}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	scope := opportunity.DealScope{Restrict: true, AssigneeOverride: "u-1"}
	mock.ExpectQuery(`^SELECT COALESCE\(NULLIF\(o\.currency, ''\), 'BRL'\) AS currency, COUNT\(\*\)::bigint AS won_count, COALESCE\(SUM\(o\.value_cents\), 0\)::bigint AS value_cents FROM opportunities o WHERE o\.workspace_id = \$1 AND o\.deleted_at IS NULL AND o\.id IN \(SELECT od\.id FROM opportunities od WHERE od\.workspace_id = \$2 AND od\.lead_id = ANY\(\$3::uuid\[\]\) .*\) AND \(o\.owner_id = \$5\) AND o\.status = 'won' AND o\.close_date IS NOT NULL AND o\.close_date >= \$6 AND o\.close_date < \$7 GROUP BY 1 ORDER BY 3 DESC, 1$`).
		WithArgs("ws-1", "ws-1", sqlmock.AnyArg(), sqlmock.AnyArg(), "u-1", from, to).
		WillReturnRows(sqlmock.NewRows([]string{"currency", "won_count", "value_cents"}).AddRow("BRL", 2, 150000))
	got, err := repo.WonDealsOfLeads(context.Background(), "ws-1", []string{"lead-1"}, from, to, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (opportunity.WonTotal{Currency: "BRL", Count: 2, ValueCents: 150000}) {
		t.Fatalf("got %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	none, err := repo.WonDealsOfLeads(context.Background(), "ws-1", nil, from, to, scope)
	if err != nil || len(none) != 0 {
		t.Fatalf("no leads, no query and no wins: %+v %v", none, err)
	}
	if _, err := repo.WonDealsOfLeads(context.Background(), "", []string{"lead-1"}, from, to, scope); err == nil {
		t.Fatal("a missing workspace refuses")
	}
}
