package sitepublish

import (
	"errors"
	"fmt"
	"slices"
)

var (
	ErrDeploymentTransition = errors.New("sitepublish: the deployment cannot move to that status")
	ErrHostnameTransition   = errors.New("sitepublish: the hostname cannot move to that status")
	ErrHostnameNotVerified  = errors.New("sitepublish: the hostname dns is not verified yet")
	ErrSSLPending           = errors.New("sitepublish: the hostname certificate is still pending")
	ErrHostnameUnavailable  = errors.New("sitepublish: the hostname is not serving")
)

type DeploymentStatus string

const (
	DeploymentBuilding   DeploymentStatus = "building"
	DeploymentReady      DeploymentStatus = "ready"
	DeploymentFailed     DeploymentStatus = "failed"
	DeploymentActive     DeploymentStatus = "active"
	DeploymentSuperseded DeploymentStatus = "superseded"
)

var deploymentMoves = map[DeploymentStatus][]DeploymentStatus{
	DeploymentBuilding:   {DeploymentReady, DeploymentFailed},
	DeploymentReady:      {DeploymentActive},
	DeploymentActive:     {DeploymentSuperseded},
	DeploymentSuperseded: {DeploymentActive},
	DeploymentFailed:     {},
}

func DeploymentStatuses() []DeploymentStatus {
	return []DeploymentStatus{DeploymentBuilding, DeploymentReady, DeploymentFailed, DeploymentActive, DeploymentSuperseded}
}

func (s DeploymentStatus) Valid() bool {
	_, ok := deploymentMoves[s]
	return ok
}

func CheckDeploymentTransition(from, to DeploymentStatus) error {
	if !from.Valid() || !to.Valid() || !slices.Contains(deploymentMoves[from], to) {
		return fmt.Errorf("%w: %s to %s", ErrDeploymentTransition, from, to)
	}
	return nil
}

type Deployment struct {
	ID         string
	ProjectID  string
	RevisionID string
	Status     DeploymentStatus
}

func newDeployment(id, projectID, revisionID string) Deployment {
	return Deployment{ID: id, ProjectID: projectID, RevisionID: revisionID, Status: DeploymentBuilding}
}

func (d *Deployment) Transition(to DeploymentStatus) error {
	if err := CheckDeploymentTransition(d.Status, to); err != nil {
		return err
	}
	d.Status = to
	return nil
}

func (d Deployment) Activatable() bool {
	return slices.Contains(deploymentMoves[d.Status], DeploymentActive)
}

type HostnameKind string

const (
	HostnamePlatform HostnameKind = "platform"
	HostnameCustom   HostnameKind = "custom"
)

func (k HostnameKind) Valid() bool {
	return k == HostnamePlatform || k == HostnameCustom
}

type HostnameStatus string

const (
	HostnamePendingDNS HostnameStatus = "pending_dns"
	HostnamePendingSSL HostnameStatus = "pending_ssl"
	HostnameActive     HostnameStatus = "active"
	HostnameRemoving   HostnameStatus = "removing"
	HostnameFailed     HostnameStatus = "failed"
	HostnameSuspended  HostnameStatus = "suspended"
)

var hostnameMoves = map[HostnameKind]map[HostnameStatus][]HostnameStatus{
	HostnameCustom: {
		HostnamePendingDNS: {HostnamePendingSSL, HostnameFailed, HostnameRemoving, HostnameSuspended},
		HostnamePendingSSL: {HostnameActive, HostnameFailed, HostnameRemoving, HostnameSuspended},
		HostnameActive:     {HostnameFailed, HostnameRemoving, HostnameSuspended},
		HostnameFailed:     {HostnamePendingDNS, HostnamePendingSSL, HostnameRemoving, HostnameSuspended},
		HostnameSuspended:  {HostnamePendingDNS, HostnamePendingSSL, HostnameRemoving},
		HostnameRemoving:   {},
	},
	HostnamePlatform: {
		HostnameActive:   {HostnameRemoving},
		HostnameRemoving: {},
	},
}

func HostnameStatuses() []HostnameStatus {
	return []HostnameStatus{HostnamePendingDNS, HostnamePendingSSL, HostnameActive, HostnameRemoving, HostnameFailed, HostnameSuspended}
}

func (s HostnameStatus) Valid() bool {
	_, ok := hostnameMoves[HostnameCustom][s]
	return ok
}

var providerless = []HostnameStatus{HostnamePendingDNS, HostnameFailed, HostnameSuspended}

func ProviderlessStatuses() []HostnameStatus {
	return slices.Clone(providerless)
}

func (s HostnameStatus) HoldsNoProviderRecord() bool {
	return slices.Contains(providerless, s)
}

func InitialHostnameStatus(kind HostnameKind) HostnameStatus {
	if kind == HostnamePlatform {
		return HostnameActive
	}
	return HostnamePendingDNS
}

func CheckHostnameTransition(kind HostnameKind, from, to HostnameStatus) error {
	moves, ok := hostnameMoves[kind]
	if !ok || !slices.Contains(moves[from], to) {
		return fmt.Errorf("%w: %s %s to %s", ErrHostnameTransition, kind, from, to)
	}
	return nil
}

func (s HostnameStatus) serving() error {
	switch s {
	case HostnameActive:
		return nil
	case HostnamePendingDNS:
		return ErrHostnameNotVerified
	case HostnamePendingSSL:
		return ErrSSLPending
	}
	return ErrHostnameUnavailable
}

type Hostname struct {
	ID        string
	ProjectID string
	Hostname  string
	Kind      HostnameKind
	Status    HostnameStatus
}

func (h *Hostname) Transition(to HostnameStatus) error {
	if err := CheckHostnameTransition(h.Kind, h.Status, to); err != nil {
		return err
	}
	h.Status = to
	return nil
}

var errorCodes = []struct {
	err  error
	code string
}{
	{ErrDeploymentTransition, "deployment_transition_invalid"},
	{ErrHostnameTransition, "hostname_transition_invalid"},
	{ErrHostnameInvalid, "hostname_invalid"},
	{ErrHostnameForbidden, "hostname_forbidden"},
	{ErrHostnameNotVerified, "hostname_not_verified"},
	{ErrSSLPending, "ssl_pending"},
	{ErrHostnameUnavailable, "hostname_unavailable"},
	{ErrSlugReserved, "slug_reserved"},
	{ErrSlugInvalid, "slug_invalid"},
	{ErrServingInvalid, "serving_invalid"},
}

func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	for _, known := range errorCodes {
		if errors.Is(err, known.err) {
			return known.code
		}
	}
	return ""
}
