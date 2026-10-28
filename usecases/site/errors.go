package site_usecase

import (
	"errors"

	"vozko/domain/site"
)

var (
	ErrMissingDependency    = errors.New("site: the service is missing a dependency")
	ErrWorkspaceRequired    = errors.New("site: the workspace is required")
	ErrAuthorInvalid        = errors.New("site: the author is not a person or an Elo thread")
	ErrProjectNotFound      = errors.New("site: the page was not found")
	ErrProjectArchived      = errors.New("site: the page is archived")
	ErrProjectPublished     = errors.New("site: the page is live; unpublish it first")
	ErrProjectSuspended     = errors.New("site: the page is suspended")
	ErrProjectDomainsBusy   = errors.New("site: a domain of the page is being checked right now")
	ErrNameInvalid          = errors.New("site: the name must have 1 to 120 characters")
	ErrSlugTaken            = errors.New("site: another page of the workspace already uses this address")
	ErrStarterUnknown       = errors.New("site: the starter does not exist for this stack")
	ErrOriginTaken          = errors.New("site: this Elo request created a page for someone else")
	ErrRevisionNotFound     = errors.New("site: the version was not found")
	ErrRevisionDamaged      = errors.New("site: a file of this version is missing from storage")
	ErrLabelInvalid         = errors.New("site: the version label must have up to 120 characters")
	ErrFilesRefused         = errors.New("site: the files break a project rule")
	ErrPathNotFound         = errors.New("site: a deleted file does not exist")
	ErrStepsInvalid         = errors.New("site: the builder returned steps that do not fit the project")
	ErrBuilderUnavailable   = errors.New("site: the page builder is unavailable")
	ErrPreviewUnavailable   = errors.New("site: the preview could not be prepared")
	ErrPreviewNotBuilt      = errors.New("site: no preview was built for the working version yet")
	ErrDesignSystemNotFound = errors.New("site: the design system was not found")
	ErrFormFunnelUnknown    = errors.New("site: the form funnel is not in this workspace")
	ErrFormStageUnknown     = errors.New("site: the form stage is not in this workspace or funnel")
	ErrFormOwnerUnknown     = errors.New("site: the form owner is not in this workspace")
	ErrFormWorkflowUnknown  = errors.New("site: the form workflow is not in this workspace")
	ErrFormChannelUnknown   = errors.New("site: the form follow up channel is not in this workspace")
	ErrStatusInvalid        = errors.New("site: the status filter is not a page status")
	ErrPublicationInvalid   = errors.New("site: a live page needs its deployment and a draft has none")
	ErrSnippetsFull         = errors.New("site: the saved sections are larger than a project")
)

type IssuesError struct {
	Issues []site.Issue
}

func (e *IssuesError) Error() string {
	if len(e.Issues) == 0 {
		return ErrFilesRefused.Error()
	}
	return ErrFilesRefused.Error() + ": " + e.Issues[0].Code + " " + e.Issues[0].Path
}

func (e *IssuesError) Is(target error) bool {
	return target == ErrFilesRefused
}

func (e *IssuesError) Code() string {
	if len(e.Issues) == 0 {
		return site.CodeEditRefused
	}
	return e.Issues[0].Code
}

type EditRefusal struct {
	Failures []site.Failure
	Skipped  []site.Skip
	Ignored  []site.Ignored
	Issues   []site.Issue
}

func (e *EditRefusal) Error() string {
	return site.ErrEditRefused.Error()
}

func (e *EditRefusal) Is(target error) bool {
	return target == site.ErrEditRefused
}

type FormReferenceError struct {
	Form string
	Err  error
}

func (e *FormReferenceError) Error() string {
	return e.Err.Error() + ": " + e.Form
}

func (e *FormReferenceError) Unwrap() error {
	return e.Err
}

var ErrBuildFailed = errors.New("site: the page did not compile")

type BuildIssue struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type BuildFailedError struct {
	Errors []BuildIssue
}

func (e *BuildFailedError) Error() string {
	if len(e.Errors) == 0 {
		return ErrBuildFailed.Error()
	}
	return ErrBuildFailed.Error() + ": " + e.Errors[0].Path + ": " + e.Errors[0].Message
}

func (e *BuildFailedError) Is(target error) bool {
	return target == ErrBuildFailed
}
