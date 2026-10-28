package sitepublish_usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"vozko/domain/dnsrecord"
	"vozko/domain/sitepublish"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

func addDomain(t *testing.T, r *rig, host string) sitepublish.HostnameRecord {
	t.Helper()
	rec, err := r.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnameCustom, Hostname: host})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func liveDomain(t *testing.T, r *rig, host string) sitepublish.HostnameRecord {
	t.Helper()
	rec := addDomain(t, r, host)
	r.dns.prove(rec)
	rec, err := r.svc.VerifyHostname(context.Background(), actor, projectID, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	state := r.cloudflare.states[rec.CloudflareRef]
	state.Phase, state.SSLStatus = sitepublish.HostnameActive, "active"
	r.cloudflare.states[rec.CloudflareRef] = state
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := r.hostnames.rows[rec.ID]
	if got.Status != sitepublish.HostnameActive {
		t.Fatalf("%s should be live, got %+v", host, got)
	}
	return got
}

func TestADomainReachesTheProviderOnlyAfterItsOwnershipIsProven(t *testing.T) {
	r := newRig(t)
	rec := addDomain(t, r, "www.Cliente.com.br")
	if rec.Hostname != "www.cliente.com.br" || rec.Status != sitepublish.HostnamePendingDNS || rec.CloudflareRef != "" || rec.Verification.CNAMETarget != "sites.paginas.test" {
		t.Fatalf("a new domain waits for its TXT record, got %+v", rec)
	}
	if rec.Verification.OwnershipToken == "" || rec.Verification.OwnershipStatus != dnsrecord.Pending || rec.Verification.PendingSince == nil {
		t.Fatalf("the ownership challenge is recorded, got %+v", rec.Verification)
	}
	if len(r.cloudflare.created) != 0 {
		t.Fatal("nothing that costs money happens before the ownership check")
	}
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.cloudflare.created) != 0 || r.hostnames.rows[rec.ID].Status != sitepublish.HostnamePendingDNS {
		t.Fatal("without the TXT record the sweep keeps waiting")
	}
	r.dns.prove(rec)
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := r.hostnames.rows[rec.ID]
	if got.Status != sitepublish.HostnamePendingSSL || got.CloudflareRef != "cf-www.cliente.com.br" || got.Verification.OwnershipVerifiedAt == nil {
		t.Fatalf("a proven domain goes to the provider for its certificate, got %+v", got)
	}
	if _, served := r.hosting.hosts["www.cliente.com.br"]; served {
		t.Fatal("never served before the certificate")
	}
}

func TestATXTAlreadyInPlaceIsCheckedWhenTheDomainIsAdded(t *testing.T) {
	r := newRig(t)
	r.svc.NewID = func() string { return "11111111-2222-4333-8444-555555555555" }
	r.dns.prove(sitepublish.HostnameRecord{Hostname: "www.cliente.com.br", Verification: sitepublish.Verification{OwnershipToken: "11111111222243338444555555555555"}})
	rec := addDomain(t, r, "www.cliente.com.br")
	if rec.Status != sitepublish.HostnamePendingSSL || len(r.cloudflare.created) != 1 {
		t.Fatalf("got %+v created %v", rec, r.cloudflare.created)
	}
}

func TestADomainNeedsAFreeSlot(t *testing.T) {
	r := newRig(t)
	r.build(t, 1)
	addDomain(t, r, "www.cliente.com.br")
	if _, err := r.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnameCustom, Hostname: "loja.cliente.com.br"}); !errors.Is(err, ErrSiteDomainsExhausted) {
		t.Fatalf("one slot holds one domain, got %v", err)
	}
	for _, bad := range []string{"https://x.com/a", "loja.paginas.test", "app.vozko.test", "intranet.local"} {
		r.build(t, 10)
		if _, err := r.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnameCustom, Hostname: bad}); err == nil {
			t.Fatalf("%s must refuse", bad)
		}
	}
}

func TestAnUnreadableEntitlementRefusesTheDomain(t *testing.T) {
	r := newRig(t)
	r.entitled.err = errors.New("database down")
	if _, err := r.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnameCustom, Hostname: "www.cliente.com.br"}); err == nil {
		t.Fatal("an unreadable entitlement must refuse")
	}
	r = newRig(t)
	r.reversals.err = errors.New("database down")
	if _, err := r.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnameCustom, Hostname: "www.cliente.com.br"}); err == nil {
		t.Fatal("an unreadable reversal state must refuse")
	}
	if len(r.hostnames.rows) != 0 {
		t.Fatalf("nothing is created, rows %+v", r.hostnames.rows)
	}
}

