package sitepublish_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

func (f *fakeProjects) CurrentSettings(_ context.Context, workspaceID, id string) (site.Settings, error) {
	if f.err != nil {
		return site.Settings{}, f.err
	}
	if workspaceID != f.snap.WorkspaceID || id != f.snap.ID {
		return site.Settings{}, ErrProjectNotFound
	}
	return f.snap.Settings, nil
}

func TestSettingsChangesCompareTheLiveDeploymentWithTheCurrentSettings(t *testing.T) {
	r := newRig(t)
	r.projects.snap.Settings = site.Settings{Redirects: []site.Redirect{{From: "/velho", To: "/", Status: 301}}}
	first, err := r.svc.SettingsChanges(context.Background(), actor, projectID)
	if err != nil || !first.First || len(first.Redirects) != 1 || first.Redirects[0].Action != sitepublish.ChangeAdded {
		t.Fatalf("before any publish everything is new: %+v %v", first, err)
	}
	dep := publish(t, r)
	if stored := r.deployments.rows[dep.ID].Settings; len(stored.Redirects) != 1 {
		t.Fatalf("the deployment keeps the settings it went live with: %+v", stored)
	}
	same, err := r.svc.SettingsChanges(context.Background(), actor, projectID)
	if err != nil || same.First || !same.Empty() {
		t.Fatalf("nothing changed since the publish: %+v %v", same, err)
	}
	r.projects.snap.Settings.Redirects = []site.Redirect{{From: "/velho", To: "https://golpe.example.net/", Status: 302}}
	r.projects.snap.Settings.Integrations.GA4 = "G-ABC1234"
	changed, err := r.svc.SettingsChanges(context.Background(), actor, projectID)
	if err != nil || len(changed.Redirects) != 1 || changed.Redirects[0].PreviousTo != "/" || len(changed.Integrations) != 1 || !changed.Routing() {
		t.Fatalf("got %+v %v", changed, err)
	}
}

func TestSettingsChangesNeedReadAccessAndTheOwnWorkspace(t *testing.T) {
	r := newRig(t)
	r.access.denied["sites:read"] = true
	if _, err := r.svc.SettingsChanges(context.Background(), actor, projectID); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("got %v", err)
	}
	r.access.denied["sites:read"] = false
	other := Actor{UserID: userID, WorkspaceID: "ws-other"}
	if _, err := r.svc.SettingsChanges(context.Background(), other, projectID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("another workspace never reads the page: %v", err)
	}
	publish(t, r)
	r.projects.snap.WorkspaceID = "ws-other"
	r.projects.snap.ID = projectID
	if _, err := r.svc.SettingsChanges(context.Background(), other, projectID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("a live deployment of another workspace is never compared: %v", err)
	}
}

func TestSettingsChangesSurfaceStoreErrors(t *testing.T) {
	r := newRig(t)
	r.projects.err = errors.New("down")
	if _, err := r.svc.SettingsChanges(context.Background(), actor, projectID); err == nil {
		t.Fatal("an unreadable settings document refuses")
	}
}
