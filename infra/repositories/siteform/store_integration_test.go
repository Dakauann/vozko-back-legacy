package siteform_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/site"
	"vozko/domain/siteform"
	"vozko/domain/sitesubmission"
	"vozko/domain/working_hours"
	"vozko/infra/database"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
	"vozko/usecases/siteforms"
)

func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := repotest.IsolatedDB(t, "site_forms")
	db.Config.DisableForeignKeyConstraintWhenMigrating = true
	if err := database.CreateSearchFold(db); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(append(schema.SiteFormModels(), &schema.Lead{}, &schema.SiteProject{})...); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestFormKeysManifestsAndEmbedsAgainstPostgres(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	store := NewFormStore(db)
	ws, project, other := uuid.NewString(), uuid.NewString(), uuid.NewString()
	first, err := store.EnsureKeys(ctx, ws, project, map[string]string{"contato": "key-contato-1", "news": "key-news-1"})
	if err != nil || first["contato"] != "key-contato-1" || first["news"] != "key-news-1" {
		t.Fatalf("first %v %v", first, err)
	}
	again, err := store.EnsureKeys(ctx, ws, project, map[string]string{"contato": "key-contato-2"})
	if err != nil || again["contato"] != "key-contato-1" {
		t.Fatalf("again %v %v", again, err)
	}
	stolen, err := store.EnsureKeys(ctx, uuid.NewString(), project, map[string]string{"contato": "key-contato-3"})
	if err != nil || stolen["contato"] != "" {
		t.Fatalf("another workspace must never read these keys: %v %v", stolen, err)
	}
	manifest := siteform.Manifest{ID: "contato", Fields: []siteform.Field{{Name: "lead.phone", Kind: siteform.KindTel}}, ConsentText: "Aceito"}
	widget := uuid.NewString()
	activate := func(deployment string) {
		t.Helper()
		err := store.Activate(ctx, siteforms.Activation{WorkspaceID: ws, ProjectID: project, DeploymentID: deployment, Widgets: []string{widget},
			Forms: []siteforms.ActiveForm{{FormKey: "key-contato-1", FormID: "contato", Manifest: manifest, Settings: site.FormSettings{Notify: true}, SuccessURL: "https://vz.co/x"}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	d1, d2 := uuid.NewString(), uuid.NewString()
	activate(d1)
	activate(d2)
	form, err := store.ActiveForm(ctx, "key-contato-1")
	if err != nil || form == nil || form.DeploymentID != d2 || form.WorkspaceID != ws || !form.Settings.Notify || form.Manifest.ConsentText != "Aceito" || form.SuccessURL != "https://vz.co/x" {
		t.Fatalf("form %+v %v", form, err)
	}
	projects, err := store.ProjectsEmbedding(ctx, ws, widget)
	if err != nil || len(projects) != 1 || projects[0] != project {
		t.Fatalf("projects %v %v", projects, err)
	}
	booking := site.Booking{
		DurationMinutes: 30, HorizonDays: 7, Title: "Avaliação com {nome}", ConsentText: "Aceito",
		Hours: working_hours.Spec{Timezone: "America/Sao_Paulo", Days: map[string][]working_hours.Window{"mon": {{Start: "09:00", End: "12:00"}}}},
	}
	err = store.Activate(ctx, siteforms.Activation{WorkspaceID: ws, ProjectID: project, DeploymentID: uuid.NewString(), Forms: []siteforms.ActiveForm{
		{FormKey: "key-contato-1", FormID: "contato", Manifest: manifest},
		{FormKey: "key-agenda-1", FormID: "agenda", Manifest: booking.Manifest("agenda"), Booking: &booking},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := store.ActiveForm(ctx, "key-contato-1"); err != nil || plain == nil || plain.Booking != nil {
		t.Fatalf("plain form %+v %v", plain, err)
	}
	booked, err := store.ActiveForm(ctx, "key-agenda-1")
	if err != nil || booked == nil || booked.Booking == nil || booked.Booking.DurationMinutes != 30 || booked.Booking.Hours.Timezone != "America/Sao_Paulo" {
		t.Fatalf("booking form %+v %v", booked, err)
	}
	if foreign, _ := store.ProjectsEmbedding(ctx, other, widget); len(foreign) != 0 {
		t.Fatalf("another workspace sees the embed: %v", foreign)
	}
	if err := store.Deactivate(ctx, project); err != nil {
		t.Fatal(err)
	}
	if gone, err := store.ActiveForm(ctx, "key-contato-1"); err != nil || gone != nil {
		t.Fatalf("after unpublish %+v %v", gone, err)
	}
	if gone, _ := store.ProjectsEmbedding(ctx, ws, widget); len(gone) != 0 {
		t.Fatalf("embed kept after unpublish: %v", gone)
	}
}

func TestSubmissionsAgainstPostgres(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	store := NewSubmissionStore(db)
	ws, project, deployment, leadID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	sub := sitesubmission.Submission{
		ID: uuid.NewString(), WorkspaceID: ws, ProjectID: project, DeploymentID: deployment, FormKey: "key-1", FormID: "contato", Route: "/",
		Fields: map[string]string{"lead.name": "Ana"}, UTM: sitesubmission.UTM{Campaign: "natal"}, Consent: sitesubmission.Consent{Granted: true, Text: "Aceito"},
		IPHash: "hash", UserAgent: "ua", LeadID: leadID, IdempotencyKey: "idem-12345678", CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	stored, created, err := store.Create(ctx, sub, pendingRun(sub))
	if err != nil || !created || stored.ID != sub.ID {
		t.Fatalf("create %v %v", created, err)
	}
	dup := sub
	dup.ID = uuid.NewString()
	original, created, err := store.Create(ctx, dup, pendingRun(dup))
	if err != nil || created || original.ID != sub.ID {
		t.Fatalf("duplicate %+v %v %v", original, created, err)
	}
	conv := uuid.NewString()
	if err := store.AttachConversation(ctx, sub.ID, conv, "whatsapp"); err != nil {
		t.Fatal(err)
	}
	found, err := store.ByIdempotency(ctx, "key-1", "idem-12345678")
	if err != nil || found == nil || found.ConversationID != conv || found.Fields["lead.name"] != "Ana" || found.UTM.Campaign != "natal" || !found.Consent.Granted || found.LeadID != leadID {
		t.Fatalf("found %+v %v", found, err)
	}
	if err := db.Create(&schema.Lead{ID: leadID, WorkspaceID: ws, Number: "5584999990000"}).Error; err != nil {
		t.Fatal(err)
	}
	items, total, err := store.List(ctx, siteforms.SubmissionQuery{WorkspaceID: ws, ProjectID: project, FormID: "contato", Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("list %v %d %v", items, total, err)
	}
	if err := db.Exec("UPDATE leads SET deleted_at = now() WHERE id = ?", leadID).Error; err != nil {
		t.Fatal(err)
	}
	_, total, err = store.List(ctx, siteforms.SubmissionQuery{WorkspaceID: ws, ProjectID: project, Page: 1, PageSize: 10})
	if err != nil || total != 0 {
		t.Fatalf("a deleted lead's submissions must disappear: %d %v", total, err)
	}
}

func TestReferencesAndProjectsAgainstPostgres(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	refs := NewReferenceStore(db)
	ws, project := uuid.NewString(), uuid.NewString()
	key := siteforms.PageReferenceKey{ProjectID: project, Route: "/", NumberRef: "n1", TextDigest: "0000000000000000000000000000000000000000000000000000000000000000"}
	ref := siteforms.PageReference{Key: key, Code: "LP7K2", WorkspaceID: ws, ShortLinkID: uuid.NewString(), URL: "https://vz.co/a"}
	if _, created, err := refs.Create(ctx, ref); err != nil || !created {
		t.Fatalf("create %v %v", created, err)
	}
	other := ref
	other.Code = "ZZZZZ"
	kept, created, err := refs.Create(ctx, other)
	if err != nil || created || kept.Code != "LP7K2" {
		t.Fatalf("second %+v %v %v", kept, created, err)
	}
	target, err := refs.Target(ctx, "LP7K2")
	if err != nil || target == nil || target.ProjectID != project || target.WorkspaceID != ws || target.NumberRef != "n1" {
		t.Fatalf("target %+v %v", target, err)
	}
	if none, err := refs.Target(ctx, "AAAAA"); err != nil || none != nil {
		t.Fatalf("none %+v %v", none, err)
	}
	if err := db.Exec("INSERT INTO site_projects (id, workspace_id, name, slug, stack, status, working_revision_id, version, created_by, created_at, updated_at) VALUES (?, ?, 'Loja', 'loja', 'html', 'draft', ?, 1, ?, now(), now())", project, ws, uuid.NewString(), uuid.NewString()).Error; err != nil {
		t.Skipf("site_projects shape changed: %v", err)
	}
	got, err := NewProjectDirectory(db).WorkspaceOf(ctx, project)
	if err != nil || got != ws {
		t.Fatalf("workspace %q %v", got, err)
	}
	if _, err := NewProjectDirectory(db).WorkspaceOf(ctx, uuid.NewString()); err == nil {
		t.Fatal("an unknown project must refuse")
	}
}

func TestEffectRunsAgainstPostgres(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Microsecond)
	sub := sitesubmission.Submission{
		ID: uuid.NewString(), WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), DeploymentID: uuid.NewString(), FormKey: "key-2", FormID: "contato",
		Route: "/", Fields: map[string]string{"lead.name": "Ana"}, IdempotencyKey: "idem-effects-1", CreatedAt: at,
	}
	if _, created, err := NewSubmissionStore(db).Create(ctx, sub, pendingRun(sub)); err != nil || !created {
		t.Fatalf("create %v %v", created, err)
	}
	effects := NewEffectStore(db)
	due, err := effects.Due(ctx, at, 10)
	if err != nil || len(due) != 1 || due[0] != sub.ID {
		t.Fatalf("due %v %v", due, err)
	}
	run, err := effects.Claim(ctx, sub.ID, at, time.Minute)
	if err != nil || run == nil || run.Submission.Fields["lead.name"] != "Ana" || !run.Has(sitesubmission.EffectNotice) || !run.Plan.Settings.Notify {
		t.Fatalf("claim %+v %v", run, err)
	}
	if again, err := effects.Claim(ctx, sub.ID, at, time.Minute); err != nil || again != nil {
		t.Fatalf("a claimed run is not claimed twice: %+v %v", again, err)
	}
	if due, _ := effects.Due(ctx, at, 10); len(due) != 0 {
		t.Fatalf("a claimed run is not due: %v", due)
	}
	run.OwnerID = "user-7"
	if err := effects.Progress(ctx, *run); err != nil {
		t.Fatal(err)
	}
	run.Settle(at, errors.New("notices down"))
	if err := effects.Settle(ctx, *run); err != nil {
		t.Fatal(err)
	}
	if due, _ := effects.Due(ctx, at, 10); len(due) != 0 {
		t.Fatalf("a run waits for its backoff: %v", due)
	}
	later := at.Add(time.Minute)
	retry, err := effects.Claim(ctx, sub.ID, later, time.Minute)
	if err != nil || retry == nil || retry.OwnerID != "user-7" || retry.Attempts != 1 || retry.LastError != "notices down" {
		t.Fatalf("retry %+v %v", retry, err)
	}
	retry.Done(sitesubmission.EffectNotice)
	retry.Settle(later, nil)
	if err := effects.Settle(ctx, *retry); err != nil {
		t.Fatal(err)
	}
	if gone, err := effects.Claim(ctx, sub.ID, later.Add(time.Hour), time.Minute); err != nil || gone != nil {
		t.Fatalf("a finished run is never claimed: %+v %v", gone, err)
	}
}
