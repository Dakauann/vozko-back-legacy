package siteform_repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"vozko/usecases/siteforms"
)

func TestWorkspaceFormsJoinTheProjectNameAndTheLiveManifest(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(sqlPattern(selectWorkspaceFormsSQL)).WithArgs("w1").
		WillReturnRows(sqlmock.NewRows([]string{"project_id", "project_name", "form_id", "form_key", "live"}).
			AddRow("p1", "Clínica Sorriso", "contato", "key-1", true).
			AddRow("p1", "Clínica Sorriso", "newsletter", "key-2", false))
	got, err := NewFormStore(db).WorkspaceForms(context.Background(), "w1")
	if err != nil {
		t.Fatal(err)
	}
	want := []siteforms.PageForm{
		{ProjectID: "p1", ProjectName: "Clínica Sorriso", FormID: "contato", FormKey: "key-1", Live: true},
		{ProjectID: "p1", ProjectName: "Clínica Sorriso", FormID: "newsletter", FormKey: "key-2"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v", got)
	}
	done(t, mock)
}

func TestWorkspaceFormsRefuseWhenTheQueryFails(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(sqlPattern(selectWorkspaceFormsSQL)).WithArgs("w1").WillReturnError(errors.New("db down"))
	if _, err := NewFormStore(db).WorkspaceForms(context.Background(), "w1"); err == nil {
		t.Fatal("a failed query must refuse")
	}
	done(t, mock)
}
