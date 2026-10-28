package sitepublish_usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"vozko/domain/sitepublish"
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

type staleFirstRead struct {
	*memHostnames
	stale map[string]sitepublish.HostnameRecord
}

func (s *staleFirstRead) Get(ctx context.Context, workspaceID, id string) (sitepublish.HostnameRecord, error) {
	if rec, ok := s.stale[id]; ok {
		delete(s.stale, id)
		return rec, nil
	}
	return s.memHostnames.Get(ctx, workspaceID, id)
}

type staleListing struct {
	*memHostnames
	listed []sitepublish.HostnameRecord
}

func (s staleListing) ListPage(_ context.Context, page sitepublish.HostnamePage) ([]sitepublish.HostnameRecord, error) {
	var out []sitepublish.HostnameRecord
	for _, rec := range s.listed {
		if slices.Contains(page.Statuses, rec.Status) && rec.ID > page.AfterID {
			out = append(out, rec)
		}
	}
	return out, nil
}

func TestARemovalDuringProvisioningDeletesTheProviderRecordItCreated(t *testing.T) {
	r := newRig(t)
	rec := addDomain(t, r, "www.cliente.com.br")
	stale := r.hostnames.rows[rec.ID]
	r.dns.prove(rec)
	provisioned, err := r.svc.VerifyHostname(context.Background(), actor, projectID, rec.ID)
	if err != nil || provisioned.CloudflareRef == "" {
		t.Fatalf("got %+v %v", provisioned, err)
	}
	r.svc.Hostnames = &staleFirstRead{memHostnames: r.hostnames, stale: map[string]sitepublish.HostnameRecord{rec.ID: stale}}

	if _, err := r.svc.RemoveHostname(context.Background(), actor, projectID, rec.ID); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.cloudflare.deleted, provisioned.CloudflareRef) {
		t.Fatalf("the removal reads the row inside the lock and deletes the provider record, deleted %v", r.cloudflare.deleted)
	}
	if _, kept := r.hostnames.rows[rec.ID]; kept {
		t.Fatal("the row goes once the provider record is gone")
	}
}

func TestARemovalWaitsForTheDomainLock(t *testing.T) {
	r := newRig(t)
	rec := addDomain(t, r, "www.cliente.com.br")
	r.locks.held[domainLockPrefix+rec.ID] = "provisioning"

	if _, err := r.svc.RemoveHostname(context.Background(), actor, projectID, rec.ID); !errors.Is(err, ErrDomainBusy) {
		t.Fatalf("a removal never runs beside a provisioning step, got %v", err)
	}
	if got := r.hostnames.rows[rec.ID]; got.Status != sitepublish.HostnamePendingDNS {
		t.Fatalf("nothing changes while the lock is held, got %+v", got)
	}

	r.locks.err = errors.New("redis down")
	delete(r.locks.held, domainLockPrefix+rec.ID)
	if _, err := r.svc.RemoveHostname(context.Background(), actor, projectID, rec.ID); err == nil {
		t.Fatal("an unreadable lock refuses the removal")
	}
	if got := r.hostnames.rows[rec.ID]; got.Status != sitepublish.HostnamePendingDNS {
		t.Fatalf("never removed without the lock, got %+v", got)
	}
}

func TestTheSweepNeverActsOnARecordListedBeforeItChanged(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	live := liveDomain(t, r, "www.cliente.com.br")
	stale := live
	stale.Status, stale.CloudflareRef = sitepublish.HostnameSuspended, "cf-old"
	r.svc.Hostnames = staleListing{memHostnames: r.hostnames, listed: []sitepublish.HostnameRecord{stale}}

	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := r.hostnames.rows[live.ID]
	if got.Status != sitepublish.HostnameActive || got.CloudflareRef != live.CloudflareRef {
		t.Fatalf("the step reads the current row, got %+v", got)
	}
	if slices.Contains(r.cloudflare.deleted, live.CloudflareRef) {
		t.Fatal("the live provider record is never deleted from a stale listing")
	}
}

func suspendedWithRef(r *rig, i int, ref string) sitepublish.HostnameRecord {
	at := r.svc.Now()
	rec := sitepublish.HostnameRecord{
		ID: fmt.Sprintf("00000000-0000-4000-9000-%012d", i), WorkspaceID: wsID, ProjectID: projectID,
		Hostname: fmt.Sprintf("a%d.cliente.com.br", i), Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnameFailed,
		CloudflareRef: ref, CreatedAt: at, UpdatedAt: at,
	}
	if ref != "" {
		rec.Hostname = fmt.Sprintf("z%d.cliente.com.br", i)
		rec.Status, rec.Verification.Offline = sitepublish.HostnameSuspended, sitepublish.OfflineSlots
		r.cloudflare.states[ref] = sitepublish.CustomHostnameState{Ref: ref, Hostname: rec.Hostname}
	}
	r.hostnames.rows[rec.ID] = rec
	return rec
}

