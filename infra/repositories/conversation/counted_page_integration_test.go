package conversation_repository

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/conversation"
	"vozko/domain/personalinbox"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func twoScanUnassignedOrMine(column string) string {
	return "(NOT EXISTS (SELECT 1 FROM inbox_assignments ia WHERE ia.entry_id = " + column + ") OR EXISTS (SELECT 1 FROM inbox_assignments ia2 WHERE ia2.entry_id = " + column + " AND ia2.assigned_user_id = ?))"
}

func twoScanAssignedSelf(column string) string {
	return " AND (NOT EXISTS (SELECT 1 FROM inbox_assignments ia_s WHERE ia_s.entry_id = " + column + ")" +
		" OR EXISTS (SELECT 1 FROM inbox_assignments ia_s2 WHERE ia_s2.entry_id = " + column + " AND ia_s2.assigned_user_id = ?))"
}

type assignmentSeed struct {
	exec func(sql string, args ...interface{})
	kind int
}

func (s *assignmentSeed) assign(workspace, entryID string, assignee *string) {
	s.kind++
	s.exec(`INSERT INTO inbox_assignments (id, workspace_id, entry_id, entry_type, assigned_user_id, assignee_kind) VALUES (?, ?, ?, ?, ?, 'human')`,
		uuid.NewString(), workspace, entryID, fmt.Sprintf("kind-%d", s.kind), assignee)
}

func execOrFail(t *testing.T, db *gorm.DB) func(sql string, args ...interface{}) {
	return func(sql string, args ...interface{}) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatalf("exec %q: %v", sql, err)
		}
	}
}

func TestUnassignedOrMineMatchesTheTwoExistsFormAgainstPostgres(t *testing.T) {
	db := repotest.IsolatedDB(t, "unassigned_mine_test", &schema.InboxAssignment{})
	exec := execOrFail(t, db)
	exec(`ALTER TABLE inbox_assignments ALTER COLUMN assigned_user_id DROP NOT NULL`)
	exec(`CREATE TABLE probe_entries (entry_id uuid PRIMARY KEY, label text NOT NULL)`)

	workspace := uuid.NewString()
	me, other, third, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	seed := &assignmentSeed{exec: exec}
	world := map[string][]*string{
		"unassigned":           nil,
		"mine":                 {&me},
		"someone else's":       {&other},
		"null assignee":        {nil},
		"mine and another's":   {&me, &other},
		"two others":           {&other, &third},
		"null and mine":        {nil, &me},
		"null and another's":   {nil, &other},
		"mine on two channels": {&me, &me},
	}
	for label, assignees := range world {
		id := uuid.NewString()
		exec(`INSERT INTO probe_entries (entry_id, label) VALUES (?, ?)`, id, label)
		for _, assignee := range assignees {
			seed.assign(workspace, id, assignee)
		}
	}

	labels := func(predicate, viewer string) []string {
		t.Helper()
		var out []string
		if err := db.Raw(`SELECT label FROM probe_entries pe WHERE `+predicate+` ORDER BY label`, viewer).Scan(&out).Error; err != nil {
			t.Fatalf("query: %v", err)
		}
		return out
	}

	for _, viewer := range []string{me, other, third, stranger} {
		before := labels(twoScanUnassignedOrMine("pe.entry_id"), viewer)
		after := labels(unassignedOrAssignedTo("pe.entry_id"), viewer)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("viewer %s:\n two scans %v\n probe     %v", viewer, before, after)
		}
	}
	if got := labels(unassignedOrAssignedTo("pe.entry_id"), me); !reflect.DeepEqual(got, []string{"mine", "mine and another's", "mine on two channels", "null and mine", "unassigned"}) {
		t.Fatalf("my view = %v", got)
	}
	if got := labels(unassignedOrAssignedTo("pe.entry_id"), stranger); !reflect.DeepEqual(got, []string{"unassigned"}) {
		t.Fatalf("a stranger's view = %v", got)
	}
}

type countedIDRow struct {
	EntryID string `gorm:"column:entry_id"`
	PageTotal
}

func entryIDsOf(rows []countedIDRow) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row.EntryID
	}
	return out
}

type beforeTheFold struct {
	page, count func(string) string
}

func (b beforeTheFold) read(t *testing.T, db *gorm.DB, q countedPage) ([]string, int64) {
	t.Helper()
	var total int64
	if err := db.Raw(b.count(q.countSQL), q.countArgs...).Scan(&total).Error; err != nil {
		t.Fatalf("old count: %v", err)
	}
	if total == 0 {
		return nil, 0
	}
	oldPage := strings.Replace(q.pageSQL, ", "+pageTotalColumn, "", 1)
	if oldPage == q.pageSQL {
		t.Fatalf("the page carries no window total:\n%s", q.pageSQL)
	}
	var rows []struct {
		EntryID string `gorm:"column:entry_id"`
	}
	if err := db.Raw(b.page(oldPage), q.pageArgs...).Scan(&rows).Error; err != nil {
		t.Fatalf("old page: %v", err)
	}
	var ids []string
	for _, row := range rows {
		ids = append(ids, row.EntryID)
	}
	return ids, total
}

