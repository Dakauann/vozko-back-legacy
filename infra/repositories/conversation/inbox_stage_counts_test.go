package conversation_repository

import (
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

const groupedStageCountQuery = `unnest\(\$\d+::uuid\[\]\) WITH ORDINALITY AS requested\(stage_id, position\)`

func stageCountInputs() map[string]conversation.SearchEntriesInput {
	return map[string]conversation.SearchEntriesInput{
		"workspace": {WorkspaceID: "ws-1", EntryType: shared.EntryTypeTelegram, StageWorkspaceID: "ws-1"},
		"container": {WorkspaceID: "ws-1", CampaignID: "c-1", EntryType: shared.EntryTypeTelegram, StageWorkspaceID: "ws-1"},
		"entry ids": {EntryIDs: []string{"e-1"}, EntryType: shared.EntryTypeTelegram, StageWorkspaceID: "ws-1"},
	}
}

func TestStageCountsAreOneGroupedQueryInTheRequestedOrder(t *testing.T) {
	for name, in := range stageCountInputs() {
		t.Run(name, func(t *testing.T) {
			repo, mock, done := orderedDB(t)
			defer done()
			mock.ExpectQuery(groupedStageCountQuery).
				WillReturnRows(sqlmock.NewRows([]string{"position", "total"}).AddRow(1, 4).AddRow(2, 0).AddRow(3, 7))

			counts, err := repo.CountEntriesByStages(in, []string{"stage-a", "stage-b", "stage-c"})
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]int64{"stage-a": 4, "stage-b": 0, "stage-c": 7}
			if len(counts) != len(want) {
				t.Fatalf("counts = %v, want %v", counts, want)
			}
			for id, n := range want {
				if counts[id] != n {
					t.Fatalf("counts = %v, want %v", counts, want)
				}
			}
		})
	}
}

func TestStageCountQueriesBindEveryPlaceholderAndDropTheStageFilter(t *testing.T) {
	for name, base := range stageCountInputs() {
		for i, in := range everyInboxFilter(base) {
			in.EntryType, in.EntryIDs, in.CampaignID, in.WorkspaceID = base.EntryType, base.EntryIDs, base.CampaignID, base.WorkspaceID
			in.StageWorkspaceID = "ws-1"
			target, ok, err := inboxTargetOf(in)
			if err != nil || !ok {
				t.Fatalf("%s case %d: no target, %v", name, i, err)
			}
			set, ok := target.set(withoutStageFilter(in))
			if !ok {
				t.Fatalf("%s case %d: no set", name, i)
			}
			sql, args := set.stageCountQuery("ws-1", []string{"stage-a", "stage-b"})
			assertBound(t, name+" grouped", sql, args)
			if strings.Contains(sql, "stage_id = ANY(?)") || strings.Contains(sql, "NOT IN (SELECT entry_id FROM entry_stages") || strings.Contains(sql, "NOT EXISTS (SELECT 1 FROM entry_stages") {
				t.Fatalf("%s case %d: the per-stage filter must give way to the grouped join:\n%s", name, i, sql)
			}
			if !strings.Contains(sql, "es.workspace_id = ? AND es.deleted_at IS NULL") {
				t.Fatalf("%s case %d: the join must keep the stage workspace and live rows:\n%s", name, i, sql)
			}
		}
	}
}

func TestUnstagedIsCountedThroughTheExistingFilter(t *testing.T) {
	cases := map[string]string{
		"workspace": `NOT IN \(SELECT entry_id FROM entry_stages WHERE workspace_id = \$\d+ AND deleted_at IS NULL\)`,
		"container": `NOT EXISTS \(SELECT 1 FROM entry_stages et WHERE et\.entry_id = entries\.entry_id`,
		"entry ids": `cm\.entry_id NOT IN \(SELECT entry_id FROM entry_stages`,
	}
	for name, in := range stageCountInputs() {
		t.Run(name, func(t *testing.T) {
			repo, mock, done := orderedDB(t)
			defer done()
			mock.ExpectQuery(groupedStageCountQuery).
				WillReturnRows(sqlmock.NewRows([]string{"position", "total"}).AddRow(1, 2))
			mock.ExpectQuery(`(?s)` + cases[name] + `.*COUNT\(\*\)|COUNT\(\*\).*` + cases[name]).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

			counts, err := repo.CountEntriesByStages(in, []string{"stage-a", unstagedStageID})
			if err != nil {
				t.Fatal(err)
			}
			if counts["stage-a"] != 2 || counts[unstagedStageID] != 5 || len(counts) != 2 {
				t.Fatalf("counts = %v", counts)
			}
		})
	}
}

func TestWithoutAStageWorkspaceEveryStageCountsEveryConversation(t *testing.T) {
	in := stageCountInputs()["workspace"]
	in.StageWorkspaceID = ""
	repo, mock, done := orderedDB(t)
	defer done()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM entries_with_msg`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(9))

	counts, err := repo.CountEntriesByStages(in, []string{"stage-a", unstagedStageID})
	if err != nil {
		t.Fatal(err)
	}
	if counts["stage-a"] != 9 || counts[unstagedStageID] != 9 {
		t.Fatalf("counts = %v, want the unfiltered total for every stage", counts)
	}
}

func TestStageCountsWithNothingToSearchAreZero(t *testing.T) {
	repo, _, done := orderedDB(t)
	defer done()
	counts, err := repo.CountEntriesByStages(conversation.SearchEntriesInput{EntryType: shared.EntryTypeTelegram, StageWorkspaceID: "ws-1"}, []string{"stage-a"})
	if err != nil || counts["stage-a"] != 0 || len(counts) != 1 {
		t.Fatalf("counts = %v, %v", counts, err)
	}
}

func TestStageCountsRefuseAnUnknownChannel(t *testing.T) {
	repo, _, done := orderedDB(t)
	defer done()
	if _, err := repo.CountEntriesByStages(conversation.SearchEntriesInput{CampaignID: "c-1", EntryType: "nonsense", StageWorkspaceID: "ws-1"}, []string{"stage-a"}); err == nil {
		t.Fatal("an unknown channel must be refused, like the page search")
	}
}

func TestStageCountErrorsPropagate(t *testing.T) {
	boom := errors.New("db down")
	repo, mock, done := orderedDB(t)
	defer done()
	mock.ExpectQuery(groupedStageCountQuery).WillReturnError(boom)

	counts, err := repo.CountEntriesByStages(stageCountInputs()["workspace"], []string{"stage-a"})
	if !errors.Is(err, boom) || counts != nil {
		t.Fatalf("got %v, %v; want the database error", counts, err)
	}
}
