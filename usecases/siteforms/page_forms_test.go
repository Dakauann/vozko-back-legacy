package siteforms

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/site"
)

func TestPageFormsListTheWorkspaceFormsThatHaveAKey(t *testing.T) {
	w := newWorld(t)
	w.forms.names = map[string]string{"p1": "Clínica Sorriso", "p2": "Outra loja"}
	key := w.publish(t, site.FormSettings{})
	if _, err := w.svc.KeysFor(context.Background(), "w2", "p2", []string{"newsletter"}); err != nil {
		t.Fatal(err)
	}
	got, err := w.svc.PageForms(context.Background(), "w1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (PageForm{ProjectID: "p1", ProjectName: "Clínica Sorriso", FormID: "contato", FormKey: key, Live: true}) {
		t.Fatalf("forms %+v", got)
	}
}

func TestPageFormsRefuseWithoutAWorkspaceOrAReadableStore(t *testing.T) {
	w := newWorld(t)
	if _, err := w.svc.PageForms(context.Background(), " "); !errors.Is(err, ErrWorkspaceRequired) {
		t.Fatalf("no workspace = %v", err)
	}
	w.forms.failList = errors.New("db down")
	if _, err := w.svc.PageForms(context.Background(), "w1"); err == nil {
		t.Fatal("a store failure must refuse")
	}
}
