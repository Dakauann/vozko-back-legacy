package sitepublish_usecase

import (
	"context"
	"errors"
	"slices"
	"testing"

	"vozko/domain/sitepublish"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

func TestAFailedPublishRecordsNothingAndKeepsTheEditorVersion(t *testing.T) {
	r := newRig(t)
	r.builder.err = errors.New("builder down")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err == nil {
		t.Fatal("the build failed")
	}
	if len(r.projects.published) != 0 || len(r.projects.publications) != 0 {
		t.Fatalf("a failed publish records nothing: %+v %+v", r.projects.published, r.projects.publications)
	}
	if r.deployments.rows[r.deployments.seq[0]].RevisionID != "rev-7" {
		t.Fatalf("the build reads the working revision: %+v", r.deployments.rows[r.deployments.seq[0]])
	}
}

func TestAPublishMarksTheBuiltRevisionWithoutANewVersion(t *testing.T) {
	r := newRig(t)
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	got := r.projects.published[0]
	if got.SourceRevisionID != "rev-7" || got.SourceVersion != 12 || got.DeploymentID != res.Deployment.ID {
		t.Fatalf("record %+v", got)
	}
	if res.ProjectVersion != 12 || res.RevisionID != "rev-publish-"+res.Deployment.ID || res.HasUnpublishedChanges {
		t.Fatalf("result %+v", res)
	}
	r.projects.editedTo = 13
	res, err = r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if res.ProjectVersion != 13 || !res.HasUnpublishedChanges {
		t.Fatalf("an edit made while publishing stays unpublished: %+v", res)
	}
}

func TestTheFinalRecordNeverWritesADeploymentStatus(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	publish(t, r)
	if len(r.deployments.statusAfter) != 0 {
		t.Fatalf("status written after going live: %v", r.deployments.statusAfter)
	}
	r = newRig(t)
	r.deployments.recordErr = errors.New("database down")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrPublishUnrecorded) {
		t.Fatalf("got %v", err)
	}
}

func TestOnePublishAtATimePerPage(t *testing.T) {
	r := newRig(t)
	r.locks.held[publishLockKey(projectID)] = "other"
	if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrPublishInProgress) {
		t.Fatalf("got %v", err)
	}
	if _, err := r.svc.Rollback(context.Background(), actor, projectID, "d1"); !errors.Is(err, ErrPublishInProgress) {
		t.Fatalf("got %v", err)
	}
	if len(r.deployments.seq) != 0 {
		t.Fatal("nothing was built")
	}
	delete(r.locks.held, publishLockKey(projectID))
	r.builder.err = errors.New("builder down")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err == nil {
		t.Fatal("the build failed")
	}
	if len(r.locks.held) != 0 {
		t.Fatalf("the lock is released after a failure: %v", r.locks.held)
	}
	r.locks.err = errors.New("redis down")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err == nil || errors.Is(err, ErrPublishInProgress) {
		t.Fatalf("an unreadable lock refuses: %v", err)
	}
}

func TestAPinnedPublishRefusesAPageThatChanged(t *testing.T) {
	r := newRig(t)
	if _, err := r.svc.PublishPinned(context.Background(), actor, projectID, 11); !errors.Is(err, ErrPublishStale) {
		t.Fatalf("got %v", err)
	}
	if len(r.deployments.seq) != 0 {
		t.Fatal("nothing was built")
	}
	if _, err := r.svc.PublishPinned(context.Background(), actor, projectID, 12); err != nil {
		t.Fatal(err)
	}
}

func TestAFailedGoLiveReactivatesThePreviousForms(t *testing.T) {
	r := newRig(t)
	first := publish(t, r)
	r.deployments.activateErr = errors.New("database down")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err == nil {
		t.Fatal("activation failed")
	}
	last := r.forms.activated[len(r.forms.activated)-1]
	if last.DeploymentID != first.ID {
		t.Fatalf("the previous forms come back: %+v", last)
	}
	r = newRig(t)
	r.deployments.activateErr = errors.New("database down")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err == nil {
		t.Fatal("activation failed")
	}
	if !slices.Contains(r.forms.deactivated, projectID) {
		t.Fatal("a first publish that fails leaves no form active")
	}
}

type failingAbuse struct{ *memAbuse }

func (f failingAbuse) Create(context.Context, sitepublish.AbuseCase) error {
	return errors.New("database down")
}

func TestAHeldPublishWithoutItsHoldRecordRefuses(t *testing.T) {
	r := newRig(t)
	r.builder.candidate = htmlCandidate(formHTML)
	r.svc.Abuse = failingAbuse{r.abuse}
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err == nil || res.Held {
		t.Fatalf("got %+v %v", res, err)
	}
	if got := r.deployments.rows[r.deployments.seq[0]].Status; got != sitepublish.DeploymentFailed {
		t.Fatalf("never a ready deployment without its hold: %s", got)
	}
}

type removingOnWrite struct {
	*fakeHosting
	hostnames *memHostnames
	target    string
}

func (f *removingOnWrite) PutHost(ctx context.Context, host string, value []byte) error {
	if err := f.fakeHosting.PutHost(ctx, host, value); err != nil {
		return err
	}
	if host == f.target {
		rec := f.hostnames.rows["custom"]
		rec.Status = sitepublish.HostnameRemoving
		f.hostnames.rows["custom"] = rec
	}
	return nil
}

