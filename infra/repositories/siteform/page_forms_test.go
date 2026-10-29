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
		WillReturnRows(sqlmock.NewRows([]string{"project_id", "project_name", "form_id", "form_name", "form_key", "live"}).
			AddRow("p1", "Clínica Sorriso", "contato", "Fale conosco", "key-1", true).
			AddRow("p1", "Clínica Sorriso", "newsletter", "", "key-2", false))
	got, err := NewFormStore(db).WorkspaceForms(context.Background(), "w1")
	if err != nil {
		t.Fatal(err)
	}
	want := []siteforms.PageForm{
		{ProjectID: "p1", ProjectName: "Clínica Sorriso", FormID: "contato", FormName: "Fale conosco", FormKey: "key-1", Live: true},
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

func TestWorkspaceFormReadsOneKeyOfTheWorkspaceOrNothing(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(sqlPattern(selectWorkspaceFormSQL)).WithArgs("w1", "key-1").
		WillReturnRows(sqlmock.NewRows([]string{"project_id", "project_name", "form_id", "form_name", "form_key", "live"}).AddRow("p1", "Clínica Sorriso", "contato", "Orçamento", "key-1", false))
	mock.ExpectQuery(sqlPattern(selectWorkspaceFormSQL)).WithArgs("w1", "key-2").WillReturnRows(sqlmock.NewRows([]string{"project_id", "project_name", "form_id", "form_name", "form_key", "live"}))
	mock.ExpectQuery(sqlPattern(selectWorkspaceFormSQL)).WithArgs("w1", "key-3").WillReturnError(errors.New("db down"))
	store := NewFormStore(db)
	got, err := store.WorkspaceForm(context.Background(), "w1", "key-1")
	if err != nil || got == nil || *got != (siteforms.PageForm{ProjectID: "p1", ProjectName: "Clínica Sorriso", FormID: "contato", FormName: "Orçamento", FormKey: "key-1"}) {
		t.Fatalf("got %+v %v", got, err)
	}
	if got, err := store.WorkspaceForm(context.Background(), "w1", "key-2"); err != nil || got != nil {
		t.Fatalf("got %+v %v", got, err)
	}
	if _, err := store.WorkspaceForm(context.Background(), "w1", "key-3"); err == nil {
		t.Fatal("a failed query must refuse")
	}
	done(t, mock)
}
