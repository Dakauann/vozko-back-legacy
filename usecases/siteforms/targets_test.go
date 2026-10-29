package siteforms

import (
	"context"
	"errors"
	"strings"
	"testing"

	"vozko/domain/customfield"
	"vozko/domain/siteform"
)

type leadFieldShelf struct {
	defs  []*customfield.Definition
	err   error
	asked []string
}

func (s *leadFieldShelf) ListByObject(workspaceID string, objectType customfield.ObjectType) ([]*customfield.Definition, error) {
	s.asked = append(s.asked, workspaceID+":"+string(objectType))
	return s.defs, s.err
}

func targetLine(target FormTarget) string {
	kinds := make([]string, 0, len(target.Kinds))
	for _, k := range target.Kinds {
		kinds = append(kinds, string(k))
	}
	return target.Target + "|" + target.Label + "|" + strings.Join(kinds, ",")
}

func TestFormTargetsListTheLeadFieldsAFormCanFill(t *testing.T) {
	w := newWorld(t)
	w.leadFields.defs = []*customfield.Definition{
		{Key: "cidade", Label: "Cidade", Type: customfield.TypeText, ObjectType: customfield.ObjectLead},
		{Key: "nascimento", Label: "Nascimento", Type: customfield.TypeDate, ObjectType: customfield.ObjectLead},
		{Key: "cpf", Label: "CPF", Type: customfield.TypeText, ObjectType: customfield.ObjectLead, Sensitive: true},
		{Key: "plano", Label: "Plano", Type: customfield.TypeSelect, ObjectType: customfield.ObjectLead},
		{Key: "Mal Formado", Label: "x", Type: customfield.TypeText, ObjectType: customfield.ObjectLead},
		nil,
	}
	got, err := w.svc.FormTargets(context.Background(), "w1")
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{}
	for _, target := range got.Targets {
		lines = append(lines, targetLine(target))
	}
	want := []string{
		"lead.name||text",
		"lead.phone||tel,text",
		"lead.email||email",
		"consent||checkbox",
		"email_opt_in||checkbox",
		"field.cidade|Cidade|text,textarea,email,tel,url,select,radio,hidden",
		"field.nascimento|Nascimento|date,text",
		"field.plano|Plano|select,radio",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s", strings.Join(lines, "\n"))
	}
	if !got.Targets[5].Custom || got.Targets[0].Custom || w.leadFields.asked[0] != "w1:lead" {
		t.Fatalf("got %+v %v", got.Targets, w.leadFields.asked)
	}
	if len(got.Settable) == 0 || len(got.Insertable) <= len(got.Settable) || got.Insertable[len(got.Insertable)-1] != siteform.KindTextarea {
		t.Fatalf("got %+v", got)
	}
}

func TestFormTargetsFailWhenTheLeadFieldsCannotBeRead(t *testing.T) {
	w := newWorld(t)
	w.leadFields.err = errors.New("down")
	if _, err := w.svc.FormTargets(context.Background(), "w1"); err == nil {
		t.Fatal("a missing custom field list never reads as no custom fields")
	}
}
