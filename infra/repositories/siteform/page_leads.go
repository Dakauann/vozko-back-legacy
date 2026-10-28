package siteform_repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	crmfiltersql "vozko/infra/repositories/crmfilter"
)

var ErrPageLeadsInvalid = errors.New("siteform: the leads of a page need a workspace, a page and a period")

type PageLeads struct {
	db *gorm.DB
}

func NewPageLeads(db *gorm.DB) *PageLeads {
	return &PageLeads{db: db}
}

var pageLeadsSQL = "SELECT DISTINCT page_leads.lead_id FROM (SELECT s.lead_id FROM site_submissions s WHERE s.workspace_id = ? AND s.project_id = ? AND s.lead_id IS NOT NULL AND s.created_at >= ? AND s.created_at < ?" +
	" UNION SELECT lead_entries.lead_id FROM conversation_page_origins o JOIN " + crmfiltersql.LeadEntriesSource() +
	" ON lead_entries.entry_id = o.entry_id AND lead_entries.entry_type = o.entry_type WHERE o.workspace_id = ? AND o.project_id = ? AND o.arrived_at >= ? AND o.arrived_at < ?) page_leads WHERE page_leads.lead_id IS NOT NULL"

func (p *PageLeads) Leads(ctx context.Context, workspaceID, projectID string, from, to time.Time) ([]string, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(projectID) == "" || !to.After(from) {
		return nil, ErrPageLeadsInvalid
	}
	var leads []string
	err := p.db.WithContext(ctx).Raw(pageLeadsSQL, workspaceID, projectID, from, to, workspaceID, projectID, from, to).Scan(&leads).Error
	if err != nil {
		return nil, err
	}
	return leads, nil
}
