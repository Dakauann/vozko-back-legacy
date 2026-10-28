package sitepublish_usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"vozko/domain/notification"
	"vozko/domain/sitepublish"
	"vozko/domain/workspace"
)

const UnverifiedHostnameTTL = 72 * time.Hour

const (
	sweepBatch     = 200
	sourceSiteSSL  = "site_domain"
	domainsPathFmt = "/dashboard/pages/%s?tab=domains"
)

func sslEpisodeKey(hostnameID string) string {
	return "site_domain_ssl:" + hostnameID
}

func failedEpisodeKey(hostnameID string) string {
	return "site_domain_failed:" + hostnameID
}

func expiredEpisodeKey(hostnameID string) string {
	return "site_domain_expired:" + hostnameID
}

func quotaEpisodeKey(hostnameID string) string {
	return "site_domain_quota:" + hostnameID
}

func (s *Service) Execute(ctx context.Context) error {
	return s.Sweep(ctx)
}

func (s *Service) Sweep(ctx context.Context) error {
	var failures []error
	for _, statuses := range [][]sitepublish.HostnameStatus{
		{sitepublish.HostnamePendingDNS},
		{sitepublish.HostnamePendingSSL, sitepublish.HostnameActive},
	} {
		listed, err := s.Hostnames.ListByStatus(ctx, statuses, sitepublish.HostnameCustom, sweepBatch)
		if err != nil {
			return err
		}
		failures = append(failures, s.stepEach(ctx, listed, s.advance)...)
	}
	walks := []struct {
		page sitepublish.HostnamePage
		step domainStep
	}{
		{sitepublish.HostnamePage{Statuses: sitepublish.ProviderlessStatuses(), Kind: sitepublish.HostnameCustom, HoldingProviderRecord: true}, s.retryProviderDeletion},
		{sitepublish.HostnamePage{Statuses: []sitepublish.HostnameStatus{sitepublish.HostnameRemoving}, Kind: sitepublish.HostnameCustom}, s.retryRemoval},
		{sitepublish.HostnamePage{Statuses: []sitepublish.HostnameStatus{sitepublish.HostnameRemoving}, Kind: sitepublish.HostnamePlatform}, s.retryRemoval},
	}
	for _, walk := range walks {
		walked, err := s.walk(ctx, walk.page, walk.step)
		if err != nil {
			return err
		}
		failures = append(failures, walked...)
	}
	failures = append(failures, s.reconcile(ctx)...)
	for _, f := range failures {
		log.Printf("[sitepublish] hostname sweep: %v", f)
	}
	return nil
}

func (s *Service) walk(ctx context.Context, page sitepublish.HostnamePage, step domainStep) ([]error, error) {
	var failures []error
	page.Limit = sweepBatch
	for {
		listed, err := s.Hostnames.ListPage(ctx, page)
		if err != nil {
			return failures, err
		}
		failures = append(failures, s.stepEach(ctx, listed, step)...)
		if len(listed) < page.Limit {
			return failures, nil
		}
		page.AfterID = listed[len(listed)-1].ID
	}
}

func (s *Service) stepEach(ctx context.Context, listed []sitepublish.HostnameRecord, step domainStep) []error {
	var failures []error
	for _, rec := range listed {
		if _, err := s.lockedDomain(ctx, rec, step); err != nil && !skippable(err) {
			failures = append(failures, fmt.Errorf("%s: %w", rec.Hostname, err))
		}
	}
	return failures
}

func (s *Service) advance(ctx context.Context, rec sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
	if s.unverifiedTooLong(rec) {
		return s.expire(ctx, rec)
	}
	switch rec.Status {
	case sitepublish.HostnamePendingDNS:
		return s.checkOwnership(ctx, rec)
	case sitepublish.HostnamePendingSSL, sitepublish.HostnameActive:
		return s.poll(ctx, rec)
	}
	return rec, nil
}

