package sitehttp

import (
	"encoding/json"
	"time"

	"vozko/domain/site"
)

type CreateRequest struct {
	Name           string        `json:"name,omitempty" example:"Clínica Sorriso"`
	Brief          string        `json:"brief,omitempty" example:"Uma landing page para a Clínica Sorriso com agendamento pelo WhatsApp"`
	Stack          string        `json:"stack" enums:"react,html"`
	Starter        string        `json:"starter" example:"captura-de-leads"`
	DesignSystemID string        `json:"designSystemId,omitempty" format:"uuid"`
	Origin         *CreateOrigin `json:"origin,omitempty"`
}

type DuplicateRequest struct {
	Name string `json:"name,omitempty" example:"Clínica Sorriso (cópia)"`
}

type CreateOrigin struct {
	ThreadID  string `json:"threadId" example:"thread_9"`
	CommandID string `json:"commandId" format:"uuid"`
}

type UpdateRequest struct {
	Name           *string `json:"name,omitempty"`
	Slug           *string `json:"slug,omitempty" example:"clinica-sorriso"`
	DesignSystemID *string `json:"designSystemId,omitempty" format:"uuid"`
	Archived       *bool   `json:"archived,omitempty"`
}

type SaveFilesRequest struct {
	Files   map[string]string `json:"files"`
	Deleted []string          `json:"deleted,omitempty"`
}

type EditRequest struct {
	Operations json.RawMessage `json:"operations" swaggertype:"array,object"`
}

type SaveVersionRequest struct {
	Label string `json:"label" example:"Antes da Elo"`
}

type DesignSystemRequest struct {
	System    site.DesignSystem `json:"system"`
	IsDefault bool              `json:"isDefault"`
}

type ProjectResponse struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Slug                  string    `json:"slug"`
	Stack                 string    `json:"stack" enums:"react,html"`
	Status                string    `json:"status" enums:"draft,published,suspended,archived"`
	WorkingRevisionID     string    `json:"workingRevisionId"`
	PublishedDeploymentID string    `json:"publishedDeploymentId,omitempty"`
	DesignSystemID        string    `json:"designSystemId,omitempty"`
	ThumbnailURL          string    `json:"thumbnailUrl,omitempty"`
	HasUnpublishedChanges bool      `json:"hasUnpublishedChanges"`
	PublishState          string    `json:"publishState" enums:"draft,published,published_with_changes,suspended,archived"`
	Version               int64     `json:"version"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

type ListItemResponse struct {
	ProjectResponse
	PrimaryURL string `json:"primaryUrl,omitempty" example:"https://www.clinicasorriso.com.br"`
}

type ListResponse struct {
	Items []ListItemResponse `json:"items"`
	Total int64              `json:"total"`
}

type RevisionResponse struct {
	ID         string    `json:"id"`
	ParentID   string    `json:"parentId,omitempty"`
	Author     string    `json:"author" example:"person:7b1c2f9e-0a51-4b9e-9c1d-2f3e4a5b6c7d"`
	AuthorKind string    `json:"authorKind" enums:"person,elo"`
	AuthorID   string    `json:"authorId" example:"7b1c2f9e-0a51-4b9e-9c1d-2f3e4a5b6c7d"`
	Label      string    `json:"label"`
	Kind       string    `json:"kind" enums:"autosave,checkpoint,elo,publish,restore"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type RevisionListResponse struct {
	Items []RevisionResponse `json:"items"`
	Total int64              `json:"total"`
}

