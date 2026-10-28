package siteform_repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"vozko/domain/sitesubmission"
	"vozko/infra/database/schema"
)

const (
	insertEffectSQL   = `INSERT INTO site_submission_effects (submission_id, workspace_id, plan, pending, owner_id, entry_id, entry_type, attempts, next_attempt_at, finished_at, last_error, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	claimEffectSQL    = `UPDATE site_submission_effects SET locked_until = ? WHERE submission_id = ? AND finished_at IS NULL AND next_attempt_at <= ? AND (locked_until IS NULL OR locked_until <= ?) RETURNING *`
	dueEffectsSQL     = `SELECT submission_id FROM site_submission_effects WHERE finished_at IS NULL AND next_attempt_at <= ? AND (locked_until IS NULL OR locked_until <= ?) ORDER BY next_attempt_at LIMIT ?`
	progressEffectSQL = `UPDATE site_submission_effects SET pending = ?, owner_id = ?, entry_id = ?, entry_type = ? WHERE submission_id = ?`
	settleEffectSQL   = `UPDATE site_submission_effects SET pending = ?, owner_id = ?, entry_id = ?, entry_type = ?, attempts = ?, next_attempt_at = ?, finished_at = ?, last_error = ?, locked_until = NULL WHERE submission_id = ?`
	maxLastErrorRunes = 1000
)

var ErrEffectRunMissing = errors.New("siteform repository: the effects of the submission are gone")

type EffectStore struct {
	db *gorm.DB
}

func NewEffectStore(db *gorm.DB) *EffectStore {
	return &EffectStore{db: db}
}

func pendingJSON(pending []sitesubmission.Effect) (string, error) {
	if pending == nil {
		pending = []sitesubmission.Effect{}
	}
	raw, err := json.Marshal(pending)
	return string(raw), err
}

func effectRow(run sitesubmission.EffectRun) (schema.SiteSubmissionEffect, error) {
	plan, err := json.Marshal(run.Plan)
	if err != nil {
		return schema.SiteSubmissionEffect{}, err
	}
	pending, err := pendingJSON(run.Pending)
	if err != nil {
		return schema.SiteSubmissionEffect{}, err
	}
	return schema.SiteSubmissionEffect{
		SubmissionID: run.SubmissionID, WorkspaceID: run.WorkspaceID, Plan: plan, Pending: []byte(pending), OwnerID: run.OwnerID, EntryID: run.EntryID,
		EntryType: run.EntryType, Attempts: run.Attempts, NextAttemptAt: run.NextAttemptAt, FinishedAt: run.FinishedAt, LastError: clipError(run.LastError),
		CreatedAt: run.CreatedAt,
	}, nil
}

func effectRunOf(row schema.SiteSubmissionEffect) (sitesubmission.EffectRun, error) {
	run := sitesubmission.EffectRun{
		SubmissionID: row.SubmissionID, WorkspaceID: row.WorkspaceID, OwnerID: row.OwnerID, EntryID: row.EntryID, EntryType: row.EntryType,
		Attempts: row.Attempts, NextAttemptAt: row.NextAttemptAt, FinishedAt: row.FinishedAt, LastError: row.LastError, CreatedAt: row.CreatedAt,
	}
	if err := json.Unmarshal(row.Plan, &run.Plan); err != nil {
		return sitesubmission.EffectRun{}, fmt.Errorf("%w: effects of submission %s: %w", ErrStoredFormInvalid, row.SubmissionID, err)
	}
	if err := json.Unmarshal(row.Pending, &run.Pending); err != nil {
		return sitesubmission.EffectRun{}, fmt.Errorf("%w: effects of submission %s: %w", ErrStoredFormInvalid, row.SubmissionID, err)
	}
	return run, nil
}

func insertEffect(tx *gorm.DB, run sitesubmission.EffectRun) error {
	row, err := effectRow(run)
	if err != nil {
		return err
	}
	return tx.Exec(insertEffectSQL, row.SubmissionID, row.WorkspaceID, string(row.Plan), string(row.Pending), row.OwnerID, row.EntryID, row.EntryType, row.Attempts,
		row.NextAttemptAt, row.FinishedAt, row.LastError, row.CreatedAt).Error
}

func clipError(text string) string {
	if utf8.RuneCountInString(text) <= maxLastErrorRunes {
		return text
	}
	return string([]rune(text)[:maxLastErrorRunes])
}

func (s *EffectStore) Claim(ctx context.Context, submissionID string, now time.Time, lease time.Duration) (*sitesubmission.EffectRun, error) {
	var rows []schema.SiteSubmissionEffect
	if err := s.db.WithContext(ctx).Raw(claimEffectSQL, now.Add(lease), submissionID, now, now).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	run, err := effectRunOf(rows[0])
	if err != nil {
		return nil, err
	}
	var subs []schema.SiteSubmission
	if err := s.db.WithContext(ctx).Where("id = ?", submissionID).Limit(1).Find(&subs).Error; err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, fmt.Errorf("%w: submission %s has effects but no row", ErrStoredFormInvalid, submissionID)
	}
	if run.Submission, err = submissionOf(subs[0]); err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *EffectStore) Due(ctx context.Context, now time.Time, limit int) ([]string, error) {
	var ids []string
	if err := s.db.WithContext(ctx).Raw(dueEffectsSQL, now, now, limit).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *EffectStore) exec(ctx context.Context, submissionID, sql string, args ...any) error {
	result := s.db.WithContext(ctx).Exec(sql, args...)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrEffectRunMissing, submissionID)
	}
	return nil
}

func (s *EffectStore) Progress(ctx context.Context, run sitesubmission.EffectRun) error {
	pending, err := pendingJSON(run.Pending)
	if err != nil {
		return err
	}
	return s.exec(ctx, run.SubmissionID, progressEffectSQL, pending, run.OwnerID, run.EntryID, run.EntryType, run.SubmissionID)
}

func (s *EffectStore) Settle(ctx context.Context, run sitesubmission.EffectRun) error {
	pending, err := pendingJSON(run.Pending)
	if err != nil {
		return err
	}
	return s.exec(ctx, run.SubmissionID, settleEffectSQL, pending, run.OwnerID, run.EntryID, run.EntryType, run.Attempts, run.NextAttemptAt,
		run.FinishedAt, clipError(run.LastError), run.SubmissionID)
}