func (s *Service) retryRemoval(ctx context.Context, rec sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
	if rec.Status != sitepublish.HostnameRemoving {
		return rec, nil
	}
	return rec, s.finishRemoval(ctx, rec)
}

func (s *Service) unverifiedTooLong(rec sitepublish.HostnameRecord) bool {
	pending := rec.Status == sitepublish.HostnamePendingDNS || rec.Status == sitepublish.HostnamePendingSSL
	if !pending || (rec.Status == sitepublish.HostnamePendingSSL && rec.Verification.ActivatedAt != nil) {
		return false
	}
	since := rec.CreatedAt
	if rec.Verification.PendingSince != nil {
		since = *rec.Verification.PendingSince
	}
	return !since.IsZero() && s.Now().Sub(since) > UnverifiedHostnameTTL
}

func (s *Service) retryProviderDeletion(ctx context.Context, rec sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
	if !rec.Status.HoldsNoProviderRecord() {
		return rec, nil
	}
	return rec, s.dropProviderRecord(ctx, &rec)
}

func (s *Service) expire(ctx context.Context, rec sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
	if err := s.startRemoval(ctx, &rec); err != nil {
		return rec, err
	}
	if err := s.finishRemoval(ctx, rec); err != nil {
		return rec, err
	}
	return rec, s.alert(ctx, rec, expiredEpisodeKey(rec.ID), "O domínio "+rec.Hostname+" não foi verificado",
		"O domínio "+rec.Hostname+" ficou mais de 72 horas sem a verificação do DNS e foi removido. Confira os registros no seu provedor e adicione o domínio de novo quando estiver pronto.")
}

func (s *Service) poll(ctx context.Context, rec sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
	if rec.CloudflareRef == "" {
		return rec, s.markFailed(ctx, rec, []string{"o domínio não foi registrado no provedor"})
	}
	state, err := s.CustomHostnames.Get(ctx, rec.CloudflareRef)
	if errors.Is(err, sitepublish.ErrHostnameNotFound) {
		rec.CloudflareRef = ""
		return rec, s.markFailed(ctx, rec, []string{"o domínio não existe mais no provedor"})
	}
	if err != nil {
		return rec, err
	}
	return s.applyState(ctx, rec, state)
}

func (s *Service) applyState(ctx context.Context, rec sitepublish.HostnameRecord, state sitepublish.CustomHostnameState) (sitepublish.HostnameRecord, error) {
	rec.Verification.Records = state.Records
	rec.Verification.SSLStatus = state.SSLStatus
	rec.Verification.Errors = state.Errors
	if rec.Verification.CNAMETarget == "" {
		rec.Verification.CNAMETarget = s.cfg.CNAMETarget
	}
	switch state.Phase {
	case sitepublish.HostnameFailed:
		return rec, s.markFailed(ctx, rec, state.Errors)
	case sitepublish.HostnameActive:
		return s.activate(ctx, rec)
	case sitepublish.HostnamePendingSSL, sitepublish.HostnamePendingDNS:
		if rec.Status == sitepublish.HostnamePendingDNS || rec.Status == sitepublish.HostnameFailed {
			if err := s.moveTo(&rec, sitepublish.HostnamePendingSSL); err != nil {
				return rec, err
			}
		}
	}
	if state.SSLFailing {
		if rec.Verification.FailingSince == nil {
			at := s.Now()
			rec.Verification.FailingSince = &at
		}
	} else {
		rec.Verification.FailingSince = nil
	}
	rec.UpdatedAt = s.Now()
	if err := s.Hostnames.Update(ctx, rec); err != nil {
		return rec, err
	}
	if state.SSLFailing && rec.Status != sitepublish.HostnameActive {
		return rec, s.alert(ctx, rec, sslEpisodeKey(rec.ID), "O certificado do domínio "+rec.Hostname+" não foi emitido",
			"Não conseguimos emitir o certificado de segurança de "+rec.Hostname+". Confira os registros DNS indicados na página de domínios. Enquanto isso, o domínio não exibe a página."+detail(state.Errors))
	}
	return rec, s.Alerts.Resolve(ctx, rec.WorkspaceID, sslEpisodeKey(rec.ID))
}

