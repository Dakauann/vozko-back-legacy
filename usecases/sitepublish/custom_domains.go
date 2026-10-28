package sitepublish_usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"vozko/domain/dnsrecord"
	"vozko/domain/sitepublish"
	"vozko/domain/workspace"
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

const (
	domainAddKeyPrefix = "site:domains:add:"
	domainLockPrefix   = "site:domain:lock:"
	domainLockTTL      = 2 * time.Minute

	allowanceFailuresPrefix    = "site:domains:allowance_failures:"
	allowanceFailuresBeforeCut = 6
	allowanceFailureWindow     = 24 * time.Hour
)

type DomainSlots struct {
	Used    int
	Allowed int
	Offline sitepublish.OfflineReason
}

func (s *Service) DomainSlots(ctx context.Context, a Actor) (DomainSlots, error) {
	if err := s.allow(a, ResourceSiteDomains, workspace.ActionRead); err != nil {
		return DomainSlots{}, err
	}
	list, err := s.Hostnames.ListCustomByWorkspace(ctx, a.WorkspaceID)
	if err != nil {
		return DomainSlots{}, err
	}
	allowed, reason, err := s.domainAllowance(ctx, a.WorkspaceID)
	if err != nil {
		return DomainSlots{}, err
	}
	out := DomainSlots{Used: sitepublish.SlotsInUse(list), Allowed: allowed}
	if reason == sitepublish.OfflineBilling {
		out.Offline = reason
	}
	return out, nil
}

func (s *Service) UnitsInUse(workspaceID string) (int, error) {
	list, err := s.Hostnames.ListCustomByWorkspace(context.Background(), workspaceID)
	if err != nil {
		return 0, err
	}
	return sitepublish.SlotsInUse(list), nil
}

func (s *Service) domainAllowance(ctx context.Context, workspaceID string) (int, sitepublish.OfflineReason, error) {
	cut, err := s.billingCut(workspaceID)
	if err != nil {
		return refusedAllowance(err)
	}
	if cut {
		return 0, sitepublish.OfflineBilling, nil
	}
	reversed, err := s.Reversals.HasOpenReversal(ctx, workspaceID)
	if err != nil {
		return 0, "", fmt.Errorf("read payment reversals: %w", err)
	}
	if reversed {
		return 0, sitepublish.OfflineBilling, nil
	}
	allowed, err := s.Entitlements.Resolve(workspaceID, workspace_addon.EntitlementSiteDomains)
	if err != nil {
		return refusedAllowance(err)
	}
	return max(allowed, 0), sitepublish.OfflineSlots, nil
}

func refusedAllowance(err error) (int, sitepublish.OfflineReason, error) {
	switch {
	case errors.Is(err, workspace_plan.ErrSubscriptionNotActive), errors.Is(err, workspace_plan.ErrSubscriptionNotCurrent),
		errors.Is(err, workspace_plan.ErrSubscriptionNotFound), errors.Is(err, workspace_plan.ErrPlanNotFound):
		return 0, sitepublish.OfflineBilling, nil
	case errors.Is(err, workspace_addon.ErrInvalidEntitlementKind):
		return 0, sitepublish.OfflineSlots, nil
	}
	return 0, "", err
}

func allowanceFailuresKey(workspaceID string) string {
	return allowanceFailuresPrefix + workspaceID
}

func (s *Service) enforcedAllowance(ctx context.Context, workspaceID string) (int, sitepublish.OfflineReason, error) {
	allowed, reason, err := s.domainAllowance(ctx, workspaceID)
	if err == nil {
		if err := s.Counters.Del(allowanceFailuresKey(workspaceID)); err != nil {
			log.Printf("[sitepublish] workspace %s: the allowance failure count stays until it expires: %v", workspaceID, err)
		}
		return allowed, reason, nil
	}
	failures, countErr := s.Counters.IncrWithTTL(allowanceFailuresKey(workspaceID), allowanceFailureWindow)
	if countErr == nil && failures < allowanceFailuresBeforeCut {
		return 0, "", err
	}
	log.Printf("[sitepublish] workspace %s: the domain allowance stays unreadable, its domains go offline: %v", workspaceID, errors.Join(err, countErr))
	return 0, sitepublish.OfflineBilling, nil
}

