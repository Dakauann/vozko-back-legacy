package siteform_repository

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vozko/domain/sitesubmission"
	"vozko/usecases/siteforms"
)

func mockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

func sqlPattern(query string) string {
	return "^" + strings.ReplaceAll(regexp.QuoteMeta(query), `\?`, `\$[0-9]+`) + "$"
}

func done(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureKeysInsertsThenReadsWhatTheProjectKeeps(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(sqlPattern(insertKeySQL)).WithArgs("new-key", "w1", "p1", "contato", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT form_id, form_key FROM site_form_keys WHERE workspace_id = \$1 AND project_id = \$2 AND form_id IN \(\$3\)`).
		WithArgs("w1", "p1", "contato").WillReturnRows(sqlmock.NewRows([]string{"form_id", "form_key"}).AddRow("contato", "kept-key"))
	got, err := NewFormStore(db).EnsureKeys(context.Background(), "w1", "p1", map[string]string{"contato": "new-key"})
	if err != nil || got["contato"] != "kept-key" {
		t.Fatalf("got %v %v", got, err)
	}
	done(t, mock)
}

func TestActivationRetiresThePreviousFormsInOneTransaction(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(sqlPattern(retireManifestsSQL)).WithArgs("p1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlPattern(retireEmbedsSQL)).WithArgs("p1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO "site_form_manifests"`).WillReturnError(errors.New("down"))
	mock.ExpectRollback()
	err := NewFormStore(db).Activate(context.Background(), siteforms.Activation{WorkspaceID: "w1", ProjectID: "p1", DeploymentID: "d1",
		Forms: []siteforms.ActiveForm{{FormKey: "k", FormID: "contato"}}})
	if err == nil {
		t.Fatal("a failed insert must roll the activation back")
	}
	done(t, mock)
}

func TestAStoredFormThatNoLongerDecodesRefuses(t *testing.T) {
	db, mock := mockDB(t)
	columns := []string{"workspace_id", "project_id", "deployment_id", "form_key", "form_id", "manifest", "settings", "success_url", "booking"}
	mock.ExpectQuery(sqlPattern(selectActiveFormSQL)).WithArgs("k").
		WillReturnRows(sqlmock.NewRows(columns).AddRow("w1", "p1", "d1", "k", "contato", []byte(`{"id":"contato","fields":[],"extra":1}`), []byte(`{}`), "", nil))
	if _, err := NewFormStore(db).ActiveForm(context.Background(), "k"); !errors.Is(err, ErrStoredFormInvalid) {
		t.Fatalf("err = %v", err)
	}
	mock.ExpectQuery(sqlPattern(selectActiveFormSQL)).WithArgs("k").
		WillReturnRows(sqlmock.NewRows(columns).AddRow("w1", "p1", "d1", "k", "agenda", []byte(`{"id":"agenda","fields":[]}`), []byte(`{}`), "", []byte(`{"duration_minutes":0}`)))
	if _, err := NewFormStore(db).ActiveForm(context.Background(), "k"); !errors.Is(err, ErrStoredFormInvalid) {
		t.Fatalf("a stored booking that breaks the rules must refuse: %v", err)
	}
	mock.ExpectQuery(sqlPattern(selectActiveFormSQL)).WithArgs("k").WillReturnError(errors.New("down"))
	if form, err := NewFormStore(db).ActiveForm(context.Background(), "k"); err == nil || form != nil {
		t.Fatalf("a store error must refuse: %+v %v", form, err)
	}
	mock.ExpectQuery(sqlPattern(selectActiveFormSQL)).WithArgs("none").WillReturnRows(sqlmock.NewRows(columns))
	if form, err := NewFormStore(db).ActiveForm(context.Background(), "none"); err != nil || form != nil {
		t.Fatalf("an unknown key reads as none: %+v %v", form, err)
	}
	done(t, mock)
}

func TestADuplicateSubmissionReturnsTheOriginal(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "site_submissions" .* ON CONFLICT DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT \* FROM "site_submissions" WHERE form_key = \$1 AND idempotency_key = \$2 LIMIT \$3`).WithArgs("k", "idem-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "form_key", "idempotency_key", "fields", "utm", "consent"}).AddRow("original", "k", "idem-1", []byte(`{}`), []byte(`{}`), []byte(`{}`)))
	got, created, err := NewSubmissionStore(db).Create(context.Background(), sitesubmission.Submission{ID: "new", FormKey: "k", IdempotencyKey: "idem-1", CreatedAt: time.Now()}, pendingRun(sitesubmission.Submission{ID: "new"}))
	if err != nil || created || got.ID != "original" {
		t.Fatalf("got %+v %v %v", got, created, err)
	}
	done(t, mock)
}

func TestAnUnknownProjectHasNoWorkspace(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(sqlPattern(selectProjectWorkspaceSQL)).WithArgs("p9").WillReturnRows(sqlmock.NewRows([]string{"workspace_id"}))
	if _, err := NewProjectDirectory(db).WorkspaceOf(context.Background(), "p9"); !errors.Is(err, siteforms.ErrProjectNotFound) {
		t.Fatalf("err = %v", err)
	}
	done(t, mock)
}