func assertFoldMatches(t *testing.T, db *gorm.DB, q countedPage, old beforeTheFold, wantRows bool) {
	t.Helper()
	wantIDs, wantTotal := old.read(t, db, q)
	rows, total, err := readCountedPage[countedIDRow](db, q)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDsOf(rows); len(got) != len(wantIDs) || (len(got) > 0 && !reflect.DeepEqual(got, wantIDs)) {
		t.Fatalf("page = %v, want %v", got, wantIDs)
	}
	if total != wantTotal {
		t.Fatalf("total = %d, want %d", total, wantTotal)
	}
	if wantRows && len(rows) == 0 {
		t.Fatalf("the case was meant to return rows")
	}
}

func replacing(from, to string) func(string) string {
	return func(sql string) string { return strings.ReplaceAll(sql, from, to) }
}

func TestFoldedInboxTotalsMatchTheCountQueryAgainstPostgres(t *testing.T) {
	w := seedInboxWorld(t)
	repo := &repository{db: w.db}
	exec := execOrFail(t, w.db)
	exec(`ALTER TABLE inbox_assignments ALTER COLUMN assigned_user_id DROP NOT NULL`)
	stranger := uuid.NewString()
	seed := &assignmentSeed{exec: exec}
	seed.assign(w.workspace, w.byName["team-at-11"], &w.ana)
	seed.assign(w.workspace, w.byName["customer-at-12"], &w.ana)
	seed.assign(w.workspace, w.byName["customer-at-12"], &w.bia)
	seed.assign(w.workspace, w.byName["pinned-by-ana"], nil)
	seed.assign(w.workspace, w.byName["customer-at-10"], nil)
	seed.assign(w.workspace, w.byName["customer-at-10"], &w.bia)

	ch, _ := channelQueryFor(shared.EntryTypeTelegram)
	all := w.ids("customer-at-10", "team-at-11", "customer-at-9.5", "customer-at-12", "pinned-by-ana", "finished-long-ago-waiting", "finished-recently")
	paths := map[string]struct {
		base   conversation.SearchEntriesInput
		column string
		build  func(in conversation.SearchEntriesInput, pageSize, offset int) countedPage
	}{
		"workspace": {conversation.SearchEntriesInput{WorkspaceID: w.workspace, EntryType: shared.EntryTypeTelegram}, "ae.entry_id",
			func(in conversation.SearchEntriesInput, pageSize, offset int) countedPage {
				q, ok := workspaceInboxQueries(in, pageSize, offset)
				if !ok {
					t.Fatal("no workspace query")
				}
				return q
			}},
		"container": {conversation.SearchEntriesInput{CampaignID: w.account, EntryType: shared.EntryTypeTelegram}, "cm.entry_id",
			func(in conversation.SearchEntriesInput, pageSize, offset int) countedPage {
				return containerInboxQueries(in, ch, nil, pageSize, offset)
			}},
		"entry ids": {conversation.SearchEntriesInput{EntryIDs: all, EntryType: shared.EntryTypeTelegram}, "cm.entry_id",
			func(in conversation.SearchEntriesInput, pageSize, offset int) countedPage {
				return containerInboxQueries(in, ch, all, pageSize, offset)
			}},
	}
	views := map[string]func(*conversation.SearchEntriesInput){
		"everyone":            func(*conversation.SearchEntriesInput) {},
		"ana unassigned+mine": func(in *conversation.SearchEntriesInput) { in.AssignedUserID = w.ana },
		"bia unassigned+mine": func(in *conversation.SearchEntriesInput) { in.AssignedUserID = w.bia },
		"stranger":            func(in *conversation.SearchEntriesInput) { in.AssignedUserID = stranger },
		"ana inbox waiting": func(in *conversation.SearchEntriesInput) {
			in.Personal, in.PersonalUserID, in.SortOrder, in.AssignedUserID = personalinbox.ViewInbox, w.ana, conversation.SortOrderWaiting, w.ana
		},
		"nothing matches": func(in *conversation.SearchEntriesInput) { in.LabelID = uuid.NewString() },
	}
	inboxTotals := map[string]int64{
		"everyone": 5, "ana unassigned+mine": 3, "bia unassigned+mine": 3, "stranger": 1, "ana inbox waiting": 3, "nothing matches": 0,
	}

	for pathName, path := range paths {
		old := beforeTheFold{
			page:  replacing(unassignedOrAssignedTo(path.column), twoScanUnassignedOrMine(path.column)),
			count: replacing(unassignedOrAssignedTo(path.column), twoScanUnassignedOrMine(path.column)),
		}
		for viewName, adjust := range views {
			for _, page := range []int{1, 2, 3, 4} {
				t.Run(fmt.Sprintf("%s/%s/page %d", pathName, viewName, page), func(t *testing.T) {
					in := path.base
					adjust(&in)
					in.Page, in.PageSize = page, 2
					q := path.build(in, 2, (page-1)*2)
					if in.AssignedUserID != "" && (old.page(q.pageSQL) == q.pageSQL || old.count(q.countSQL) == q.countSQL) {
						t.Fatal("the old predicate was not restored, so the comparison would be vacuous")
					}
					assertFoldMatches(t, w.db, q, old, viewName == "everyone" && page < 4)

					wantIDs, wantTotal := old.read(t, w.db, q)
					entries, total, err := repo.SearchEntriesWithMessages(in)
					if err != nil {
						t.Fatal(err)
					}
					got := make([]string, len(entries))
					for i, e := range entries {
						got[i] = e.EntryID
					}
					if total != wantTotal || len(got) != len(wantIDs) || (len(got) > 0 && !reflect.DeepEqual(got, wantIDs)) {
						t.Fatalf("search = %v (%d), want %v (%d)", got, total, wantIDs, wantTotal)
					}
					if total != inboxTotals[viewName] {
						t.Fatalf("total = %d, the seeded world has %d for %s", total, inboxTotals[viewName], viewName)
					}
				})
			}
		}
	}
}