func TestDomainsCannotBeAddedFasterThanTheDailyLimit(t *testing.T) {
	r := newRig(t)
	r.build(t, 100)
	for i := 0; i < sitepublish.MaxCustomDomainAddsPerDay; i++ {
		addDomain(t, r, "d"+strings.Repeat("x", i+1)+".cliente.com.br")
	}
	if _, err := r.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnameCustom, Hostname: "mais.cliente.com.br"}); !errors.Is(err, ErrDomainAddsLimited) {
		t.Fatalf("got %v", err)
	}
	r2 := newRig(t)
	r2.build(t, 0)
	for i := 0; i < sitepublish.MaxCustomDomainAddsPerDay+1; i++ {
		if _, err := r2.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnameCustom, Hostname: "www.cliente.com.br"}); !errors.Is(err, ErrSiteDomainsExhausted) {
			t.Fatalf("a refused add gives its daily slot back, got %v", err)
		}
	}
	r3 := newRig(t)
	r3.locks.err = errors.New("redis down")
	if _, err := r3.svc.AddHostname(context.Background(), actor, projectID, AddHostnameInput{Kind: sitepublish.HostnameCustom, Hostname: "www.cliente.com.br"}); !errors.Is(err, ErrDomainAddsLimited) {
		t.Fatalf("an unreadable limiter refuses, got %v", err)
	}
}

func TestAccessCutTakesDomainsOfflineAndOffTheProviderAndPaymentBringsThemBack(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	rec := liveDomain(t, r, "www.cliente.com.br")
	ref := rec.CloudflareRef

	r.subs.err = workspace_plan.ErrSubscriptionNotActive
	if err := r.svc.OnAccessCut(context.Background(), wsID); err != nil {
		t.Fatal(err)
	}
	got := r.hostnames.rows[rec.ID]
	if got.Status != sitepublish.HostnameSuspended || got.Verification.Offline != sitepublish.OfflineBilling || got.CloudflareRef != "" {
		t.Fatalf("the domain is offline for billing with no provider record, got %+v", got)
	}
	if !slices.Contains(r.cloudflare.deleted, ref) {
		t.Fatal("the provider record is deleted at once, so no cost accrues")
	}
	if _, served := r.hosting.hosts["www.cliente.com.br"]; served {
		t.Fatal("the domain serves nothing while access is cut")
	}
	if got.Verification.OwnershipVerifiedAt == nil || got.Verification.OwnershipToken != rec.Verification.OwnershipToken {
		t.Fatal("the row and its ownership proof stay")
	}

	r.subs.err = nil
	if err := r.svc.OnAccessRestored(context.Background(), wsID); err != nil {
		t.Fatal(err)
	}
	back := r.hostnames.rows[rec.ID]
	if back.Status != sitepublish.HostnamePendingSSL || back.CloudflareRef == "" || back.Verification.Offline != "" {
		t.Fatalf("payment recreates the provider record without a new TXT check, got %+v", back)
	}
	if n := len(r.cloudflare.created); n != 2 {
		t.Fatalf("created %v", r.cloudflare.created)
	}
}

func TestTheSweepRetriesAProviderDeletionThatFailed(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	rec := liveDomain(t, r, "www.cliente.com.br")
	r.subs.err = workspace_plan.ErrSubscriptionNotActive
	r.cloudflare.err = errors.New("cloudflare down")
	if err := r.svc.OnAccessCut(context.Background(), wsID); err != nil {
		t.Fatal(err)
	}
	got := r.hostnames.rows[rec.ID]
	if got.Status != sitepublish.HostnameSuspended || got.CloudflareRef == "" {
		t.Fatalf("offline at once, provider record kept for the retry, got %+v", got)
	}
	if _, served := r.hosting.hosts["www.cliente.com.br"]; served {
		t.Fatal("never live without paid slots")
	}
	r.cloudflare.err = nil
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.hostnames.rows[rec.ID].CloudflareRef != "" || !slices.Contains(r.cloudflare.deleted, rec.CloudflareRef) {
		t.Fatalf("the sweep finishes the deletion, got %+v", r.hostnames.rows[rec.ID])
	}
}

