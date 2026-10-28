package sitepublish_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/sitepublish"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

func publish(t *testing.T, r *rig) sitepublish.DeploymentRecord {
	t.Helper()
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	return res.Deployment
}

func TestRollbackFlipsBackToAnEarlierDeployment(t *testing.T) {
	r := newRig(t)
	first := publish(t, r)
	second := publish(t, r)
	got, err := r.svc.Rollback(context.Background(), actor, projectID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != sitepublish.DeploymentActive || r.deployments.rows[second.ID].Status != sitepublish.DeploymentSuperseded {
		t.Fatalf("got %+v", got)
	}
	if r.hosting.serving(t, "bolos-loja-da-ana.paginas.test").DeploymentID != first.ID || len(r.forms.activated) != 3 || r.forms.activated[2].DeploymentID != first.ID {
		t.Fatal("rollback flips the host key and reactivates the old forms")
	}
	if _, err := r.svc.Rollback(context.Background(), actor, "other-project", first.ID); !errors.Is(err, sitepublish.ErrDeploymentNotFound) {
		t.Fatalf("another page's deployment, got %v", err)
	}
}

func TestRollbackRefusesFailedDeployments(t *testing.T) {
	r := newRig(t)
	r.deployments.rows["d1"] = sitepublish.DeploymentRecord{ID: "d1", WorkspaceID: wsID, ProjectID: projectID, Status: sitepublish.DeploymentFailed}
	r.deployments.seq = append(r.deployments.seq, "d1")
	if _, err := r.svc.Rollback(context.Background(), actor, projectID, "d1"); !errors.Is(err, sitepublish.ErrDeploymentTransition) {
		t.Fatalf("got %v", err)
	}
}

func TestHostnamesNeedAPaidWorkspaceAndPermission(t *testing.T) {
	r := newRig(t)
	r.subs.err = workspace_plan.ErrSubscriptionNotActive
	if _, err := r.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnameCustom, Hostname: "www.cliente.com.br"}); !errors.Is(err, ErrSubscriptionInactive) {
		t.Fatalf("got %v", err)
	}
	r = newRig(t)
	r.access.denied["site_domains:update"] = true
	if _, err := r.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnamePlatform}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("got %v", err)
	}
}

func TestPlatformSubdomainsAvoidTakenAndReservedNames(t *testing.T) {
	r := newRig(t)
	r.hostnames.rows["other"] = sitepublish.HostnameRecord{ID: "other", WorkspaceID: "ws-2", ProjectID: "p2", Hostname: "bolos-loja-da-ana.paginas.test", Kind: sitepublish.HostnamePlatform, Status: sitepublish.HostnameActive}
	rec, err := r.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnamePlatform})
	if err != nil || rec.Hostname != "bolos-loja-da-ana-2.paginas.test" {
		t.Fatalf("got %+v %v", rec, err)
	}
	if _, err := r.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnamePlatform, Slug: "outra"}); !errors.Is(err, ErrPlatformHostnameTaken) {
		t.Fatalf("one subdomain per page, got %v", err)
	}
	r2 := newRig(t)
	if _, err := r2.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnamePlatform, Slug: "vozko"}); !errors.Is(err, sitepublish.ErrSlugReserved) {
		t.Fatalf("brand words are reserved, got %v", err)
	}
}

func TestAddingASubdomainToALivePageServesItAtOnce(t *testing.T) {
	r := newRig(t)
	dep := publish(t, r)
	platform, _, _ := r.svc.platformRecord(context.Background(), wsID, projectID)
	if _, err := r.svc.RemoveHostname(context.Background(), actor, projectID, platform.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.hosting.hosts[platform.Hostname]; ok || len(r.hostnames.rows) != 0 {
		t.Fatal("removing deletes the key and the row")
	}
	rec, err := r.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnamePlatform, Slug: "bolos-da-ana"})
	if err != nil {
		t.Fatal(err)
	}
	if r.hosting.serving(t, rec.Hostname).DeploymentID != dep.ID {
		t.Fatal("a new subdomain of a live page is served")
	}
}