func TestFoldedBoardTotalsMatchTheCountQueryAgainstPostgres(t *testing.T) {
	db := entryUnionDB(t)
	seeded := seedEntryUnion(t, db)
	exec := execOrFail(t, db)
	exec(`CREATE TABLE inbox_assignments (id text PRIMARY KEY, workspace_id uuid, entry_id uuid NOT NULL, entry_type text NOT NULL, assigned_user_id uuid, assignee_kind text)`)
	for _, table := range []string{"whatsapp_campaign_entries", "instagram_conversations", "telegram_conversations", "mercadolivre_conversations",
		"tiktok_conversations", "email_conversations", "rcs_conversations", "sms_conversations"} {
		exec(`UPDATE ` + table + ` SET last_message_at = now() - random() * interval '100 days', created_at = now() - random() * interval '100 days'`)
	}

	me, other, third := uuid.NewString(), uuid.NewString(), uuid.NewString()
	seed := &assignmentSeed{exec: exec}
	ids := seeded.ids
	seed.assign(seeded.workspaceID, ids[0], &me)
	seed.assign(seeded.workspaceID, ids[1], &other)
	seed.assign(seeded.workspaceID, ids[2], nil)
	seed.assign(seeded.workspaceID, ids[3], &me)
	seed.assign(seeded.workspaceID, ids[3], &other)
	seed.assign(seeded.workspaceID, ids[4], &other)
	seed.assign(seeded.workspaceID, ids[4], &third)
	seed.assign(seeded.workspaceID, ids[5], nil)
	seed.assign(seeded.workspaceID, ids[5], &me)

	restore := func(sql string) string {
		for _, src := range entrySources {
			sql = strings.ReplaceAll(sql, " AND "+unassignedOrAssignedTo(src.EntryID), twoScanAssignedSelf(src.EntryID))
		}
		return sql
	}
	old := beforeTheFold{page: restore, count: restore}
	boardTotals := map[string]int64{
		"everyone": 11, "mine": 8, "other": 8, "oldest created": 11, "some excluded": 8, "nothing matches": 0,
	}

	views := map[string]func(*conversation.SearchByFilterInput){
		"everyone":        func(*conversation.SearchByFilterInput) {},
		"mine":            func(in *conversation.SearchByFilterInput) { in.AssignedUserID = me },
		"other":           func(in *conversation.SearchByFilterInput) { in.AssignedUserID = other },
		"oldest created":  func(in *conversation.SearchByFilterInput) { in.SortField, in.SortOrder = "created", "asc" },
		"some excluded":   func(in *conversation.SearchByFilterInput) { in.ExcludeEntryIDs = ids[:3] },
		"nothing matches": func(in *conversation.SearchByFilterInput) { in.ExcludeEntryIDs = ids },
	}
	for viewName, adjust := range views {
		for _, page := range []int{1, 2, 3, 4} {
			t.Run(fmt.Sprintf("%s/page %d", viewName, page), func(t *testing.T) {
				in := conversation.SearchByFilterInput{WorkspaceID: seeded.workspaceID, Page: page, PageSize: 4}
				adjust(&in)
				q, err := filteredPageQueries(in)
				if err != nil {
					t.Fatal(err)
				}
				if in.AssignedUserID != "" && strings.Count(restore(q.pageSQL), "ia_s2.assigned_user_id = ?") != len(entrySources) {
					t.Fatal("the old predicate was not restored on every channel, so the comparison would be vacuous")
				}
				assertFoldMatches(t, db, q, old, viewName == "everyone" && page < 4)
				if _, total, err := readCountedPage[countedIDRow](db, q); err != nil || total != boardTotals[viewName] {
					t.Fatalf("total = %d, %v; the seeded union has %d for %s", total, err, boardTotals[viewName], viewName)
				}
			})
		}
	}
}
