package attendance_repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/attendance"
	"vozko/infra/repositories/repotest"
)

type serviceMessageSeed struct {
	db  *gorm.DB
	t   *testing.T
	day time.Time
}

func (s serviceMessageSeed) exec(sql string, args ...interface{}) {
	s.t.Helper()
	if err := s.db.Exec(sql, args...).Error; err != nil {
		s.t.Fatalf("seed: %v\n%s", err, sql)
	}
}

func (s serviceMessageSeed) campaign(workspaceID string, departmentID interface{}) string {
	id := uuid.NewString()
	s.exec(`INSERT INTO whatsapp_campaigns (id, workspace_id, department_id, name, created_at, updated_at) VALUES (?, ?, ?, 'Atendimento', ?, ?)`,
		id, workspaceID, departmentID, s.day.AddDate(0, -1, 0), s.day.AddDate(0, -1, 0))
	return id
}

func (s serviceMessageSeed) conversation(workspaceID, campaignID, memberID string, lastMessageAt time.Time) string {
	id := uuid.NewString()
	s.exec(`INSERT INTO whatsapp_campaign_entries (id, campaign_id, lead_id, conversation_status, last_message_at, created_at, updated_at)
		VALUES (?, ?, ?, 'ongoing', ?, ?, ?)`, id, campaignID, uuid.NewString(), lastMessageAt, s.day.AddDate(0, 0, -20), s.day.AddDate(0, 0, -20))
	if memberID != "" {
		s.exec(`INSERT INTO inbox_assignments (id, entry_id, entry_type, assigned_user_id, workspace_id, created_at, updated_at) VALUES (?, ?, 'whatsapp', ?, ?, ?, ?)`,
			uuid.NewString(), id, memberID, workspaceID, s.day, s.day)
	}
	return id
}

type seededMessage struct {
	entryType   string
	messageType string
	direction   string
	status      string
	sentVia     string
	billable    interface{}
	category    string
	origin      string
	at          time.Time
	deleted     bool
}

func chargedAt(at time.Time) seededMessage {
	return seededMessage{entryType: "whatsapp", messageType: "operator", direction: "OUTBOUND", status: "delivered", billable: true, category: "service", at: at}
}

