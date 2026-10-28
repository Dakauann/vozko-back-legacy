package sitepublish_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/sitepublish"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

const liveHost = "bolos-loja-da-ana.paginas.test"

func TestBillingCutSuspendsAndPaymentRestores(t *testing.T) {
	r := newRig(t)
	dep := publish(t, r)
	r.subs.err = workspace_plan.ErrSubscriptionNotActive
	if err := r.svc.OnAccessCut(context.Background(), wsID); err != nil {
		t.Fatal(err)
	}
	if s := r.hosting.serving(t, liveHost); !s.Suspended || s.DeploymentID != dep.ID {
		t.Fatalf("got %+v", s)
	}
	deploy, _ := sitepublish.DecodeServing(r.hosting.deployments[dep.ID])
	if !deploy.Suspended {
		t.Fatal("the immutable address is suspended too")
	}
	r.subs.err = nil
	if err := r.svc.OnAccessRestored(context.Background(), wsID); err != nil {
		t.Fatal(err)
	}
	if r.hosting.serving(t, liveHost).Suspended {
		t.Fatal("payment restores the page")
	}
}

func TestPaymentDoesNotLiftAnAdminSuspension(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	if _, err := r.svc.AdminSuspend(context.Background(), "admin-1", projectID, "phishing denunciado"); err != nil {
		t.Fatal(err)
	}
	if !r.hosting.serving(t, liveHost).Suspended {
		t.Fatal("admin suspension flips the key")
	}
	if err := r.svc.OnAccessRestored(context.Background(), wsID); err != nil {
		t.Fatal(err)
	}
	if !r.hosting.serving(t, liveHost).Suspended {
		t.Fatal("a payment never lifts an admin suspension")
	}
	if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrSiteSuspended) {
		t.Fatalf("a suspended page cannot publish, got %v", err)
	}
	closed, err := r.svc.AdminRestore(context.Background(), "admin-1", projectID, "resolvido")
	if err != nil || len(closed) != 1 || closed[0].Status != sitepublish.AbuseRestored {
		t.Fatalf("got %+v %v", closed, err)
	}
	if r.hosting.serving(t, liveHost).Suspended {
		t.Fatal("restored")
	}
}

func TestAdminActionsNeedAReason(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	if _, err := r.svc.AdminSuspend(context.Background(), "admin-1", projectID, "  "); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("got %v", err)
	}
	if _, err := r.svc.AdminSuspend(context.Background(), "admin-1", "unknown", "x"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestSuspendSurfacesKVFailuresForTheBillingRetry(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	r.hosting.err = errors.New("kv down")
	if err := r.svc.Suspend(context.Background(), wsID); !errors.Is(err, ErrHostingUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestAdminCasesCarryThePageAndWorkspaceNames(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	if _, err := r.svc.AdminSuspend(context.Background(), "admin-1", projectID, "phishing"); err != nil {
		t.Fatal(err)
	}
	r.abuse.rows = append(r.abuse.rows, sitepublish.AbuseCase{ID: "gone", WorkspaceID: wsID, ProjectID: "deleted-page", Status: sitepublish.AbuseHeld})
	cases, err := r.svc.AdminCases(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[0].ProjectName != "Bolos" || cases[0].WorkspaceName != "Loja da Ana" || cases[0].Reason != "phishing" {
		t.Fatalf("cases %+v", cases)
	}
	if cases[1].ProjectName != "" || cases[1].WorkspaceName != "Loja da Ana" {
		t.Fatalf("a deleted page keeps its case without a name: %+v", cases[1])
	}
}
