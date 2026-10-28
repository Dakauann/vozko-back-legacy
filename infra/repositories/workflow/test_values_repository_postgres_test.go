package workflow_repository

import (
	"reflect"
	"testing"

	"github.com/google/uuid"

	"vozko/infra/database/schema"
)

func TestTestValuesAreKeptPerWorkflowAndPersonOnPostgres(t *testing.T) {
	db := postgresSchemaTx(t, &schema.WorkflowTestValue{})
	repo := NewTestValuesRepository(db)
	workspace, wf, otherWf := uuid.NewString(), uuid.NewString(), uuid.NewString()

	if err := repo.SaveTestValues(workspace, wf, "ana", map[string]string{"matricula": "123", "valor": "10"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveTestValues(workspace, wf, "bia", map[string]string{"matricula": "999"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveTestValues(workspace, otherWf, "ana", map[string]string{"matricula": "777"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveTestValues(workspace, wf, "ana", map[string]string{"matricula": "124"}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.TestValues(workspace, wf, "ana")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]string{"matricula": "124"}) {
		t.Fatalf("ana = %v, want the last save to replace the previous values", got)
	}
	if got, _ := repo.TestValues(workspace, wf, "bia"); !reflect.DeepEqual(got, map[string]string{"matricula": "999"}) {
		t.Fatalf("bia = %v", got)
	}
	if got, _ := repo.TestValues(uuid.NewString(), wf, "ana"); len(got) != 0 {
		t.Fatalf("another workspace reads %v", got)
	}

	if err := repo.ClearTestValues(workspace, wf, "ana"); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.TestValues(workspace, wf, "ana"); len(got) != 0 {
		t.Fatalf("cleared = %v", got)
	}
	if got, _ := repo.TestValues(workspace, otherWf, "ana"); !reflect.DeepEqual(got, map[string]string{"matricula": "777"}) {
		t.Fatalf("clearing one workflow touched another: %v", got)
	}
}