func detail(errs []string) string {
	if len(errs) == 0 {
		return ""
	}
	return " Motivo informado: " + strings.Join(errs, "; ") + "."
}

func (s *Service) moveTo(rec *sitepublish.HostnameRecord, to sitepublish.HostnameStatus) error {
	if rec.Status == to {
		return nil
	}
	if rec.Status == sitepublish.HostnamePendingDNS && to == sitepublish.HostnameActive {
		if err := rec.Transition(sitepublish.HostnamePendingSSL); err != nil {
			return err
		}
	}
	if rec.Status == sitepublish.HostnameFailed && to != sitepublish.HostnamePendingDNS && to != sitepublish.HostnameRemoving {
		if err := rec.Transition(sitepublish.HostnamePendingDNS); err != nil {
			return err
		}
		return s.moveTo(rec, to)
	}
	return rec.Transition(to)
}

func (s *Service) activate(ctx context.Context, rec sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
	wasActive := rec.Status == sitepublish.HostnameActive
	if err := s.moveTo(&rec, sitepublish.HostnameActive); err != nil {
		return rec, err
	}
	if !wasActive {
		if err := s.serveHostname(ctx, rec); err != nil {
			return rec, err
		}
	}
	rec.Verification.FailingSince = nil
	if rec.Verification.ActivatedAt == nil {
		at := s.Now()
		rec.Verification.ActivatedAt = &at
	}
	rec.UpdatedAt = s.Now()
	if err := s.Hostnames.Update(ctx, rec); err != nil {
		return rec, err
	}
	return rec, s.Alerts.Resolve(ctx, rec.WorkspaceID, sslEpisodeKey(rec.ID), failedEpisodeKey(rec.ID))
}

func (s *Service) markFailed(ctx context.Context, rec sitepublish.HostnameRecord, reasons []string) error {
	if rec.Status == sitepublish.HostnameFailed {
		return nil
	}
	served := rec.Status == sitepublish.HostnameActive
	if err := s.moveTo(&rec, sitepublish.HostnameFailed); err != nil {
		return err
	}
	if served {
		if err := s.Hosting.DeleteHost(ctx, rec.Hostname); err != nil {
			return fmt.Errorf("%w: %w", ErrHostingUnavailable, err)
		}
	}
	rec.Verification.Errors = reasons
	rec.UpdatedAt = s.Now()
	if err := s.Hostnames.Update(ctx, rec); err != nil {
		return err
	}
	if err := s.dropProviderRecord(ctx, &rec); err != nil {
		log.Printf("[sitepublish] hostname %s failed and its provider record waits for the sweep: %v", rec.Hostname, err)
	}
	return s.alert(ctx, rec, failedEpisodeKey(rec.ID), "O domínio "+rec.Hostname+" parou de funcionar",
		"O domínio "+rec.Hostname+" não aponta mais para a página e foi tirado do ar. Confira o DNS no seu provedor e use Verificar de novo na página de domínios quando estiver pronto."+detail(reasons))
}

func (s *Service) alert(ctx context.Context, rec sitepublish.HostnameRecord, key, title, body string) error {
	_, err := s.Alerts.Publish(ctx, notification.Draft{
		WorkspaceID: rec.WorkspaceID,
		Category:    notification.CategorySystem,
		Severity:    notification.SeverityWarning,
		Title:       notification.ClipTitle(title),
		Body:        body,
		ActionURL:   fmt.Sprintf(domainsPathFmt, rec.ProjectID),
		ActionLabel: "Ver domínios",
		Audience:    notification.Holding(ResourceSiteDomains, workspace.ActionUpdate),
		Email:       true,
		EpisodeKey:  key,
		Source:      sourceSiteSSL,
	})
	return err
}
