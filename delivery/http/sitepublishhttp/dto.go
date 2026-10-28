package sitepublishhttp

import (
	"time"

	"vozko/domain/dnsrecord"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

type DeploymentDTO struct {
	ID          string       `json:"id"`
	Status      string       `json:"status" enums:"building,ready,failed,active,superseded"`
	RevisionID  string       `json:"revisionId"`
	Address     string       `json:"address"`
	FileCount   int          `json:"fileCount"`
	Checks      []site.Check `json:"checks"`
	Gaps        []site.Issue `json:"gaps"`
	PublishedBy string       `json:"publishedBy,omitempty"`
	PublishedAt *time.Time   `json:"publishedAt,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
}

type PublishResponse struct {
	Deployment            DeploymentDTO `json:"deployment"`
	Held                  bool          `json:"held"`
	Hostnames             []string      `json:"hostnames"`
	ProjectVersion        int64         `json:"projectVersion" example:"14"`
	RevisionID            string        `json:"revisionId" example:"0b9c1f2e-4d5a-4c6b-8e7f-1a2b3c4d5e6f"`
	HasUnpublishedChanges bool          `json:"hasUnpublishedChanges"`
}

type DNSRecordDTO struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Host   string `json:"host" example:"www"`
	Value  string `json:"value"`
	Status string `json:"status,omitempty" enums:"verified,pending,unknown"`
}

type VerificationDTO struct {
	CNAMETarget  string         `json:"cnameTarget,omitempty"`
	Zone         string         `json:"zone,omitempty" example:"clinicasorriso.com.br"`
	CNAME        *DNSRecordDTO  `json:"cname,omitempty"`
	Ownership    *DNSRecordDTO  `json:"ownership,omitempty"`
	Records      []DNSRecordDTO `json:"records"`
	SSLStatus    string         `json:"sslStatus,omitempty"`
	Errors       []string       `json:"errors"`
	FailingSince *time.Time     `json:"failingSince,omitempty"`
	Offline      string         `json:"offline,omitempty" enums:"billing,slots"`
	PendingUntil *time.Time     `json:"pendingUntil,omitempty"`
}

type DomainSlotsDTO struct {
	Used    int    `json:"used" example:"2"`
	Allowed int    `json:"allowed" example:"3"`
	Offline string `json:"offline,omitempty" enums:"billing"`
}

type HostnameDTO struct {
	ID           string          `json:"id"`
	Hostname     string          `json:"hostname"`
	Kind         string          `json:"kind" enums:"platform,custom"`
	Status       string          `json:"status" enums:"pending_dns,pending_ssl,active,removing,failed,suspended"`
	Verification VerificationDTO `json:"verification"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

type AddHostnameRequest struct {
	Kind     string `json:"kind" enums:"platform,custom"`
	Slug     string `json:"slug,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

type AdminReasonRequest struct {
	Reason string `json:"reason"`
}

type AbuseCaseDTO struct {
	ID            string     `json:"id"`
	WorkspaceID   string     `json:"workspaceId"`
	ProjectID     string     `json:"projectId"`
	ProjectName   string     `json:"projectName,omitempty"`
	WorkspaceName string     `json:"workspaceName,omitempty"`
	DeploymentID  string     `json:"deploymentId,omitempty"`
	Source        string     `json:"source"`
	Status        string     `json:"status"`
	Reason        string     `json:"reason"`
	DecidedBy     string     `json:"decidedBy,omitempty"`
	DecidedAt     *time.Time `json:"decidedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

func deploymentDTO(rec sitepublish.DeploymentRecord, addresses sitepublish.Addresses) DeploymentDTO {
	checks, gaps := rec.Checks, rec.Gaps
	if checks == nil {
		checks = []site.Check{}
	}
	if gaps == nil {
		gaps = []site.Issue{}
	}
	address := ""
	if rec.Status == sitepublish.DeploymentActive {
		address = addresses.DeploymentURL(rec.ID)
	}
	return DeploymentDTO{
		ID: rec.ID, Status: string(rec.Status), RevisionID: rec.RevisionID, Address: address, FileCount: len(rec.Files),
		Checks: checks, Gaps: gaps, PublishedBy: rec.PublishedBy, PublishedAt: rec.PublishedAt, CreatedAt: rec.CreatedAt,
	}
}

func recordDTO(r sitepublish.DNSRecord, zone, status string) DNSRecordDTO {
	return DNSRecordDTO{Type: r.Type, Name: r.Name, Host: dnsrecord.RelativeHost(r.Name, zone), Value: r.Value, Status: status}
}

func hostnameDTO(rec sitepublish.HostnameRecord) HostnameDTO {
	zone := sitepublish.DNSZone(rec.Hostname)
	records := make([]DNSRecordDTO, 0, len(rec.Verification.Records))
	for _, r := range rec.Verification.Records {
		records = append(records, recordDTO(r, zone, ""))
	}
	errs := rec.Verification.Errors
	if errs == nil {
		errs = []string{}
	}
	verification := VerificationDTO{
		CNAMETarget: rec.Verification.CNAMETarget, Records: records, SSLStatus: rec.Verification.SSLStatus, Errors: errs,
		FailingSince: rec.Verification.FailingSince, Offline: string(rec.Verification.Offline),
	}
	if rec.Kind == sitepublish.HostnameCustom {
		verification.Zone = zone
		if rec.Verification.CNAMETarget != "" {
			cname := recordDTO(sitepublish.CNAMERecord(rec.Hostname, rec.Verification.CNAMETarget), zone, "")
			verification.CNAME = &cname
		}
	}
	if rec.Kind == sitepublish.HostnameCustom && rec.Verification.OwnershipToken != "" {
		status := string(rec.Verification.OwnershipStatus)
		if rec.Verification.OwnershipVerifiedAt != nil {
			status = string(dnsrecord.Verified)
		}
		own := recordDTO(sitepublish.OwnershipRecord(rec.Hostname, rec.Verification.OwnershipToken), zone, status)
		verification.Ownership = &own
	}
	if rec.Status == sitepublish.HostnamePendingDNS && rec.Verification.PendingSince != nil {
		until := rec.Verification.PendingSince.Add(sitepublish_usecase.UnverifiedHostnameTTL)
		verification.PendingUntil = &until
	}
	return HostnameDTO{
		ID: rec.ID, Hostname: rec.Hostname, Kind: string(rec.Kind), Status: string(rec.Status),
		Verification: verification, CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
	}
}

func abuseCaseDTO(c sitepublish.AbuseCase) AbuseCaseDTO {
	return AbuseCaseDTO{
		ID: c.ID, WorkspaceID: c.WorkspaceID, ProjectID: c.ProjectID, DeploymentID: c.DeploymentID, Source: string(c.Source),
		Status: string(c.Status), Reason: c.Reason, DecidedBy: c.DecidedBy, DecidedAt: c.DecidedAt, CreatedAt: c.CreatedAt,
	}
}