func (s serviceMessageSeed) message(entryID string, m seededMessage) {
	var deletedAt interface{}
	if m.deleted {
		deletedAt = m.at
	}
	s.exec(`INSERT INTO conversation_messages (id, entry_id, entry_type, message_type, direction, text, sender_kind, delivery_status, sent_via,
			meta_pricing_billable, meta_pricing_category, meta_conversation_origin, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, 'oi', 'human', ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), entryID, m.entryType, m.messageType, m.direction, m.status, m.sentVia, m.billable, m.category, m.origin, m.at, m.at, deletedAt)
}

func countServiceMessages(t *testing.T, reader attendance.ServiceMessageReader, workspaceID string, q attendance.ServiceMessageQuery) (int64, map[string]int64) {
	t.Helper()
	rows, err := reader.ReadServiceMessages(context.Background(), workspaceID, q)
	if err != nil {
		t.Fatalf("ReadServiceMessages: %v", err)
	}
	section := attendance.BuildServiceMessages(q, rows)
	byDay := map[string]int64{}
	for _, b := range section.Buckets {
		if b.Messages > 0 {
			byDay[b.Bucket] = b.Messages
		}
	}
	return *section.Total, byDay
}

func TestServiceMessagesCountOnlyWhatMetaChargedInTheCallerScopeAgainstPostgres(t *testing.T) {
	db := repotest.IsolatedDB(t, "att_svcmsg", attendanceModels()...)
	day := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	seed := serviceMessageSeed{db: db, t: t, day: day}
	paying := seedAttendanceWorkspace(t, db, day)
	other := uuid.NewString()
	d1, d2 := uuid.NewString(), uuid.NewString()
	m1, m2 := uuid.NewString(), uuid.NewString()

	salesCampaign := seed.campaign(paying, d1)
	supportCampaign := seed.campaign(paying, d2)
	otherCampaign := seed.campaign(other, nil)

	sales := seed.conversation(paying, salesCampaign, m1, day.Add(time.Hour))
	support := seed.conversation(paying, supportCampaign, m2, day.AddDate(0, 0, 1))
	stale := seed.conversation(paying, salesCampaign, m1, day.AddDate(0, 0, -30))
	foreign := seed.conversation(other, otherCampaign, "", day)

	seed.message(sales, chargedAt(day))
	seed.message(sales, chargedAt(day.Add(time.Minute)))
	seed.message(support, chargedAt(day.AddDate(0, 0, 1)))
	seed.message(foreign, chargedAt(day))
	seed.message(stale, chargedAt(day.AddDate(0, 0, -30)))

	notCharged := []func(seededMessage) seededMessage{
		func(m seededMessage) seededMessage { m.billable = false; return m },
		func(m seededMessage) seededMessage { m.billable = nil; return m },
		func(m seededMessage) seededMessage { m.category = "utility"; return m },
		func(m seededMessage) seededMessage { m.origin = "referral_conversion"; return m },
		func(m seededMessage) seededMessage { m.messageType = "template"; return m },
		func(m seededMessage) seededMessage { m.direction = "INBOUND"; return m },
		func(m seededMessage) seededMessage { m.status = "failed"; return m },
		func(m seededMessage) seededMessage { m.sentVia = "business_app"; return m },
		func(m seededMessage) seededMessage { m.deleted = true; return m },
		func(m seededMessage) seededMessage { m.entryType = "unofficial_whatsapp"; return m },
		func(m seededMessage) seededMessage { m.at = day.AddDate(0, 0, 9); return m },
	}
	for _, change := range notCharged {
		seed.message(sales, change(chargedAt(day.Add(2*time.Minute))))
	}

	reader := NewServiceMessageReader(db)
	period := serviceMessageQueryFor("2026-10-08", "2026-10-14")

	total, byDay := countServiceMessages(t, reader, paying, period)
	if total != 3 || byDay["2026-10-10"] != 2 || byDay["2026-10-11"] != 1 {
		t.Fatalf("paying workspace counted %d %v, want 3 (2 on the 10th, 1 on the 11th)", total, byDay)
	}

	if total, _ := countServiceMessages(t, reader, other, period); total != 1 {
		t.Fatalf("the other workspace counted %d, want only its own message", total)
	}

	cases := map[string]struct {
		mutate func(*attendance.ServiceMessageQuery)
		want   int64
	}{
		"the department filter":        {func(q *attendance.ServiceMessageQuery) { q.Filter.DepartmentID = d1 }, 2},
		"the caller's departments":     {func(q *attendance.ServiceMessageQuery) { q.Departments = []string{d2} }, 1},
		"both of the caller's":         {func(q *attendance.ServiceMessageQuery) { q.Departments = []string{d1, d2} }, 3},
		"a department nobody uses":     {func(q *attendance.ServiceMessageQuery) { q.Departments = []string{uuid.NewString()} }, 0},
		"the member filter":            {func(q *attendance.ServiceMessageQuery) { q.Filter.MemberID = m2 }, 1},
		"the campaign filter":          {func(q *attendance.ServiceMessageQuery) { q.Filter.CampaignID = salesCampaign }, 2},
		"the official WhatsApp filter": {func(q *attendance.ServiceMessageQuery) { q.Filter.Channel = "whatsapp" }, 3},
		"another channel":              {func(q *attendance.ServiceMessageQuery) { q.Filter.Channel = "instagram" }, 0},
		"a period before the messages": {func(q *attendance.ServiceMessageQuery) { *q = serviceMessageQueryFor("2026-09-01", "2026-09-05") }, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			q := serviceMessageQueryFor("2026-10-08", "2026-10-14")
			tc.mutate(&q)
			if total, _ := countServiceMessages(t, reader, paying, q); total != tc.want {
				t.Fatalf("counted %d, want %d", total, tc.want)
			}
		})
	}
}
