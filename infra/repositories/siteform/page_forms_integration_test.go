package siteform_repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/infra/database/schema"
)

func TestWorkspaceFormsReadTheFriendlyNameFromTheSettingsAgainstPostgres(t *testing.T) {
	db := integrationDB(t)
	if err := db.AutoMigrate(&schema.SiteSettings{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store := NewFormStore(db)
	ws, named, bare := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	for _, p := range []schema.SiteProject{
		{ID: named, WorkspaceID: ws, Name: "Clínica", Slug: "clinica", Stack: "html", WorkingRevisionID: uuid.NewString(), CreatedBy: uuid.NewString(), CreatedAt: now, UpdatedAt: now},
		{ID: bare, WorkspaceID: ws, Name: "Loja", Slug: "loja", Stack: "html", WorkingRevisionID: uuid.NewString(), CreatedBy: uuid.NewString(), CreatedAt: now, UpdatedAt: now},
	} {
		if err := db.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&schema.SiteSettings{ProjectID: named, WorkspaceID: ws, Settings: []byte(`{"forms":{"contato":{"name":"Fale conosco"}}}`), UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureKeys(ctx, ws, named, map[string]string{"contato": "key-named", "news": "key-news"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureKeys(ctx, ws, bare, map[string]string{"contato": "key-bare"}); err != nil {
		t.Fatal(err)
	}
	forms, err := store.WorkspaceForms(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, f := range forms {
		names[f.FormKey] = f.FormName
	}
	if len(forms) != 3 || names["key-named"] != "Fale conosco" || names["key-news"] != "" || names["key-bare"] != "" {
		t.Fatalf("got %+v", forms)
	}
	one, err := store.WorkspaceForm(ctx, ws, "key-named")
	if err != nil || one == nil || one.FormName != "Fale conosco" {
		t.Fatalf("got %+v %v", one, err)
	}
}
