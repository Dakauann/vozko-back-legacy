package siteform_repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	crmfiltersql "vozko/infra/repositories/crmfilter"
)

func TestPageLeadsAreSubmissionsAndConversationsThatCameFromThePage(t *testing.T) {
	db, mock := mockDB(t)
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 30)
	query := "SELECT DISTINCT page_leads.lead_id FROM (SELECT s.lead_id FROM site_submissions s WHERE s.workspace_id = ? AND s.project_id = ? AND s.lead_id IS NOT NULL AND s.created_at >= ? AND s.created_at < ?" +
		" UNION SELECT lead_entries.lead_id FROM conversation_page_origins o JOIN " + crmfiltersql.LeadEntriesSource() +
		" ON lead_entries.entry_id = o.entry_id AND lead_entries.entry_type = o.entry_type WHERE o.workspace_id = ? AND o.project_id = ? AND o.arrived_at >= ? AND o.arrived_at < ?) page_leads WHERE page_leads.lead_id IS NOT NULL"
	mock.ExpectQuery(sqlPattern(query)).
		WithArgs("ws-1", "p-1", from, to, "ws-1", "p-1", from, to).
		WillReturnRows(sqlmock.NewRows([]string{"lead_id"}).AddRow("lead-1").AddRow("lead-2"))
	leads, err := NewPageLeads(db).Leads(context.Background(), "ws-1", "p-1", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(leads) != 2 || leads[0] != "lead-1" || leads[1] != "lead-2" {
		t.Fatalf("got %v", leads)
	}
	done(t, mock)
	if _, err := NewPageLeads(db).Leads(context.Background(), "", "p-1", from, to); err == nil {
		t.Fatal("a missing workspace refuses")
	}
}
