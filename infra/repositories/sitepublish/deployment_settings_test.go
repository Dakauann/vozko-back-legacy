package sitepublish_repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

func TestDeploymentRowsKeepThePublishedSettings(t *testing.T) {
	rec := sitepublish.DeploymentRecord{
		ID: "11111111-1111-4111-8111-111111111111", WorkspaceID: projectID, ProjectID: projectID, Status: sitepublish.DeploymentActive, CreatedAt: time.Now(),
		Settings: site.Settings{Integrations: site.Integrations{GA4: "G-ABC1234"}, Redirects: []site.Redirect{{From: "/velho", To: "https://loja.example.com/", Status: 301}}},
	}
	row, err := deploymentRow(rec)
	if err != nil {
		t.Fatal(err)
	}
	back, err := deploymentRecord(row)
	if err != nil {
		t.Fatal(err)
	}
	if back.Settings.Integrations.GA4 != "G-ABC1234" || len(back.Settings.Redirects) != 1 || back.Settings.Redirects[0].To != "https://loja.example.com/" {
		t.Fatalf("settings lost: %+v", back.Settings)
	}
	empty, err := deploymentRow(sitepublish.DeploymentRecord{ID: rec.ID, ProjectID: projectID, Status: sitepublish.DeploymentReady})
	if err != nil || string(empty.Settings) != "{}" {
		t.Fatalf("empty settings are stored as an object: %q %v", empty.Settings, err)
	}
}

func TestDeploymentRowsRefuseUnreadableSettings(t *testing.T) {
	row, err := deploymentRow(sitepublish.DeploymentRecord{ID: "11111111-1111-4111-8111-111111111111", ProjectID: projectID, Status: sitepublish.DeploymentReady})
	if err != nil {
		t.Fatal(err)
	}
	row.Settings = []byte(`{"redirects":"x"}`)
	if _, err := deploymentRecord(row); err == nil {
		t.Fatal("unreadable settings must refuse")
	}
}

func TestDeploymentUpdateWritesTheSettings(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "site_deployments" SET .*"settings"=`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err := NewDeployments(db).Update(context.Background(), sitepublish.DeploymentRecord{ID: "11111111-1111-4111-8111-111111111111", ProjectID: projectID, Status: sitepublish.DeploymentReady})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
