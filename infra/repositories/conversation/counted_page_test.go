package conversation_repository

import (
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

type countedList struct {
	name      string
	pageQuery string
	countSQL  string
	columns   []string
	row       func(id string, total int64) []driver.Value
	search    func(r *repository, page int) ([]conversation.EntryWithLastMessage, int64, error)
}

func countedLists() []countedList {
	return []countedList{
		{
			name:      "workspace inbox",
			pageQuery: `SELECT entry_id, entry_type, COUNT\(\*\) OVER \(\) AS page_total FROM entries_with_msg`,
			countSQL:  `SELECT COUNT\(\*\) FROM entries_with_msg`,
			columns:   []string{"entry_id", "entry_type", "page_total"},
			row: func(id string, total int64) []driver.Value {
				return []driver.Value{id, string(shared.EntryTypeTelegram), total}
			},
			search: func(r *repository, page int) ([]conversation.EntryWithLastMessage, int64, error) {
				return r.SearchEntriesWithMessages(conversation.SearchEntriesInput{WorkspaceID: "ws-1", EntryType: shared.EntryTypeTelegram, Page: page, PageSize: 2})
			},
		},
		{
			name:      "container inbox",
			pageQuery: `SELECT matching\.entry_id, COUNT\(\*\) OVER \(\) AS page_total FROM \(`,
			countSQL:  `SELECT COUNT\(\*\) FROM \(.*\) AS matching_entries`,
			columns:   []string{"entry_id", "page_total"},
			row: func(id string, total int64) []driver.Value {
				return []driver.Value{id, total}
			},
			search: func(r *repository, page int) ([]conversation.EntryWithLastMessage, int64, error) {
				return r.SearchEntriesWithMessages(conversation.SearchEntriesInput{CampaignID: "c-1", EntryType: shared.EntryTypeTelegram, Page: page, PageSize: 2})
			},
		},
		{
			name:      "board",
			pageQuery: `SELECT entry_id, entry_type, lead_name, lead_number, COUNT\(\*\) OVER \(\) AS page_total FROM entries_with_msg`,
			countSQL:  `SELECT COUNT\(\*\) FROM entries_with_msg`,
			columns:   []string{"entry_id", "entry_type", "lead_name", "lead_number", "page_total"},
			row: func(id string, total int64) []driver.Value {
				return []driver.Value{id, string(shared.EntryTypeTelegram), "Ana", "5511999999999", total}
			},
			search: func(r *repository, page int) ([]conversation.EntryWithLastMessage, int64, error) {
				in := selectionInput()
				in.Page, in.PageSize = page, 2
				return r.SearchEntriesByFilter(in)
			},
		},
	}
}

func orderedDB(t *testing.T) (*repository, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, sqlDB := newStatusDB(t)
	mock.MatchExpectationsInOrder(true)
	return &repository{db: db}, mock, func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		sqlDB.Close()
	}
}

func TestAPageCarriesItsOwnTotalWithoutACountQuery(t *testing.T) {
	for _, list := range countedLists() {
		t.Run(list.name, func(t *testing.T) {
			repo, mock, done := orderedDB(t)
			defer done()
			mock.ExpectQuery(list.pageQuery).
				WillReturnRows(sqlmock.NewRows(list.columns).AddRow(list.row("e-1", 7)...).AddRow(list.row("e-2", 7)...))
			mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(SELECT u\.entry_id FROM unnest`).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

			entries, total, err := list.search(repo, 2)
			if err != nil {
				t.Fatal(err)
			}
			if total != 7 || len(entries) != 0 {
				t.Fatalf("total = %d with %d hydrated entries, want the window total 7", total, len(entries))
			}
		})
	}
}

func TestAnEmptyFirstPageIsAnEmptyList(t *testing.T) {
	for _, list := range countedLists() {
		t.Run(list.name, func(t *testing.T) {
			repo, mock, done := orderedDB(t)
			defer done()
			mock.ExpectQuery(list.pageQuery).WillReturnRows(sqlmock.NewRows(list.columns))

			entries, total, err := list.search(repo, 1)
			if err != nil || entries != nil || total != 0 {
				t.Fatalf("got %v, %d, %v; want nothing and no count query", entries, total, err)
			}
		})
	}
}

func TestAPagePastTheEndStillReportsTheTotal(t *testing.T) {
	for _, list := range countedLists() {
		for _, count := range []int64{0, 5} {
			t.Run(list.name, func(t *testing.T) {
				repo, mock, done := orderedDB(t)
				defer done()
				mock.ExpectQuery(list.pageQuery).WillReturnRows(sqlmock.NewRows(list.columns))
				mock.ExpectQuery(list.countSQL).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))

				entries, total, err := list.search(repo, 4)
				if err != nil || entries != nil || total != count {
					t.Fatalf("got %v, %d, %v; want no entries and the counted total %d", entries, total, err, count)
				}
			})
		}
	}
}

func TestCountedPageErrorsPropagate(t *testing.T) {
	boom := errors.New("db down")
	for _, list := range countedLists() {
		t.Run(list.name+"/page", func(t *testing.T) {
			repo, mock, done := orderedDB(t)
			defer done()
			mock.ExpectQuery(list.pageQuery).WillReturnError(boom)

			entries, total, err := list.search(repo, 1)
			if !errors.Is(err, boom) || entries != nil || total != 0 {
				t.Fatalf("got %v, %d, %v; want the page error", entries, total, err)
			}
		})
		t.Run(list.name+"/count", func(t *testing.T) {
			repo, mock, done := orderedDB(t)
			defer done()
			mock.ExpectQuery(list.pageQuery).WillReturnRows(sqlmock.NewRows(list.columns))
			mock.ExpectQuery(list.countSQL).WillReturnError(boom)

			entries, total, err := list.search(repo, 3)
			if !errors.Is(err, boom) || entries != nil || total != 0 {
				t.Fatalf("got %v, %d, %v; want the count error", entries, total, err)
			}
		})
	}
}

func TestCountedPagesBindEveryPlaceholder(t *testing.T) {
	q, ok := workspaceInboxQueries(conversation.SearchEntriesInput{WorkspaceID: "ws-1", AssignedUserID: "user-1"}, 20, 40)
	if !ok || q.offset != 40 {
		t.Fatalf("the workspace page must remember its offset, got %d", q.offset)
	}
	ch, _ := channelQueryFor(shared.EntryTypeTelegram)
	if cq := containerInboxQueries(conversation.SearchEntriesInput{CampaignID: "c-1", EntryType: shared.EntryTypeTelegram}, ch, nil, 20, 60); cq.offset != 60 {
		t.Fatalf("the container page must remember its offset, got %d", cq.offset)
	}
	in := selectionInput()
	in.AssignedUserID = "user-1"
	in.Page, in.PageSize = 3, 10
	bq, err := filteredPageQueries(in)
	if err != nil {
		t.Fatal(err)
	}
	if bq.offset != 20 {
		t.Fatalf("the board page offset = %d, want 20", bq.offset)
	}
	for name, page := range map[string]countedPage{"workspace": q, "board": bq} {
		assertBound(t, name+" count", page.countSQL, page.countArgs)
		assertBound(t, name+" page", page.pageSQL, page.pageArgs)
		if !strings.Contains(page.pageSQL, pageTotalColumn) || strings.Contains(page.countSQL, pageTotalColumn) {
			t.Fatalf("%s: only the page carries the window total", name)
		}
	}
}
