package siteform_repository

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/sitesubmission"
	"vozko/infra/database/schema"
	"vozko/usecases/siteforms"
)

const attachConversationSQL = `UPDATE site_submissions SET conversation_id = ?, conversation_type = ? WHERE id = ?`

type SubmissionStore struct {
	db *gorm.DB
}

func NewSubmissionStore(db *gorm.DB) *SubmissionStore {
	return &SubmissionStore{db: db}
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func valueOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func submissionRow(s sitesubmission.Submission) (schema.SiteSubmission, error) {
	fields, err := json.Marshal(s.Fields)
	if err != nil {
		return schema.SiteSubmission{}, err
	}
	utm, err := json.Marshal(s.UTM)
	if err != nil {
		return schema.SiteSubmission{}, err
	}
	consent, err := json.Marshal(s.Consent)
	if err != nil {
		return schema.SiteSubmission{}, err
	}
	return schema.SiteSubmission{
		ID: s.ID, WorkspaceID: s.WorkspaceID, ProjectID: s.ProjectID, DeploymentID: s.DeploymentID, FormKey: s.FormKey, FormID: s.FormID,
		Route: s.Route, Variant: s.Variant, Fields: fields, UTM: utm, ReferrerOrigin: s.ReferrerOrigin, Fbclid: s.Fbclid, Gclid: s.Gclid,
		Fbp: s.Fbp, Fbc: s.Fbc, Consent: consent, IPHash: s.IPHash, UserAgent: s.UserAgent, LeadID: optional(s.LeadID),
		ConversationID: optional(s.ConversationID), ConversationType: s.ConversationType, IdempotencyKey: s.IdempotencyKey, CreatedAt: s.CreatedAt,
	}, nil
}

func submissionOf(row schema.SiteSubmission) (sitesubmission.Submission, error) {
	s := sitesubmission.Submission{
		ID: row.ID, WorkspaceID: row.WorkspaceID, ProjectID: row.ProjectID, DeploymentID: row.DeploymentID, FormKey: row.FormKey, FormID: row.FormID,
		Route: row.Route, Variant: row.Variant, ReferrerOrigin: row.ReferrerOrigin, Fbclid: row.Fbclid, Gclid: row.Gclid, Fbp: row.Fbp, Fbc: row.Fbc,
		IPHash: row.IPHash, UserAgent: row.UserAgent, LeadID: valueOf(row.LeadID), ConversationID: valueOf(row.ConversationID),
		ConversationType: row.ConversationType, IdempotencyKey: row.IdempotencyKey, CreatedAt: row.CreatedAt,
	}
	for _, part := range []struct {
		raw  []byte
		into any
	}{{row.Fields, &s.Fields}, {row.UTM, &s.UTM}, {row.Consent, &s.Consent}} {
		if err := json.Unmarshal(part.raw, part.into); err != nil {
			return sitesubmission.Submission{}, fmt.Errorf("%w: submission %s: %w", ErrStoredFormInvalid, row.ID, err)
		}
	}
	return s, nil
}

func (s *SubmissionStore) ByIdempotency(ctx context.Context, formKey, key string) (*sitesubmission.Submission, error) {
	var rows []schema.SiteSubmission
	if err := s.db.WithContext(ctx).Where("form_key = ? AND idempotency_key = ?", formKey, key).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	sub, err := submissionOf(rows[0])
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *SubmissionStore) Create(ctx context.Context, sub sitesubmission.Submission, effects sitesubmission.EffectRun) (sitesubmission.Submission, bool, error) {
	row, err := submissionRow(sub)
	if err != nil {
		return sitesubmission.Submission{}, false, err
	}
	created := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if result.Error != nil || result.RowsAffected != 1 {
			return result.Error
		}
		created = true
		if len(effects.Pending) == 0 {
			return nil
		}
		return insertEffect(tx, effects)
	})
	if err != nil {
		return sitesubmission.Submission{}, false, err
	}
	if created {
		return sub, true, nil
	}
	existing, err := s.ByIdempotency(ctx, sub.FormKey, sub.IdempotencyKey)
	if err != nil {
		return sitesubmission.Submission{}, false, err
	}
	if existing == nil {
		return sitesubmission.Submission{}, false, fmt.Errorf("%w: submission %s collided without an original", ErrStoredFormInvalid, sub.ID)
	}
	return *existing, false, nil
}

func (s *SubmissionStore) AttachConversation(ctx context.Context, id, conversationID, conversationType string) error {
	return s.db.WithContext(ctx).Exec(attachConversationSQL, conversationID, conversationType, id).Error
}

func (s *SubmissionStore) List(ctx context.Context, q siteforms.SubmissionQuery) ([]sitesubmission.Submission, int64, error) {
	scope := s.db.WithContext(ctx).Table("site_submissions AS s").
		Joins("LEFT JOIN leads l ON l.id = s.lead_id").
		Where("s.workspace_id = ? AND s.project_id = ?", q.WorkspaceID, q.ProjectID).
		Where("s.lead_id IS NULL OR l.deleted_at IS NULL")
	if q.FormID != "" {
		scope = scope.Where("s.form_id = ?", q.FormID)
	}
	if q.From != nil {
		scope = scope.Where("s.created_at >= ?", *q.From)
	}
	if q.To != nil {
		scope = scope.Where("s.created_at < ?", *q.To)
	}
	var total int64
	if err := scope.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []schema.SiteSubmission
	if err := scope.Session(&gorm.Session{}).Select("s.*").Order("s.created_at DESC, s.id DESC").
		Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]sitesubmission.Submission, 0, len(rows))
	for _, row := range rows {
		sub, err := submissionOf(row)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, sub)
	}
	return out, total, nil
}
