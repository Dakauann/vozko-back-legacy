package site_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"vozko/domain/site"
)

type SaveFilesInput struct {
	WorkspaceID     string
	ProjectID       string
	ExpectedVersion int64
	Author          Author
	Files           map[string]string
	Deleted         []string
}

func (s *Service) SaveFiles(ctx context.Context, in SaveFilesInput) (*View, error) {
	if !in.Author.Valid() {
		return nil, ErrAuthorInvalid
	}
	base, err := s.editableAt(ctx, in.WorkspaceID, in.ProjectID, in.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	next := base.Source().Clone()
	for _, path := range in.Deleted {
		if !next.Has(path) {
			return nil, fmt.Errorf("%w: %s", ErrPathNotFound, path)
		}
		delete(next.Files, path)
	}
	for _, path := range slices.Sorted(maps.Keys(in.Files)) {
		content := in.Files[path]
		current, exists := base.Files[path]
		if exists && current.Content == content && next.Has(path) {
			continue
		}
		next.Files[path] = site.File{Content: content, Rev: current.Rev + 1}
	}
	if sameFiles(base.Files, next.Files) {
		base.Changes = site.Changes(base.Files, base.Files, nil)
		return base, nil
	}
	if issues := introduced(base.Source(), next); len(issues) > 0 {
		return nil, &IssuesError{Issues: issues}
	}
	return s.commit(ctx, base, next.Files, revisionPlan{author: in.Author, kind: RevisionAutosave})
}

type EditInput struct {
	WorkspaceID     string
	ProjectID       string
	ExpectedVersion int64
	Author          Author
	Label           string
	Batch           json.RawMessage
}

type EditResult struct {
	View    View
	Steps   []site.StepPatch
	Ignored []site.Ignored
}

func validLabel(label string) bool {
	return utf8.RuneCountInString(label) <= maxLabelRunes && !strings.ContainsAny(label, "\r\n")
}

func (s *Service) Edit(ctx context.Context, in EditInput) (*EditResult, error) {
	if !in.Author.Valid() {
		return nil, ErrAuthorInvalid
	}
	if !validLabel(in.Label) {
		return nil, ErrLabelInvalid
	}
	base, err := s.editableAt(ctx, in.WorkspaceID, in.ProjectID, in.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	project := base.Source()
	batch, err := site.PrepareBatch(project, in.Batch)
	var refused *site.BatchError
	if errors.As(err, &refused) {
		return nil, &EditRefusal{Failures: refused.Failures, Skipped: refused.Skipped, Ignored: batch.Ignored}
	}
	if err != nil {
		return nil, err
	}
	if failures := s.foreignNumbers(ctx, in.WorkspaceID, batch); len(failures) > 0 {
		return nil, &EditRefusal{Failures: failures, Ignored: batch.Ignored}
	}
	elementFailures, err := s.foreignElements(ctx, in.WorkspaceID, in.ProjectID, batch)
	if err != nil {
		return nil, err
	}
	if len(elementFailures) > 0 {
		return nil, &EditRefusal{Failures: elementFailures, Ignored: batch.Ignored}
	}
	bases, err := s.bases(ctx, base, batch.StaleRevs(project))
	if err != nil {
		return nil, err
	}
	result, err := s.executor.Execute(ctx, ExecuteRequest{WorkspaceID: in.WorkspaceID, ProjectID: in.ProjectID, Project: project, Batch: batch, Bases: bases})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuilderUnavailable, err)
	}
	result = sourced(batch, result)
	if len(result.Failures) > 0 {
		return nil, &EditRefusal{Failures: result.Failures, Skipped: result.Skipped, Ignored: batch.Ignored}
	}
	next, err := site.ApplySteps(project, result.Steps)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStepsInvalid, err)
	}
	if issues := introduced(project, next); len(issues) > 0 {
		return nil, &EditRefusal{Issues: issues, Ignored: batch.Ignored}
	}
	plan := revisionPlan{author: in.Author, kind: RevisionAutosave, steps: result.Steps}
	if in.Author.Elo() {
		plan = revisionPlan{author: in.Author, kind: RevisionElo, label: strings.TrimSpace(in.Label), steps: result.Steps}
	}
	view := base
	view.Changes = site.Changes(base.Files, base.Files, nil)
	if !sameFiles(base.Files, next.Files) {
		view, err = s.commit(ctx, base, next.Files, plan)
		if err != nil {
			return nil, err
		}
	}
	return &EditResult{View: *view, Steps: result.Steps, Ignored: batch.Ignored}, nil
}
