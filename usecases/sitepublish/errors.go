package sitepublish_usecase

import (
	"errors"
	"fmt"
	"strings"

	"vozko/domain/site"
)

var (
	ErrAccessDenied          = errors.New("sitepublish: the actor may not do this")
	ErrSubscriptionInactive  = errors.New("sitepublish: the workspace subscription is not active")
	ErrProjectNotFound       = errors.New("sitepublish: the page was not found")
	ErrBuildFailed           = errors.New("sitepublish: the page did not build")
	ErrPrerenderFailed       = errors.New("sitepublish: the page could not be prerendered")
	ErrPublishBlocked        = errors.New("sitepublish: the rendered page breaks a publishing rule")
	ErrGapsOpen              = errors.New("sitepublish: the page has open gaps")
	ErrFormKeyMissing        = errors.New("sitepublish: a form has no public key")
	ErrFormsUnavailable      = errors.New("sitepublish: the forms could not be activated")
	ErrMediaUnavailable      = errors.New("sitepublish: a library image could not be copied")
	ErrHostingUnavailable    = errors.New("sitepublish: the hosting store is unavailable")
	ErrUnderReview           = errors.New("sitepublish: the page is waiting for a review")
	ErrSiteSuspended         = errors.New("sitepublish: the page is suspended")
	ErrPlatformHostnameTaken = errors.New("sitepublish: the page already has a subdomain")
	ErrSiteDomainsExhausted  = errors.New("sitepublish: the plan has no custom domain left")
	ErrDomainProvider        = errors.New("sitepublish: the domain provider is unavailable")
	ErrDomainAddsLimited     = errors.New("sitepublish: too many custom domains were added today")
	ErrDomainBusy            = errors.New("sitepublish: the domain is being checked right now")
	ErrHostnameKindInvalid   = errors.New("sitepublish: the hostname kind is unknown")
	ErrReasonRequired        = errors.New("sitepublish: a reason is required")
	ErrWidgetUnknown         = errors.New("sitepublish: an embedded chat widget is not one of this workspace")
	ErrLiveScanFailed        = errors.New("sitepublish: the page could not be checked after its scripts ran")
	ErrPublishUnrecorded     = errors.New("sitepublish: the page went live but the publish was not recorded")
	ErrPublishInProgress     = errors.New("sitepublish: the page is already being published")
	ErrPublishStale          = errors.New("sitepublish: the page changed after the publish was proposed")
	ErrMediaRefused          = fmt.Errorf("%w: the file is not a public library file of this workspace", ErrMediaUnavailable)
)

type BuildFailure struct {
	Errors []BuildError
}

func (e *BuildFailure) Error() string {
	parts := make([]string, 0, len(e.Errors))
	for _, b := range e.Errors {
		parts = append(parts, b.Path+": "+b.Message)
	}
	return ErrBuildFailed.Error() + ": " + strings.Join(parts, "; ")
}

func (e *BuildFailure) Is(target error) bool {
	return target == ErrBuildFailed
}

type IssuesFailure struct {
	Kind   error
	Issues []site.Issue
}

func (e *IssuesFailure) Error() string {
	codes := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		codes = append(codes, issue.Code)
	}
	return e.Kind.Error() + ": " + strings.Join(codes, ", ")
}

func (e *IssuesFailure) Is(target error) bool {
	return target == e.Kind
}
