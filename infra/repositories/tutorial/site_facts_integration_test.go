package tutorial_repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"vozko/domain/tutorial"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

var _ tutorial.SiteFactsReader = (*SiteFacts)(nil)

var factsAt = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func siteDB(t *testing.T) *gorm.DB {
	t.Helper()
	return repotest.IsolatedDB(t, "tutorial_site_facts_test",
		&schema.SiteProject{}, &schema.SiteSettings{}, &schema.SiteRevision{}, &schema.SiteHostname{}, &schema.SiteDeployment{})
}

func addProject(t *testing.T, db *gorm.DB, workspaceID string) string {
	t.Helper()
	id := uuid.NewString()
	row := schema.SiteProject{ID: id, WorkspaceID: workspaceID, Name: "Loja", Slug: "loja-" + id[:8], Stack: "react", Status: "draft",
		WorkingRevisionID: uuid.NewString(), CreatedBy: uuid.NewString(), CreatedAt: factsAt, UpdatedAt: factsAt}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func addRevision(t *testing.T, db *gorm.DB, workspaceID, projectID, author, kind string) {
	t.Helper()
	row := schema.SiteRevision{ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID, Manifest: datatypes.JSON(`{}`),
		Author: author, Kind: kind, CreatedAt: factsAt, UpdatedAt: factsAt}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
}

func addSettings(t *testing.T, db *gorm.DB, workspaceID, projectID, settings string) {
	t.Helper()
	row := schema.SiteSettings{ProjectID: projectID, WorkspaceID: workspaceID, Settings: datatypes.JSON(settings), UpdatedAt: factsAt}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
}

func addHostname(t *testing.T, db *gorm.DB, workspaceID, projectID, kind, status string) {
	t.Helper()
	row := schema.SiteHostname{ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID, Hostname: uuid.NewString()[:8] + ".example.com",
		Kind: kind, Status: status, CreatedAt: factsAt, UpdatedAt: factsAt}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
}

func addDeployment(t *testing.T, db *gorm.DB, workspaceID, projectID, status string) {
	t.Helper()
	row := schema.SiteDeployment{ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID, Status: status, CreatedAt: factsAt}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
}

func TestAnEmptyWorkspaceHasNoSiteFactsAgainstPostgres(t *testing.T) {
	facts, err := NewSiteFacts(siteDB(t)).SiteFacts(context.Background(), uuid.NewString())
	if err != nil || facts != (tutorial.SiteFacts{}) {
		t.Fatalf("facts = %+v, %v", facts, err)
	}
}

func TestSiteFactsComeFromTheWorkspaceDataAgainstPostgres(t *testing.T) {
	db := siteDB(t)
	workspace, other := uuid.NewString(), uuid.NewString()
	project := addProject(t, db, workspace)
	addRevision(t, db, workspace, project, "person:abc", "checkpoint")
	addRevision(t, db, workspace, project, "elo:thread1", "elo")
	addSettings(t, db, workspace, project, `{"forms":{"contato":{"notify":true}}}`)
	addHostname(t, db, workspace, project, "custom", "pending_dns")
	addHostname(t, db, workspace, project, "platform", "active")
	addDeployment(t, db, workspace, project, "failed")

	theirs := addProject(t, db, other)
	addRevision(t, db, other, theirs, "person:xyz", "autosave")
	addSettings(t, db, other, theirs, `{"forms":{"a":{"funnel_id":"f1"}},"bookings":{"agenda":{"duration_minutes":30}},"site":{"webchat_widget":"w1"}}`)
	addHostname(t, db, other, theirs, "custom", "active")
	addDeployment(t, db, other, theirs, "active")

	reader := NewSiteFacts(db)
	facts, err := reader.SiteFacts(context.Background(), workspace)
	if err != nil || facts != (tutorial.SiteFacts{HasPage: true}) {
		t.Fatalf("only a page so far = %+v, %v", facts, err)
	}

	second := addProject(t, db, workspace)
	addRevision(t, db, workspace, second, "person:abc", "autosave")
	addSettings(t, db, workspace, second, `{"forms":{"contato":{"funnel_id":"f1"}},"bookings":{"agenda":{"duration_minutes":30}},"site":{"webchat_widget":"w1"}}`)
	addHostname(t, db, workspace, second, "custom", "active")
	addDeployment(t, db, workspace, second, "superseded")

	facts, err = reader.SiteFacts(context.Background(), workspace)
	want := tutorial.SiteFacts{HasPage: true, EditedByHand: true, FormToFunnel: true, SiteChat: true, Booking: true, CustomDomain: true, Published: true}
	if err != nil || facts != want {
		t.Fatalf("facts = %+v, %v; want %+v", facts, err, want)
	}
}