func (s *Service) claimDomainAdd(workspaceID string) (string, error) {
	for i := 0; i < sitepublish.MaxCustomDomainAddsPerDay; i++ {
		key := domainAddKeyPrefix + workspaceID + ":" + strconv.Itoa(i)
		acquired, err := s.Locks.SetNX(key, s.NewID(), sitepublish.CustomDomainAddWindow)
		if err != nil {
			return "", fmt.Errorf("%w: %w", ErrDomainAddsLimited, err)
		}
		if acquired {
			return key, nil
		}
	}
	return "", ErrDomainAddsLimited
}

func (s *Service) releaseDomainAdd(key string) {
	if err := s.Locks.Del(key); err != nil {
		log.Printf("[sitepublish] a refused domain keeps its add slot until it expires: %v", err)
	}
}

func (s *Service) addCustom(ctx context.Context, workspaceID, projectID, raw string) (sitepublish.HostnameRecord, error) {
	platform := sitepublish.PlatformDomains(s.cfg.Addresses.Domain, s.cfg.ApplicationDomains)
	host, err := sitepublish.ValidateCustomHostname(raw, platform)
	if err != nil {
		return sitepublish.HostnameRecord{}, err
	}
	allowed, _, err := s.domainAllowance(ctx, workspaceID)
	if err != nil {
		return sitepublish.HostnameRecord{}, err
	}
	claim, err := s.claimDomainAdd(workspaceID)
	if err != nil {
		return sitepublish.HostnameRecord{}, err
	}
	now := s.Now()
	rec := sitepublish.HostnameRecord{
		ID: s.NewID(), WorkspaceID: workspaceID, ProjectID: projectID, Hostname: host, Kind: sitepublish.HostnameCustom,
		Status: sitepublish.InitialHostnameStatus(sitepublish.HostnameCustom),
		Verification: sitepublish.Verification{
			CNAMETarget: s.cfg.CNAMETarget, OwnershipToken: strings.ReplaceAll(s.NewID(), "-", ""),
			OwnershipStatus: dnsrecord.Pending, PendingSince: &now,
		},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Hostnames.CreateCustom(ctx, rec, allowed); err != nil {
		s.releaseDomainAdd(claim)
		if errors.Is(err, sitepublish.ErrDomainQuota) {
			return sitepublish.HostnameRecord{}, fmt.Errorf("%w: %w", ErrSiteDomainsExhausted, err)
		}
		return sitepublish.HostnameRecord{}, err
	}
	return s.lockedDomain(ctx, rec, s.verify)
}

func (s *Service) VerifyHostname(ctx context.Context, a Actor, projectID, hostnameID string) (sitepublish.HostnameRecord, error) {
	if err := s.allow(a, ResourceSiteDomains, workspace.ActionUpdate); err != nil {
		return sitepublish.HostnameRecord{}, err
	}
	rec, err := s.Hostnames.Get(ctx, a.WorkspaceID, hostnameID)
	if err != nil {
		return sitepublish.HostnameRecord{}, err
	}
	if rec.ProjectID != projectID || rec.Kind != sitepublish.HostnameCustom {
		return sitepublish.HostnameRecord{}, sitepublish.ErrHostnameNotFound
	}
	if rec.Status != sitepublish.HostnamePendingDNS && rec.Status != sitepublish.HostnameFailed {
		return rec, nil
	}
	return s.lockedDomain(ctx, rec, s.verify)
}

func (s *Service) verify(ctx context.Context, rec sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
	switch rec.Status {
	case sitepublish.HostnamePendingDNS:
		return s.checkOwnership(ctx, rec)
	case sitepublish.HostnameFailed:
		return s.provision(ctx, rec)
	}
	return rec, nil
}

type domainStep func(context.Context, sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error)

func (s *Service) lockedDomain(ctx context.Context, rec sitepublish.HostnameRecord, fn domainStep) (sitepublish.HostnameRecord, error) {
	key := domainLockPrefix + rec.ID
	acquired, err := s.Locks.SetNX(key, s.NewID(), domainLockTTL)
	if err != nil {
		return rec, fmt.Errorf("sitepublish: the domain lock is unreadable: %w", err)
	}
	if !acquired {
		return rec, ErrDomainBusy
	}
	defer func() {
		if err := s.Locks.Del(key); err != nil {
			log.Printf("[sitepublish] hostname %s: the lock stays until it expires: %v", rec.Hostname, err)
		}
	}()
	current, err := s.Hostnames.Get(ctx, rec.WorkspaceID, rec.ID)
	if err != nil {
		return rec, err
	}
	return fn(ctx, current)
}

func skippable(err error) bool {
	return errors.Is(err, ErrDomainBusy) || errors.Is(err, sitepublish.ErrHostnameNotFound)
}

func (s *Service) checkOwnership(ctx context.Context, rec sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
	answers := dnsrecord.Lookup(ctx, s.DNS.LookupTXT, []string{sitepublish.OwnershipName(rec.Hostname)}, func(name string, err error) {
		log.Printf("[sitepublish] ownership lookup of %s failed: %v", name, err)
	})
	status := sitepublish.ObserveOwnership(answers, rec.Hostname, rec.Verification.OwnershipToken)
	rec.Verification.OwnershipStatus = status
	rec.UpdatedAt = s.Now()
	if status != dnsrecord.Verified {
		return rec, s.Hostnames.Update(ctx, rec)
	}
	at := s.Now()
	rec.Verification.OwnershipVerifiedAt = &at
	if err := s.Hostnames.Update(ctx, rec); err != nil {
		return rec, err
	}
	return s.provision(ctx, rec)
}

func (s *Service) entitledNow(ctx context.Context, rec sitepublish.HostnameRecord) (bool, sitepublish.OfflineReason, error) {
	allowed, reason, err := s.domainAllowance(ctx, rec.WorkspaceID)
	if err != nil {
		return false, "", err
	}
	list, err := s.Hostnames.ListCustomByWorkspace(ctx, rec.WorkspaceID)
	if err != nil {
		return false, "", err
	}
	return sitepublish.Entitled(list, allowed)[rec.ID], reason, nil
}

func (s *Service) provision(ctx context.Context, rec sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
	if rec.Verification.OwnershipVerifiedAt == nil {
		return rec, sitepublish.ErrHostnameNotVerified
	}
	entitled, reason, err := s.entitledNow(ctx, rec)
	if err != nil {
		return rec, err
	}
	if !entitled {
		return s.suspendDomain(ctx, rec, reason)
	}
	if rec.CloudflareRef != "" {
		if err := s.dropProviderRecord(ctx, &rec); err != nil {
			return rec, err
		}
	}
	state, err := s.CustomHostnames.Create(ctx, rec.Hostname)
	if err != nil {
		return rec, fmt.Errorf("%w: %w", ErrDomainProvider, err)
	}
	now := s.Now()
	rec.CloudflareRef = state.Ref
	rec.Verification.Offline, rec.Verification.PendingSince = "", &now
	if err := s.moveTo(&rec, sitepublish.HostnamePendingSSL); err != nil {
		s.forgetProviderRecord(ctx, rec.Hostname, state.Ref)
		return rec, err
	}
	rec.UpdatedAt = now
	if err := s.Hostnames.Update(ctx, rec); err != nil {
		s.forgetProviderRecord(ctx, rec.Hostname, state.Ref)
		return rec, err
	}
	return s.applyState(ctx, rec, state)
}

func (s *Service) forgetProviderRecord(ctx context.Context, host, ref string) {
	if err := s.CustomHostnames.Delete(ctx, ref); err != nil {
		log.Printf("[sitepublish] hostname %s: the provider record %s could not be deleted after its ref was lost: %v", host, ref, err)
	}
}

func (s *Service) deleteProviderRecord(ctx context.Context, ref string) error {
	if err := s.CustomHostnames.Delete(ctx, ref); err != nil && !errors.Is(err, sitepublish.ErrHostnameNotFound) {
		return fmt.Errorf("%w: %w", ErrDomainProvider, err)
	}
	return nil
}

func (s *Service) dropProviderRecord(ctx context.Context, rec *sitepublish.HostnameRecord) error {
	if rec.CloudflareRef == "" {
		return nil
	}
	if err := s.deleteProviderRecord(ctx, rec.CloudflareRef); err != nil {
		return err
	}
	rec.CloudflareRef = ""
	rec.UpdatedAt = s.Now()
	return s.Hostnames.Update(ctx, *rec)
}

func (s *Service) suspendDomain(ctx context.Context, rec sitepublish.HostnameRecord, reason sitepublish.OfflineReason) (sitepublish.HostnameRecord, error) {
	if err := s.Hosting.DeleteHost(ctx, rec.Hostname); err != nil {
		return rec, fmt.Errorf("%w: %w", ErrHostingUnavailable, err)
	}
	announce := rec.Status != sitepublish.HostnameSuspended || rec.Verification.Offline != reason
	if err := s.moveTo(&rec, sitepublish.HostnameSuspended); err != nil {
		return rec, err
	}
	rec.Verification.Offline = reason
	rec.Verification.FailingSince = nil
	rec.UpdatedAt = s.Now()
	if err := s.Hostnames.Update(ctx, rec); err != nil {
		return rec, err
	}
	if err := s.dropProviderRecord(ctx, &rec); err != nil {
		log.Printf("[sitepublish] hostname %s is offline and its provider record waits for the sweep: %v", rec.Hostname, err)
	}
	if !announce {
		return rec, nil
	}
	return rec, s.announceOffline(ctx, rec)
}

func (s *Service) announceOffline(ctx context.Context, rec sitepublish.HostnameRecord) error {
	if rec.Verification.Offline == sitepublish.OfflineBilling {
		return s.alert(ctx, rec, quotaEpisodeKey(rec.ID), "O domínio "+rec.Hostname+" saiu do ar",
			"O domínio "+rec.Hostname+" saiu do ar porque a assinatura do workspace está pendente. Ele volta sozinho assim que o pagamento for confirmado.")
	}
	return s.alert(ctx, rec, quotaEpisodeKey(rec.ID), "O domínio "+rec.Hostname+" saiu do ar",
		"O domínio "+rec.Hostname+" saiu do ar porque não há domínio disponível para ele e ele é o mais recente. Compre mais domínios ou remova outro domínio; ele volta sozinho quando houver vaga.")
}

func (s *Service) resumeDomain(ctx context.Context, rec sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
	if rec.Verification.OwnershipVerifiedAt == nil {
		now := s.Now()
		if err := s.moveTo(&rec, sitepublish.HostnamePendingDNS); err != nil {
			return rec, err
		}
		rec.Verification.Offline, rec.Verification.PendingSince = "", &now
		rec.UpdatedAt = now
		if err := s.Hostnames.Update(ctx, rec); err != nil {
			return rec, err
		}
		if err := s.Alerts.Resolve(ctx, rec.WorkspaceID, quotaEpisodeKey(rec.ID)); err != nil {
			return rec, err
		}
		return s.checkOwnership(ctx, rec)
	}
	out, err := s.provision(ctx, rec)
	if err != nil {
		return out, err
	}
	if out.Status == sitepublish.HostnameSuspended {
		return out, nil
	}
	return out, s.Alerts.Resolve(ctx, rec.WorkspaceID, quotaEpisodeKey(rec.ID))
}

func (s *Service) enforcement(rec sitepublish.HostnameRecord, entitled bool, reason sitepublish.OfflineReason) domainStep {
	switch {
	case rec.Kind != sitepublish.HostnameCustom || rec.Status == sitepublish.HostnameRemoving:
		return nil
	case !entitled && rec.Status == sitepublish.HostnamePendingDNS:
		return nil
	case !entitled && (rec.Status != sitepublish.HostnameSuspended || rec.Verification.Offline != reason):
		return func(ctx context.Context, rec sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
			return s.suspendDomain(ctx, rec, reason)
		}
	case entitled && rec.Status == sitepublish.HostnameSuspended:
		return s.resumeDomain
	}
	return nil
}

func (s *Service) enforceDomains(ctx context.Context, workspaceID string) error {
	allowed, reason, err := s.enforcedAllowance(ctx, workspaceID)
	if err != nil {
		return err
	}
	list, err := s.Hostnames.ListCustomByWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	entitled := sitepublish.Entitled(list, allowed)
	var failures []error
	for _, rec := range sitepublish.SlotHolders(list) {
		if s.enforcement(rec, entitled[rec.ID], reason) == nil {
			continue
		}
		_, err := s.lockedDomain(ctx, rec, func(ctx context.Context, current sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
			step := s.enforcement(current, entitled[current.ID], reason)
			if step == nil {
				return current, nil
			}
			return step(ctx, current)
		})
		if err != nil && !skippable(err) {
			failures = append(failures, fmt.Errorf("%s: %w", rec.Hostname, err))
		}
	}
	return errors.Join(failures...)
}

func (s *Service) OnPaymentReversed(ctx context.Context, workspaceID string) error {
	return s.enforceDomains(ctx, workspaceID)
}

func (s *Service) OnReversalReleased(ctx context.Context, workspaceID string) error {
	return s.enforceDomains(ctx, workspaceID)
}
