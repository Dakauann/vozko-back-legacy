package sitebuild

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"vozko/domain/inlinemarkup"
	"vozko/domain/site"
)

type Mode string

const (
	ModePreview Mode = "preview"
	ModePublish Mode = "publish"
)

type ExecuteRequest struct {
	Project     site.Project
	Operations  []site.Operation
	Texts       map[int][]inlinemarkup.Run
	Media       map[string]string
	Sections    map[string]string
	JSXSections map[string]string
	Bases       map[string][]site.File
}

type ExtractRequest struct {
	Project site.Project
	Node    string
	Rev     uint64
	Bases   map[string][]site.File
}

type ExtractResult struct {
	Source  string
	Failure *ExecuteFailure
}

type Splice struct {
	Index  int    `json:"index"`
	Path   string `json:"path"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Length int    `json:"length"`
}

type ExecuteFailure struct {
	Index  int    `json:"index"`
	Code   string `json:"code"`
	Field  string `json:"field,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type ExecuteResult struct {
	Steps    []site.StepPatch
	Splices  []Splice
	Failures []ExecuteFailure
	Skipped  []site.Skip
}

func (r ExecuteResult) SiteFailures() []site.Failure {
	out := make([]site.Failure, len(r.Failures))
	for i, f := range r.Failures {
		out[i] = site.Failure{Index: f.Index, Code: f.Code, Field: f.Field, Detail: f.Detail}
	}
	return out
}

type PageMeta struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Canonical   string `json:"canonical,omitempty"`
	OGImage     string `json:"og_image,omitempty"`
	NoIndex     bool   `json:"noindex,omitempty"`
}

type Shell struct {
	API            string              `json:"api"`
	Forms          map[string]string   `json:"forms"`
	Variant        string              `json:"variant,omitempty"`
	Pages          map[string]PageMeta `json:"pages"`
	Integrations   map[string]string   `json:"integrations"`
	Loaders        map[string]string   `json:"loaders"`
	ConsentText    string              `json:"consent_text,omitempty"`
	PrimaryOrigin  string              `json:"primary_origin,omitempty"`
	Favicon        string              `json:"favicon,omitempty"`
	StructuredData []json.RawMessage   `json:"structured_data,omitempty"`
	Webchat        string              `json:"webchat,omitempty"`
}

type BuildRequest struct {
	Mode    Mode
	Project site.Project
	Shell   Shell
	Runtime map[string]string
}

type Asset struct {
	Path        string
	ContentType string
	Body        []byte
}

type RouteOutput struct {
	Route       string
	HTMLPath    string
	Interactive bool
}

type BuildError struct {
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type BuildWarning struct {
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type BuildResult struct {
	Assets        []Asset
	Imports       []site.Import
	Routes        []RouteOutput
	Errors        []BuildError
	Warnings      []BuildWarning
	PrerenderPath string
}

type Builder interface {
	Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error)
	Build(ctx context.Context, req BuildRequest) (BuildResult, error)
	Extract(ctx context.Context, req ExtractRequest) (ExtractResult, error)
}

type MediaLocator interface {
	Locate(ctx context.Context, workspaceID string, mediaIDs []string) (map[string]string, error)
}

type Section struct {
	Stack  site.Stack
	Source string
}

type SectionLibrary interface {
	Section(ctx context.Context, workspaceID string, stack site.Stack, key string) (Section, bool, error)
}

type WhatsAppLinks interface {
	DirectLink(ctx context.Context, workspaceID, numberRef, text string) (string, error)
}

type WorkspaceNames interface {
	WorkspaceName(ctx context.Context, workspaceID string) (string, error)
}

var (
	ErrBuilderUnavailable = errors.New("sitebuild: the site builder is unavailable")
	ErrBuilderBusy        = errors.New("sitebuild: the site builder is busy")
	ErrBuilderTimeout     = errors.New("sitebuild: the site builder took too long")
	ErrBuilderRefused     = errors.New("sitebuild: the site builder refused the request")
	ErrBuilderReply       = errors.New("sitebuild: the site builder replied with a malformed answer")

	ErrProjectInvalid     = errors.New("sitebuild: the project breaks the source rules")
	ErrBuildFailed        = errors.New("sitebuild: the build failed")
	ErrImportsRefused     = errors.New("sitebuild: the build imports packages outside the allow list")
	ErrMediaUnavailable   = errors.New("sitebuild: a media item could not be located")
	ErrPreviewUnavailable = errors.New("sitebuild: the preview store is unavailable")
	ErrPreviewNotFound    = errors.New("sitebuild: the preview does not exist or expired")

	ErrSectionsUnavailable = errors.New("sitebuild: the saved sections cannot be read now")
	ErrSectionStackInvalid = errors.New("sitebuild: the sections are listed for react or html")
)

type BuilderError struct {
	Kind    error
	Status  int
	Code    string
	Message string
}

func (e *BuilderError) Error() string {
	parts := []string{e.Kind.Error()}
	if e.Status != 0 {
		parts = append(parts, "status "+strconv.Itoa(e.Status))
	}
	if e.Code != "" {
		parts = append(parts, e.Code)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, ": ")
}

func (e *BuilderError) Unwrap() error {
	return e.Kind
}

type ProjectInvalid struct {
	Issues []site.Issue
}

func (e *ProjectInvalid) Error() string {
	return ErrProjectInvalid.Error() + ": " + strconv.Itoa(len(e.Issues)) + " issues"
}

func (e *ProjectInvalid) Unwrap() error {
	return ErrProjectInvalid
}

type BuildFailure struct {
	Errors   []BuildError
	Warnings []BuildWarning
}

func (e *BuildFailure) Error() string {
	if len(e.Errors) == 0 {
		return ErrBuildFailed.Error()
	}
	first := e.Errors[0]
	return ErrBuildFailed.Error() + ": " + first.Path + ":" + strconv.Itoa(first.Line) + ": " + first.Message
}

func (e *BuildFailure) Unwrap() error {
	return ErrBuildFailed
}

type ImportsRefused struct {
	Issues []site.Issue
}

func (e *ImportsRefused) Error() string {
	return ErrImportsRefused.Error() + ": " + strconv.Itoa(len(e.Issues)) + " imports"
}

func (e *ImportsRefused) Unwrap() error {
	return ErrImportsRefused
}