func TestTheSweepRepairsAMissedAccessCutForDomains(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	rec := liveDomain(t, r, "www.cliente.com.br")
	r.subs.err = workspace_plan.ErrSubscriptionNotCurrent
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := r.hostnames.rows[rec.ID]; got.Status != sitepublish.HostnameSuspended || got.CloudflareRef != "" {
		t.Fatalf("an unpaid workspace loses its domains even when the hook failed, got %+v", got)
	}
}

func TestSlotsLostTakeTheNewestDomainsOfflineFirstAndReturnWithNewSlots(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	clock := r.svc.Now()
	ids := map[string]string{}
	for i, host := range []string{"a.cliente.com.br", "b.cliente.com.br", "c.cliente.com.br"} {
		at := clock.Add(time.Duration(i) * time.Minute)
		r.svc.Now = func() time.Time { return at }
		ids[host] = liveDomain(t, r, host).ID
	}
	r.entitled.allowed = 1
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.hostnames.rows[ids["a.cliente.com.br"]].Status != sitepublish.HostnameActive {
		t.Fatal("the oldest domain keeps its slot")
	}
	for _, host := range []string{"b.cliente.com.br", "c.cliente.com.br"} {
		got := r.hostnames.rows[ids[host]]
		if got.Status != sitepublish.HostnameSuspended || got.Verification.Offline != sitepublish.OfflineSlots || got.CloudflareRef != "" {
			t.Fatalf("%s goes offline and off the provider, got %+v", host, got)
		}
		if _, served := r.hosting.hosts[host]; served {
			t.Fatalf("%s keeps no serving key", host)
		}
	}
	offline := 0
	for _, d := range r.alerts.published {
		if strings.HasPrefix(d.EpisodeKey, "site_domain_quota:") {
			offline++
		}
	}
	if offline != 2 {
		t.Fatalf("one notice per domain, got %d", offline)
	}
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.entitled.allowed = 3
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"b.cliente.com.br", "c.cliente.com.br"} {
		if got := r.hostnames.rows[ids[host]]; got.Status != sitepublish.HostnamePendingSSL || got.CloudflareRef == "" {
			t.Fatalf("%s comes back with the slots, got %+v", host, got)
		}
	}
}

func TestAChargebackTakesDomainsOfflineAtOnceAndAReleaseBringsThemBack(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	rec := liveDomain(t, r, "www.cliente.com.br")
	r.reversals.open = true
	if err := r.svc.OnPaymentReversed(context.Background(), wsID); err != nil {
		t.Fatal(err)
	}
	if got := r.hostnames.rows[rec.ID]; got.Status != sitepublish.HostnameSuspended || got.Verification.Offline != sitepublish.OfflineBilling || got.CloudflareRef != "" {
		t.Fatalf("a reversal cuts the domains at once, got %+v", got)
	}
	r.reversals.open = false
	if err := r.svc.OnReversalReleased(context.Background(), wsID); err != nil {
		t.Fatal(err)
	}
	if got := r.hostnames.rows[rec.ID]; got.Status != sitepublish.HostnamePendingSSL {
		t.Fatalf("a released reversal brings the domain back, got %+v", got)
	}
}