func TestAHostnameRemovedWhileKeysAreWrittenDoesNotServe(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	r.hostnames.rows["custom"] = sitepublish.HostnameRecord{ID: "custom", WorkspaceID: wsID, ProjectID: projectID, Hostname: "www.cliente.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnameActive}
	r.svc.Hosting = &removingOnWrite{fakeHosting: r.hosting, hostnames: r.hostnames, target: "www.cliente.com.br"}
	publish(t, r)
	if _, ok := r.hosting.hosts["www.cliente.com.br"]; ok {
		t.Fatal("a removed hostname keeps no key")
	}
	if _, ok := r.hosting.hosts[liveHost]; !ok {
		t.Fatal("the platform hostname serves")
	}
}

func TestPublishRecordsOutboundLinksAndRollbackKeepsThem(t *testing.T) {
	r := newRig(t)
	first := publish(t, r)
	if !slices.Contains(r.deployments.rows[first.ID].Outbound, "https://wa.me/5511999999999") {
		t.Fatalf("outbound %v", r.deployments.rows[first.ID].Outbound)
	}
	publish(t, r)
	back, err := r.svc.Rollback(context.Background(), actor, projectID, first.ID)
	if err != nil || back.Status != sitepublish.DeploymentActive || !slices.Contains(back.Outbound, "https://wa.me/5511999999999") {
		t.Fatalf("got %+v %v", back, err)
	}
}

func TestRetiringAPageRemovesItsHostnames(t *testing.T) {
	r := newRig(t)
	r.hostnames.rows["custom"] = sitepublish.HostnameRecord{ID: "custom", WorkspaceID: wsID, ProjectID: projectID, Hostname: "www.cliente.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnamePendingDNS, CloudflareRef: "cf-1"}
	r.cloudflare.states["cf-1"] = sitepublish.CustomHostnameState{Ref: "cf-1"}
	if err := r.svc.Retire(context.Background(), wsID, projectID); err != nil {
		t.Fatal(err)
	}
	if len(r.hostnames.rows) != 0 || !slices.Contains(r.cloudflare.deleted, "cf-1") {
		t.Fatalf("rows %+v deleted %v", r.hostnames.rows, r.cloudflare.deleted)
	}
	r.hostnames.rows["custom"] = sitepublish.HostnameRecord{ID: "custom", WorkspaceID: wsID, ProjectID: projectID, Hostname: "www.cliente.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnameActive, CloudflareRef: "cf-2"}
	r.cloudflare.err = errors.New("cloudflare down")
	if err := r.svc.Retire(context.Background(), wsID, projectID); err != nil {
		t.Fatal(err)
	}
	if r.hostnames.rows["custom"].Status != sitepublish.HostnameRemoving {
		t.Fatalf("the sweep finishes what the provider refused: %+v", r.hostnames.rows["custom"])
	}
}

func TestTheDomainQuotaIsCheckedWhereTheRowIsCreated(t *testing.T) {
	r := newRig(t)
	r.build(t, 0)
	if _, err := r.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnameCustom, Hostname: "www.cliente.com.br"}); !errors.Is(err, ErrSiteDomainsExhausted) {
		t.Fatalf("got %v", err)
	}
	if len(r.cloudflare.created) != 0 {
		t.Fatal("nothing reaches the provider")
	}
}

func TestActivationStampsTheDomain(t *testing.T) {
	r := newRig(t)
	r.hostnames.rows["custom"] = sitepublish.HostnameRecord{ID: "custom", WorkspaceID: wsID, ProjectID: projectID, Hostname: "www.cliente.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnamePendingSSL, CloudflareRef: "cf-1", CreatedAt: r.svc.Now()}
	r.cloudflare.states["cf-1"] = sitepublish.CustomHostnameState{Ref: "cf-1", Phase: sitepublish.HostnameActive}
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.hostnames.rows["custom"].Verification.ActivatedAt == nil {
		t.Fatal("activation is stamped")
	}
}

func TestTheSweepRepairsAMissedBillingHook(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	r.subs.err = workspace_plan.ErrSubscriptionNotActive
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !r.hosting.serving(t, liveHost).Suspended || r.projects.last().state != PublicationSuspended {
		t.Fatal("an unpaid workspace goes offline even when the hook failed")
	}
	writes := len(r.projects.publications)
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.projects.publications) != writes {
		t.Fatal("a settled page is not rewritten")
	}
	r.subs.err = nil
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.hosting.serving(t, liveHost).Suspended || r.projects.last().state != PublicationLive {
		t.Fatal("a paying workspace comes back even when the hook failed")
	}
}

type failingProjectHosting struct {
	*fakeHosting
	host string
}

func (f *failingProjectHosting) PutHost(ctx context.Context, host string, value []byte) error {
	if host == f.host {
		return errors.New("kv refused")
	}
	return f.fakeHosting.PutHost(ctx, host, value)
}

func TestARestoreContinuesPastAFailingPage(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	other := "9a6c2a52-1b0f-4f7e-9a34-0d5f8f1e2c3b"
	r.deployments.rows["other"] = sitepublish.DeploymentRecord{ID: "other", WorkspaceID: wsID, ProjectID: other, Status: sitepublish.DeploymentActive}
	r.deployments.seq = append([]string{"other"}, r.deployments.seq...)
	r.hostnames.rows["other-host"] = sitepublish.HostnameRecord{ID: "other-host", WorkspaceID: wsID, ProjectID: other, Hostname: "outra.paginas.test", Kind: sitepublish.HostnamePlatform, Status: sitepublish.HostnameActive}
	r.svc.Hosting = &failingProjectHosting{fakeHosting: r.hosting, host: "outra.paginas.test"}
	r.subs.err = workspace_plan.ErrSubscriptionNotActive
	if err := r.svc.OnAccessCut(context.Background(), wsID); err == nil {
		t.Fatal("the failing page is reported")
	}
	if !r.hosting.serving(t, liveHost).Suspended {
		t.Fatal("the other pages are still settled")
	}
}