type SettingsResponse struct {
	Settings  site.Settings `json:"settings"`
	Version   int64         `json:"version"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

type FactsResponse struct {
	Routes      []site.Route     `json:"routes"`
	RoutesIssue string           `json:"routesIssue,omitempty" enums:"site_json_invalid"`
	Tokens      []site.Token     `json:"tokens"`
	Knobs       []site.KnobState `json:"knobs"`
	KnobsIssue  string           `json:"knobsIssue,omitempty" enums:"knobs_invalid"`
	Breakpoints []string         `json:"breakpoints" enums:"sm,md,lg,xl"`
}

type DetailResponse struct {
	Project               ProjectResponse      `json:"project"`
	Files                 map[string]site.File `json:"files"`
	Settings              SettingsResponse     `json:"settings"`
	Revision              RevisionResponse     `json:"revision"`
	HasUnpublishedChanges bool                 `json:"hasUnpublishedChanges"`
	PublishState          string               `json:"publishState" enums:"draft,published,published_with_changes,suspended,archived"`
	PrimaryURL            string               `json:"primaryUrl,omitempty" example:"https://www.clinicasorriso.com.br"`
	Facts                 FactsResponse        `json:"facts"`
}

type FilesResponse struct {
	Project  ProjectResponse      `json:"project"`
	Files    map[string]site.File `json:"files"`
	Revision RevisionResponse     `json:"revision"`
	Changes  site.FileChanges     `json:"changes"`
	Facts    FactsResponse        `json:"facts"`
}

type SavedResponse struct {
	Project  ProjectResponse   `json:"project"`
	Revision RevisionResponse  `json:"revision"`
	Revs     map[string]uint64 `json:"revs"`
	Changes  site.FileChanges  `json:"changes"`
	Facts    FactsResponse     `json:"facts"`
}

type EditResponse struct {
	Project  ProjectResponse   `json:"project"`
	Revision RevisionResponse  `json:"revision"`
	Revs     map[string]uint64 `json:"revs"`
	Steps    []site.StepPatch  `json:"steps"`
	Ignored  []site.Ignored    `json:"ignored,omitempty"`
	Changes  site.FileChanges  `json:"changes"`
	Facts    FactsResponse     `json:"facts"`
}

type EditRefusalDetails struct {
	Failures []site.Failure `json:"failures,omitempty"`
	Skipped  []site.Skip    `json:"skipped,omitempty"`
	Ignored  []site.Ignored `json:"ignored,omitempty"`
	Issues   []site.Issue   `json:"issues,omitempty"`
}

type IssuesDetails struct {
	Issues []site.Issue `json:"issues"`
}

type FormDetails struct {
	Form string `json:"form"`
}

type OutlineResponse struct {
	Routes      []site.RouteOutline  `json:"routes"`
	Forms       []string             `json:"forms"`
	OpenGaps    []site.Issue         `json:"openGaps"`
	GapsChecked bool                 `json:"gapsChecked"`
	ReasonCode  string               `json:"reasonCode,omitempty" enums:"site_json_invalid,build_failed,rules_broken,preview_unavailable,capture_unavailable,snapshot_stale,snapshot_invalid,route_unreadable,not_inspected,inspection_throttled"`
	ReasonRoute string               `json:"reasonRoute,omitempty" example:"/obrigado"`
	BuildErrors []BuildErrorResponse `json:"buildErrors,omitempty"`
	RuleIssues  []site.Issue         `json:"ruleIssues,omitempty"`
	Checks      []site.Check         `json:"checks"`
	PreviewURL  string               `json:"previewUrl,omitempty"`
}

type PreviewResponse struct {
	URL        string `json:"url"`
	RevisionID string `json:"revisionId"`
}

type SectionTitleResponse struct {
	PT string `json:"pt" example:"Perguntas frequentes"`
	EN string `json:"en" example:"FAQ"`
	ES string `json:"es" example:"Preguntas frecuentes"`
	DE string `json:"de" example:"Häufige Fragen"`
}

type SectionResponse struct {
	Key    string               `json:"key" example:"faq"`
	Kind   string               `json:"kind" enums:"library,snippet"`
	Group  string               `json:"group" enums:"sections,forms,fields,buttons,media,platform,saved"`
	Title  SectionTitleResponse `json:"title"`
	Stacks []string             `json:"stacks" enums:"react,html"`
}

type SectionsResponse struct {
	Sections []SectionResponse `json:"sections"`
}

type StarterResponse struct {
	Stack       string `json:"stack" enums:"react,html"`
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PreviewURL  string `json:"previewUrl" example:"https://api.example.com/public/sites/starters/react/captura-de-leads/preview.jpg"`
}

type SaveSnippetRequest struct {
	Node string `json:"node" example:"src/routes/index.jsx#120"`
	Rev  uint64 `json:"rev" example:"4"`
	Name string `json:"name" example:"Faixa de oferta"`
}

type SnippetResponse struct {
	SectionResponse
	DesignSystemID      string `json:"designSystemId" format:"uuid"`
	DesignSystemVersion int64  `json:"designSystemVersion"`
}

type DesignSystemResponse struct {
	ID        string            `json:"id"`
	System    site.DesignSystem `json:"system"`
	IsDefault bool              `json:"isDefault"`
	Version   int64             `json:"version"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

type BuildErrorResponse struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type BuildFailedDetails struct {
	Errors []BuildErrorResponse `json:"errors"`
}
