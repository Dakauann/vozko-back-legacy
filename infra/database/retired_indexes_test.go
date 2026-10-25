package database

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"unicode"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	gormschema "gorm.io/gorm/schema"
)

func modelIndexTables(t *testing.T) map[string]string {
	t.Helper()
	tables := map[string]string{}
	cache := &sync.Map{}
	for _, group := range autoMigratedModels() {
		for _, model := range group {
			parsed, err := gormschema.Parse(model, cache, gormschema.NamingStrategy{})
			if err != nil {
				t.Fatalf("parse %T: %v", model, err)
			}
			for _, idx := range parsed.ParseIndexes() {
				tables[idx.Name] = parsed.Table
			}
		}
	}
	return tables
}

func sqlIdentifiers(sql string) []string {
	return strings.FieldsFunc(sql, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
}

func declaredDDLSections() map[string]string {
	sections := map[string]string{}
	for _, section := range declaredDDL() {
		if section.title == retiredIndexesSection {
			continue
		}
		for _, s := range section.statements {
			sections[s.name] = section.title
			for _, word := range sqlIdentifiers(s.sql) {
				sections[word] = section.title
			}
		}
	}
	return sections
}

func TestNoRetiredIndexIsDeclaredAgain(t *testing.T) {
	tags := modelIndexTables(t)
	ddlSections := declaredDDLSections()
	for _, idx := range retiredIndexes() {
		t.Run(idx.name, func(t *testing.T) {
			if table, tagged := tags[idx.name]; tagged {
				t.Fatalf("a GORM index tag on %s still declares %s, so AutoMigrate rebuilds it on the next boot", table, idx.name)
			}
			if section, listed := ddlSections[idx.name]; listed {
				t.Fatalf("%s still appears in the %s DDL list, so the next boot rebuilds it", idx.name, section)
			}
		})
	}
}

func TestEveryRetiredIndexNamesADeclaredIndexThatServesItsQueries(t *testing.T) {
	tags := modelIndexTables(t)
	ddlSections := declaredDDLSections()
	retired := map[string]bool{}
	for _, idx := range retiredIndexes() {
		retired[idx.name] = true
	}
	for _, idx := range retiredIndexes() {
		t.Run(idx.name, func(t *testing.T) {
			if len(idx.coveredBy) == 0 {
				t.Fatalf("%s names no index that keeps its queries served", idx.name)
			}
			for _, cover := range idx.coveredBy {
				if retired[cover] {
					t.Fatalf("%s is covered by %s, which is retired too", idx.name, cover)
				}
				_, tagged := tags[cover]
				_, listed := ddlSections[cover]
				if !tagged && !listed {
					t.Fatalf("%s is covered by %s, which no GORM tag or DDL list declares", idx.name, cover)
				}
			}
		})
	}
}

func TestRetiredIndexesHaveDistinctNames(t *testing.T) {
	seen := map[string]bool{}
	for _, idx := range retiredIndexes() {
		if seen[idx.name] {
			t.Fatalf("%s is retired twice", idx.name)
		}
		seen[idx.name] = true
	}
}

func TestRetiredIndexesAreDroppedWithoutBlockingWrites(t *testing.T) {
	for _, idx := range retiredIndexes() {
		t.Run(idx.name, func(t *testing.T) {
			want := "DROP INDEX CONCURRENTLY IF EXISTS " + idx.name
			if idx.constraintOf != "" {
				want = "ALTER TABLE " + idx.constraintOf + " DROP CONSTRAINT IF EXISTS " + idx.name
			}
			if got := idx.dropSQL(); got != want {
				t.Fatalf("dropSQL() = %q, want %q", got, want)
			}
		})
	}
}

var quickRetry = lockRetry{lockTimeout: "1s", attempts: 2}

func expectPresent(mock sqlmock.Sqlmock, name string, present bool) {
	mock.ExpectQuery(`SELECT to_regclass\(\$1\) IS NOT NULL`).WithArgs(name).
		WillReturnRows(sqlmock.NewRows([]string{"present"}).AddRow(present))
}

func expectReady(mock sqlmock.Sqlmock, name string, ready bool) {
	mock.ExpectQuery(`indexrelid = to_regclass\(\$1\) AND indisvalid AND indisready`).WithArgs(name).
		WillReturnRows(sqlmock.NewRows([]string{"ready"}).AddRow(ready))
}

func expectConstraintDrop(mock sqlmock.Sqlmock, err error) {
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT set_config\('lock_timeout', \$1, true\)`).WithArgs("1s").
		WillReturnResult(sqlmock.NewResult(0, 0))
	drop := mock.ExpectExec(`ALTER TABLE t DROP CONSTRAINT IF EXISTS t_a_b_key`)
	if err != nil {
		drop.WillReturnError(err)
		mock.ExpectRollback()
		return
	}
	drop.WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
}

func TestRetireIndex(t *testing.T) {
	lockTimeout := &pgconn.PgError{Code: lockNotAvailableCode}
	plain := retiredIndex{name: "idx_old", coveredBy: []string{"idx_new", "idx_twin"}}
	constraint := retiredIndex{name: "t_a_b_key", constraintOf: "t", coveredBy: []string{"idx_new"}}
	for _, tc := range []struct {
		name    string
		idx     retiredIndex
		expect  func(sqlmock.Sqlmock)
		wantErr bool
	}{
		{
			name: "an index that is already gone is left alone",
			idx:  plain,
			expect: func(mock sqlmock.Sqlmock) {
				expectPresent(mock, "idx_old", false)
			},
		},
		{
			name: "an index stays while a cover is not built",
			idx:  plain,
			expect: func(mock sqlmock.Sqlmock) {
				expectPresent(mock, "idx_old", true)
				expectReady(mock, "idx_new", true)
				expectReady(mock, "idx_twin", false)
			},
			wantErr: true,
		},
		{
			name: "a covered index is dropped concurrently",
			idx:  plain,
			expect: func(mock sqlmock.Sqlmock) {
				expectPresent(mock, "idx_old", true)
				expectReady(mock, "idx_new", true)
				expectReady(mock, "idx_twin", true)
				mock.ExpectExec(`^DROP INDEX CONCURRENTLY IF EXISTS idx_old$`).WillReturnResult(sqlmock.NewResult(0, 0))
			},
		},
		{
			name: "a missing constraint takes no table lock",
			idx:  constraint,
			expect: func(mock sqlmock.Sqlmock) {
				expectPresent(mock, "t_a_b_key", false)
			},
		},
		{
			name: "a covered constraint is dropped under a lock timeout",
			idx:  constraint,
			expect: func(mock sqlmock.Sqlmock) {
				expectPresent(mock, "t_a_b_key", true)
				expectReady(mock, "idx_new", true)
				expectConstraintDrop(mock, nil)
			},
		},
		{
			name: "a constraint drop that times out on the lock is retried",
			idx:  constraint,
			expect: func(mock sqlmock.Sqlmock) {
				expectPresent(mock, "t_a_b_key", true)
				expectReady(mock, "idx_new", true)
				expectConstraintDrop(mock, lockTimeout)
				expectConstraintDrop(mock, nil)
			},
		},
		{
			name: "a constraint drop gives up after its attempts",
			idx:  constraint,
			expect: func(mock sqlmock.Sqlmock) {
				expectPresent(mock, "t_a_b_key", true)
				expectReady(mock, "idx_new", true)
				expectConstraintDrop(mock, lockTimeout)
				expectConstraintDrop(mock, lockTimeout)
			},
			wantErr: true,
		},
		{
			name: "a constraint drop that fails for another reason is not retried",
			idx:  constraint,
			expect: func(mock sqlmock.Sqlmock) {
				expectPresent(mock, "t_a_b_key", true)
				expectReady(mock, "idx_new", true)
				expectConstraintDrop(mock, errors.New("constraint is referenced"))
			},
			wantErr: true,
		},
		{
			name: "a failed presence check drops nothing",
			idx:  plain,
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`to_regclass`).WillReturnError(errors.New("catalog unavailable"))
			},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, sqlDB := sqlmockDB(t)
			defer sqlDB.Close()
			tc.expect(mock)

			err := retireIndex(db, tc.idx, quickRetry)
			if (err != nil) != tc.wantErr {
				t.Fatalf("retireIndex() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet expectations: %v", err)
			}
		})
	}
}