func TestTheSweepRetriesEveryProviderDeletionHoweverManyRowsWait(t *testing.T) {
	r := newRig(t)
	r.build(t, 0)
	for i := 0; i < sweepBatch+50; i++ {
		suspendedWithRef(r, i, "")
	}
	var waiting []sitepublish.HostnameRecord
	for i := sweepBatch + 50; i < 3*sweepBatch+60; i++ {
		waiting = append(waiting, suspendedWithRef(r, i, fmt.Sprintf("cf-%d", i)))
	}

	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, rec := range waiting {
		if got := r.hostnames.rows[rec.ID]; got.CloudflareRef != "" || !slices.Contains(r.cloudflare.deleted, rec.CloudflareRef) {
			t.Fatalf("%s still holds its provider record, got %+v", rec.Hostname, got)
		}
	}
}

func TestARemovalWhoseProviderRecordIsAlreadyGoneFinishes(t *testing.T) {
	r := newRig(t)
	rec := suspendedWithRef(r, 1, "cf-gone")
	rec.Status = sitepublish.HostnameRemoving
	r.hostnames.rows[rec.ID] = rec
	delete(r.cloudflare.states, "cf-gone")
	r.svc.CustomHostnames = goneOnDelete{r.cloudflare}

	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, kept := r.hostnames.rows[rec.ID]; kept {
		t.Fatal("a provider record that no longer exists never blocks the removal")
	}
}

type goneOnDelete struct{ *fakeCloudflare }

func (g goneOnDelete) Delete(_ context.Context, ref string) error {
	if _, ok := g.states[ref]; !ok {
		return sitepublish.ErrHostnameNotFound
	}
	return g.fakeCloudflare.Delete(context.Background(), ref)
}

func TestADeterministicAllowanceErrorTakesTheDomainsOffline(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fail   func(*rig)
		reason sitepublish.OfflineReason
	}{
		{"plan missing", func(r *rig) { r.entitled.err = workspace_plan.ErrPlanNotFound }, sitepublish.OfflineBilling},
		{"subscription missing", func(r *rig) { r.subs.err = workspace_plan.ErrSubscriptionNotFound }, sitepublish.OfflineBilling},
		{"entitlement unknown", func(r *rig) { r.entitled.err = workspace_addon.ErrInvalidEntitlementKind }, sitepublish.OfflineSlots},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			publish(t, r)
			rec := liveDomain(t, r, "www.cliente.com.br")
			tc.fail(r)

			if err := r.svc.Sweep(context.Background()); err != nil {
				t.Fatal(err)
			}
			got := r.hostnames.rows[rec.ID]
			if got.Status != sitepublish.HostnameSuspended || got.Verification.Offline != tc.reason || got.CloudflareRef != "" {
				t.Fatalf("an error that repeats forever means no slot, got %+v", got)
			}
			if _, served := r.hosting.hosts[rec.Hostname]; served {
				t.Fatal("the serving key goes at once")
			}
		})
	}
}

func TestATransientAllowanceErrorTakesTheDomainsOfflineAfterRepeatedSweeps(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	rec := liveDomain(t, r, "www.cliente.com.br")
	r.entitled.err = errors.New("database down")

	for i := 1; i < allowanceFailuresBeforeCut; i++ {
		if err := r.svc.Sweep(context.Background()); err != nil {
			t.Fatal(err)
		}
		if got := r.hostnames.rows[rec.ID]; got.Status != sitepublish.HostnameActive {
			t.Fatalf("sweep %d: a short blip keeps the domain up, got %+v", i, got)
		}
	}
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := r.hostnames.rows[rec.ID]
	if got.Status != sitepublish.HostnameSuspended || got.Verification.Offline != sitepublish.OfflineBilling || got.CloudflareRef != "" {
		t.Fatalf("an allowance that stays unreadable takes the domain offline, got %+v", got)
	}

	r.entitled.err = nil
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := r.hostnames.rows[rec.ID]; got.Status != sitepublish.HostnamePendingSSL {
		t.Fatalf("a readable allowance brings it back, got %+v", got)
	}
}

func TestAReadableAllowanceStartsTheFailureCountAgain(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	rec := liveDomain(t, r, "www.cliente.com.br")
	for round := 0; round < 3; round++ {
		r.entitled.err = errors.New("database down")
		for i := 1; i < allowanceFailuresBeforeCut; i++ {
			if err := r.svc.Sweep(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		r.entitled.err = nil
		if err := r.svc.Sweep(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := r.hostnames.rows[rec.ID]; got.Status != sitepublish.HostnameActive {
		t.Fatalf("only consecutive failures count, got %+v", got)
	}
}

func TestAnUncountableAllowanceFailureTakesTheDomainsOffline(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	rec := liveDomain(t, r, "www.cliente.com.br")
	r.entitled.err = errors.New("database down")
	r.counters.err = errors.New("redis down")

	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := r.hostnames.rows[rec.ID]; got.Status != sitepublish.HostnameSuspended {
		t.Fatalf("a failure that cannot be counted fails closed, got %+v", got)
	}
}
