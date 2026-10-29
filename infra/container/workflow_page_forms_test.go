package container

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"vozko/domain/siteform"
	"vozko/domain/sitesubmission"
	workflow_domain "vozko/domain/workflow"
	"vozko/usecases/siteforms"
)

type fakeLiveForms map[string]siteforms.ActiveForm

func (f fakeLiveForms) PageForm(_ context.Context, workspaceID, formKey string) (siteforms.PageForm, error) {
	if formKey == "Dr4ftKey0000000000000000" && workspaceID == "ws1" {
		return siteforms.PageForm{ProjectID: "p1", FormID: "orcamento", FormKey: formKey}, nil
	}
	return siteforms.PageForm{}, siteforms.ErrFormUnknownForWorkspace
}

func (f fakeLiveForms) LiveForm(_ context.Context, workspaceID, formKey string) (siteforms.ActiveForm, error) {
	form, ok := f[formKey]
	if !ok || form.WorkspaceID != workspaceID {
		return siteforms.ActiveForm{}, sitesubmission.ErrFormUnknown
	}
	return form, nil
}

func TestThePageFormKeyOfAWorkflowTriggerReadsTheLiveFormFields(t *testing.T) {
	manifest := siteform.Manifest{ID: "contato", Fields: []siteform.Field{
		{Name: "lead.name", Kind: siteform.KindText, Label: "Nome", Required: true},
		{Name: "field.plano", Kind: siteform.KindSelect, Label: "Plano", Options: []string{"Básico", "Pro"}},
		{Name: "consent", Kind: siteform.KindCheckbox, Required: true},
	}}
	catalog := triggerFormCatalog{pages: fakeLiveForms{"Qk3xZ0aT1v9mB2cD4eF5gH6i": {WorkspaceID: "ws1", FormKey: "Qk3xZ0aT1v9mB2cD4eF5gH6i", FormID: "contato", Manifest: manifest}}}
	got, err := catalog.TriggerForm("ws1", "Qk3xZ0aT1v9mB2cD4eF5gH6i")
	if err != nil {
		t.Fatal(err)
	}
	want := workflow_domain.TriggerForm{ID: "Qk3xZ0aT1v9mB2cD4eF5gH6i", Name: "contato", Questions: []workflow_domain.TriggerFormQuestion{
		{Key: "lead.name", Prompt: "Nome", Required: true},
		{Key: "field.plano", Prompt: "Plano", Choices: []string{"Básico", "Pro"}},
		{Key: "consent", Prompt: "consent", Required: true},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if _, err := catalog.TriggerForm("ws2", "Qk3xZ0aT1v9mB2cD4eF5gH6i"); !errors.Is(err, sitesubmission.ErrFormUnknown) {
		t.Fatalf("another workspace must not read the page form, got %v", err)
	}
}

func TestADraftPageFormIsReadableByTheTriggerBeforeItsFirstPublish(t *testing.T) {
	catalog := triggerFormCatalog{pages: fakeLiveForms{}}
	got, err := catalog.TriggerForm("ws1", "Dr4ftKey0000000000000000")
	if err != nil || !reflect.DeepEqual(got, workflow_domain.TriggerForm{ID: "Dr4ftKey0000000000000000", Name: "orcamento"}) {
		t.Fatalf("got %+v %v", got, err)
	}
	if _, err := catalog.TriggerForm("ws2", "Dr4ftKey0000000000000000"); err == nil {
		t.Fatal("another workspace must not read the draft form")
	}
}
