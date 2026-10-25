package conversation_repository

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

func TestUnassignedOrMineIsOneProbePerEntry(t *testing.T) {
	want := "COALESCE((SELECT bool_or(ia_s.assigned_user_id IS NOT DISTINCT FROM ?::uuid) FROM inbox_assignments ia_s WHERE ia_s.entry_id = ae.entry_id), true)"
	if got := unassignedOrAssignedTo("ae.entry_id"); got != want {
		t.Fatalf("probe =\n%s\nwant\n%s", got, want)
	}
}

func TestAssignedSelfClauseIsTheSharedProbe(t *testing.T) {
	clause, args := assignedSelfClause("wce.id", "user-1")
	if clause != " AND "+unassignedOrAssignedTo("wce.id") {
		t.Fatalf("clause = %q", clause)
	}
	if !reflect.DeepEqual(args, []interface{}{"user-1"}) {
		t.Fatalf("args = %#v", args)
	}
	if clause, args := assignedSelfClause("wce.id", ""); clause != "" || args != nil {
		t.Fatalf("no viewer means no clause, got %q %v", clause, args)
	}
}

func TestWorkspaceInboxUnassignedOrMineIsTheSharedProbe(t *testing.T) {
	conditions, args := workspaceInboxConditions(conversation.SearchEntriesInput{AssignedUserID: "user-1"})
	if !reflect.DeepEqual(conditions, []string{unassignedOrAssignedTo("ae.entry_id")}) {
		t.Fatalf("conditions = %#v", conditions)
	}
	if !reflect.DeepEqual(args, []interface{}{"user-1"}) {
		t.Fatalf("args = %#v", args)
	}
}

func boundProbe(column string) *regexp.Regexp {
	return regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta(unassignedOrAssignedTo(column)), `\?`, `\$\d+`))
}

func capturedQueries(t *testing.T, run func(*repository)) []string {
	t.Helper()
	var seen []string
	matcher := sqlmock.QueryMatcherFunc(func(_, actual string) error {
		seen = append(seen, actual)
		return nil
	})
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	for range 4 {
		mock.ExpectQuery(".").WillReturnError(errors.New("stop"))
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	run(&repository{db: db})
	if len(seen) == 0 {
		t.Fatal("no query was issued")
	}
	return seen
}

func assertOnlyTheSharedProbe(t *testing.T, sql string, columns ...string) {
	t.Helper()
	for _, column := range columns {
		if !boundProbe(column).MatchString(sql) {
			t.Fatalf("the list does not use the shared probe on %s:\n%s", column, sql)
		}
	}
	if strings.Contains(sql, "NOT EXISTS (SELECT 1 FROM inbox_assignments") {
		t.Fatalf("the two-scan form is still there:\n%s", sql)
	}
}

func TestEveryUnassignedOrMineListUsesTheSharedProbe(t *testing.T) {
	sourceColumns := make([]string, 0, len(entrySources))
	for _, src := range entrySources {
		sourceColumns = append(sourceColumns, src.EntryID)
	}
	cases := map[string]struct {
		columns []string
		run     func(*repository)
	}{
		"workspace inbox": {[]string{"ae.entry_id"}, func(r *repository) {
			_, _, _ = r.SearchEntriesWithMessages(conversation.SearchEntriesInput{WorkspaceID: "ws-1", EntryType: shared.EntryTypeTelegram, AssignedUserID: "user-1"})
		}},
		"container inbox": {[]string{"cm.entry_id"}, func(r *repository) {
			_, _, _ = r.SearchEntriesWithMessages(conversation.SearchEntriesInput{CampaignID: "c-1", EntryType: shared.EntryTypeTelegram, AssignedUserID: "user-1"})
		}},
		"entries by id": {[]string{"u.entry_id"}, func(r *repository) {
			_, _, _ = r.GetEntriesWithMessages("", []string{"e-1"}, shared.EntryTypeTelegram, 1, 20, "user-1")
		}},
		"board": {sourceColumns, func(r *repository) {
			in := selectionInput()
			in.AssignedUserID = "user-1"
			_, _, _ = r.SearchEntriesByFilter(in)
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			assertOnlyTheSharedProbe(t, capturedQueries(t, c.run)[0], c.columns...)
		})
	}
}

func TestContainerEntriesUseTheSharedProbe(t *testing.T) {
	for _, ch := range channelQueries {
		t.Run(string(ch.EntryType), func(t *testing.T) {
			sql := capturedQueries(t, func(r *repository) {
				_, _, _ = r.GetEntriesWithMessagesForContainer("c-1", conversation.ContainerKindAccount, nil, ch.EntryType, 1, 20, "user-1")
			})[0]
			if !strings.Contains(sql, "bool_or(ia_s.assigned_user_id IS NOT DISTINCT FROM $") {
				t.Fatalf("the container list does not use the shared probe:\n%s", sql)
			}
			if strings.Contains(sql, "NOT EXISTS (SELECT 1 FROM inbox_assignments") {
				t.Fatalf("the two-scan form is still there:\n%s", sql)
			}
		})
	}
}