func TestUnverifiedDomainsExpireAfterThreeDays(t *testing.T) {
	r := newRig(t)
	now := r.svc.Now()
	old, fresh := now.Add(-73*time.Hour), now.Add(-time.Hour)
	activated := now.Add(-100 * time.Hour)
	r.hostnames.rows["old"] = sitepublish.HostnameRecord{ID: "old", WorkspaceID: wsID, ProjectID: projectID, Hostname: "old.cliente.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnamePendingDNS, CreatedAt: old, Verification: sitepublish.Verification{PendingSince: &old, OwnershipToken: "t"}}
	r.hostnames.rows["fresh"] = sitepublish.HostnameRecord{ID: "fresh", WorkspaceID: wsID, ProjectID: projectID, Hostname: "fresh.cliente.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnamePendingDNS, CreatedAt: old, Verification: sitepublish.Verification{PendingSince: &fresh, OwnershipToken: "t"}}
	r.hostnames.rows["nossl"] = sitepublish.HostnameRecord{ID: "nossl", WorkspaceID: wsID, ProjectID: projectID, Hostname: "nossl.cliente.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnamePendingSSL, CloudflareRef: "cf-nossl", CreatedAt: old, Verification: sitepublish.Verification{PendingSince: &old}}
	r.hostnames.rows["back"] = sitepublish.HostnameRecord{ID: "back", WorkspaceID: wsID, ProjectID: projectID, Hostname: "back.cliente.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnamePendingSSL, CloudflareRef: "cf-back", CreatedAt: now.Add(-200 * time.Hour), Verification: sitepublish.Verification{ActivatedAt: &activated, PendingSince: &old}}
	for _, ref := range []string{"cf-nossl", "cf-back"} {
		r.cloudflare.states[ref] = sitepublish.CustomHostnameState{Ref: ref, Phase: sitepublish.HostnamePendingSSL}
	}
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.hostnames.rows["old"]; ok {
		t.Fatal("an unverified domain expires and frees its slot")
	}
	if _, ok := r.hostnames.rows["nossl"]; ok || !slices.Contains(r.cloudflare.deleted, "cf-nossl") {
		t.Fatal("a certificate that never came expires and leaves the provider")
	}
	if _, ok := r.hostnames.rows["fresh"]; !ok {
		t.Fatal("a domain pending for an hour keeps waiting")
	}
	if _, ok := r.hostnames.rows["back"]; !ok {
		t.Fatal("a domain that was live once is not expired")
	}
}

func TestAProviderFailureKeepsTheProofAndAManualCheckRetries(t *testing.T) {
	r := newRig(t)
	rec := addDomain(t, r, "www.cliente.com.br")
	r.dns.prove(rec)
	r.cloudflare.err = errors.New("cloudflare down")
	if _, err := r.svc.VerifyHostname(context.Background(), actor, projectID, rec.ID); !errors.Is(err, ErrDomainProvider) {
		t.Fatalf("got %v", err)
	}
	got := r.hostnames.rows[rec.ID]
	if got.CloudflareRef != "" || got.Verification.OwnershipVerifiedAt == nil {
		t.Fatalf("the proof stays and nothing is half created, got %+v", got)
	}
	r.cloudflare.err = nil
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.hostnames.rows[rec.ID].Status != sitepublish.HostnamePendingSSL {
		t.Fatalf("the sweep retries, got %+v", r.hostnames.rows[rec.ID])
	}
}

type refusingRefUpdate struct{ *memHostnames }

func (f refusingRefUpdate) Update(_ context.Context, rec sitepublish.HostnameRecord) error {
	if rec.CloudflareRef != "" {
		return errors.New("database down")
	}
	return f.memHostnames.Update(context.Background(), rec)
}

func TestACustomDomainNeverLeaksItsProviderRecord(t *testing.T) {
	r := newRig(t)
	rec := addDomain(t, r, "www.cliente.com.br")
	r.dns.prove(rec)
	r.svc.Hostnames = refusingRefUpdate{r.hostnames}
	if _, err := r.svc.VerifyHostname(context.Background(), actor, projectID, rec.ID); err == nil {
		t.Fatal("the ref was not kept")
	}
	if !slices.Contains(r.cloudflare.deleted, "cf-www.cliente.com.br") {
		t.Fatalf("deleted %v", r.cloudflare.deleted)
	}
}

func TestFailedRemovalStaysRemovingForTheSweep(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	rec := liveDomain(t, r, "www.cliente.com.br")
	r.cloudflare.err = errors.New("down")
	got, err := r.svc.RemoveHostname(context.Background(), actor, projectID, rec.ID)
	if err != nil || got.Status != sitepublish.HostnameRemoving {
		t.Fatalf("got %+v %v", got, err)
	}
	r.cloudflare.err = nil
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.hostnames.rows[rec.ID]; ok || !slices.Contains(r.cloudflare.deleted, rec.CloudflareRef) {
		t.Fatalf("the sweep finishes the removal, deleted %v", r.cloudflare.deleted)
	}
}

func TestSSLFailuresAlertAndAFailedDomainLeavesTheProvider(t *testing.T) {
	r := newRig(t)
	dep := publish(t, r)
	rec := addDomain(t, r, "www.cliente.com.br")
	r.dns.prove(rec)
	rec, _ = r.svc.VerifyHostname(context.Background(), actor, projectID, rec.ID)
	state := r.cloudflare.states[rec.CloudflareRef]
	state.Phase, state.SSLStatus, state.SSLFailing, state.Errors = sitepublish.HostnamePendingSSL, "validation_timed_out", true, []string{"CAA blocks"}
	r.cloudflare.states[rec.CloudflareRef] = state
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := r.hostnames.rows[rec.ID]; got.Status != sitepublish.HostnamePendingSSL || got.Verification.FailingSince == nil {
		t.Fatalf("got %+v", got)
	}
	if last := r.alerts.published[len(r.alerts.published)-1]; last.EpisodeKey != "site_domain_ssl:"+rec.ID || !last.Email {
		t.Fatalf("alerts = %+v", r.alerts.published)
	}
	state.Phase, state.SSLFailing, state.Errors = sitepublish.HostnameActive, false, nil
	r.cloudflare.states[rec.CloudflareRef] = state
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.hosting.serving(t, "www.cliente.com.br").DeploymentID != dep.ID {
		t.Fatal("an active certificate puts the domain live")
	}
	state.Phase = sitepublish.HostnameFailed
	r.cloudflare.states[rec.CloudflareRef] = state
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := r.hostnames.rows[rec.ID]
	if got.Status != sitepublish.HostnameFailed || got.CloudflareRef != "" {
		t.Fatalf("a failed domain goes offline and off the provider, got %+v", got)
	}
	if _, served := r.hosting.hosts["www.cliente.com.br"]; served {
		t.Fatal("a failed domain serves nothing")
	}
	again, err := r.svc.VerifyHostname(context.Background(), actor, projectID, rec.ID)
	if err != nil || again.Status != sitepublish.HostnamePendingSSL {
		t.Fatalf("checking again recreates it, got %+v %v", again, err)
	}
}

func TestSweepKeepsGoingWhenTheProviderIsDown(t *testing.T) {
	r := newRig(t)
	rec := addDomain(t, r, "www.cliente.com.br")
	r.dns.prove(rec)
	rec, _ = r.svc.VerifyHostname(context.Background(), actor, projectID, rec.ID)
	r.cloudflare.getErr = errors.New("down")
	if err := r.svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.hostnames.rows[rec.ID].Status != sitepublish.HostnamePendingSSL {
		t.Fatal("an unreadable provider changes nothing")
	}
}

func TestVerifyIsScopedToTheWorkspaceAndPage(t *testing.T) {
	r := newRig(t)
	rec := addDomain(t, r, "www.cliente.com.br")
	if _, err := r.svc.VerifyHostname(context.Background(), Actor{UserID: "u", WorkspaceID: "ws-other"}, projectID, rec.ID); !errors.Is(err, sitepublish.ErrHostnameNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := r.svc.VerifyHostname(context.Background(), actor, "other-page", rec.ID); !errors.Is(err, sitepublish.ErrHostnameNotFound) {
		t.Fatalf("got %v", err)
	}
	r.access.denied["site_domains:update"] = true
	if _, err := r.svc.VerifyHostname(context.Background(), actor, projectID, rec.ID); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("got %v", err)
	}
}

func TestDomainSlotsCountEveryConnectedDomain(t *testing.T) {
	r := newRig(t)
	r.build(t, 3)
	addDomain(t, r, "a.cliente.com.br")
	addDomain(t, r, "b.cliente.com.br")
	slots, err := r.svc.DomainSlots(context.Background(), actor)
	if err != nil || slots.Used != 2 || slots.Allowed != 3 || slots.Offline != "" {
		t.Fatalf("got %+v %v", slots, err)
	}
	if used, err := r.svc.UnitsInUse(wsID); err != nil || used != 2 {
		t.Fatalf("units in use = %d %v", used, err)
	}
	r.subs.err = workspace_plan.ErrSubscriptionNotActive
	if slots, _ := r.svc.DomainSlots(context.Background(), actor); slots.Allowed != 0 || slots.Offline != sitepublish.OfflineBilling {
		t.Fatalf("an unpaid workspace has no usable slot, got %+v", slots)
	}
}
