package attendance_repository

import (
	"context"
	"slices"
	"strings"
	"testing"

	"vozko/domain/attendance"
	"vozko/domain/shared"
	"vozko/infra/database/sqlexpr"
)

func serviceMessageQueryFor(from, to string) attendance.ServiceMessageQuery {
	start, _ := attendance.ParseDay(from)
	end, _ := attendance.ParseDay(to)
	end = attendance.EndOfDay(end)
	return attendance.ServiceMessageQuery{Filter: attendance.OverviewFilter{DateFrom: &start, DateTo: &end}}
}

func flatSQL(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}

func TestTheServiceMessageCountInlinesTheChargedRuleSoThePartialIndexStaysUsable(t *testing.T) {
	sql, _ := serviceMessageCountQuery("ws1", serviceMessageQueryFor("2026-10-01", "2026-10-07")).build()
	flat := flatSQL(sql)
	for _, rule := range []string{sqlexpr.ServiceMessage("cm"), sqlexpr.MetaChargedService("cm")} {
		if !strings.Contains(flat, flatSQL(rule)) {
			t.Fatalf("the count does not apply the shared rule %q:\n%s", rule, flat)
		}
	}
	if strings.Count(flat, "?") != 4 {
		t.Fatalf("the count binds %d values, want only the workspace and the three period bounds:\n%s", strings.Count(flat, "?"), flat)
	}
}

func TestTheServiceMessageCountStartsFromTheWorkspaceConversationsActiveInThePeriod(t *testing.T) {
	q := serviceMessageQueryFor("2026-10-01", "2026-10-07")
	sql, args := serviceMessageCountQuery("ws1", q).build()
	flat := flatSQL(sql)
	for _, want := range []string{
		"FROM whatsapp_campaign_entries wce JOIN whatsapp_campaigns wc ON wc.id = wce.campaign_id AND wc.deleted_at IS NULL",
		"JOIN conversation_messages cm ON cm.entry_id = wce.id AND cm.entry_type = 'whatsapp'",
		"WHERE wc.workspace_id = ? AND wce.deleted_at IS NULL AND wce.last_message_at >= ?",
		"AND cm.created_at >= ? AND cm.created_at <= ?",
		"date_trunc('day', cm.created_at AT TIME ZONE 'UTC') AS bucket",
		"GROUP BY 1 ORDER BY 1",
	} {
		if !strings.Contains(flat, want) {
			t.Fatalf("query lacks %q:\n%s", want, flat)
		}
	}
	if len(args) != 4 || args[0] != "ws1" || args[1] != *q.Filter.DateFrom || args[2] != *q.Filter.DateFrom || args[3] != *q.Filter.DateTo {
		t.Fatalf("args = %v", args)
	}
}

func TestTheServiceMessageCountAppliesEveryAttendanceFilterAndTheCallerScope(t *testing.T) {
	q := serviceMessageQueryFor("2026-10-01", "2026-10-07")
	q.Filter.DepartmentID = "d1"
	q.Filter.MemberID = "m1"
	q.Filter.CampaignID = "c1"
	q.Filter.Origin = "receptive"
	q.Departments = []string{"d1", "d2"}
	sql, args := serviceMessageCountQuery("ws1", q).build()
	flat := flatSQL(sql)
	for _, want := range []string{
		"AND wce.campaign_id = ?",
		"AND wc.department_id = ?",
		"AND ia.assigned_user_id = ?",
		"AND wc.department_id::text IN ?",
	} {
		if !strings.Contains(flat, want) {
			t.Fatalf("query lacks %q:\n%s", want, flat)
		}
	}
	if !slices.ContainsFunc(args, func(arg interface{}) bool {
		departments, ok := arg.([]string)
		return ok && slices.Equal(departments, []string{"d1", "d2"})
	}) {
		t.Fatalf("the caller's departments are not bound: %v", args)
	}
	if !slices.Contains(args, interface{}("receptive")) {
		t.Fatalf("the origin is not bound: %v", args)
	}
}

func TestTheServiceMessageTrendBucketsFollowTheGranularity(t *testing.T) {
	sql, _ := serviceMessageCountQuery("ws1", serviceMessageQueryFor("2025-01-01", "2026-10-01")).build()
	if !strings.Contains(flatSQL(sql), "date_trunc('month', cm.created_at AT TIME ZONE 'UTC')") {
		t.Fatalf("a long period is not bucketed by month:\n%s", sql)
	}
}

func TestServiceMessagesOfAnotherChannelAreNeverRead(t *testing.T) {
	db, _, captured := newCapturingDB(t)
	q := serviceMessageQueryFor("2026-10-01", "2026-10-07")
	q.Filter.Channel = string(shared.EntryTypeInstagram)
	rows, err := NewServiceMessageReader(db).ReadServiceMessages(context.Background(), "ws1", q)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows = %v err = %v, want nothing", rows, err)
	}
	if len(captured.all()) != 0 {
		t.Fatalf("another channel reached the database: %v", captured.all())
	}
}

func TestServiceMessagesWithoutAPeriodAreRefusedBeforeAnyRead(t *testing.T) {
	db, _, captured := newCapturingDB(t)
	_, err := NewServiceMessageReader(db).ReadServiceMessages(context.Background(), "ws1", attendance.ServiceMessageQuery{})
	if err == nil {
		t.Fatalf("a read without a period was accepted")
	}
	if len(captured.all()) != 0 {
		t.Fatalf("a read without a period reached the database")
	}
}
