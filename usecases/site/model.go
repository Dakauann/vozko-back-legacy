package site_usecase

import (
	"regexp"
	"strings"
	"time"

	"vozko/domain/site"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusSuspended Status = "suspended"
	StatusArchived  Status = "archived"
)

func Statuses() []Status {
	return []Status{StatusDraft, StatusPublished, StatusSuspended, StatusArchived}
}

func (s Status) Valid() bool {
	return s == StatusDraft || s == StatusPublished || s == StatusSuspended || s == StatusArchived
}

type Origin struct {
	ThreadID  string
	CommandID string
}

func (o Origin) Empty() bool {
	return o.ThreadID == "" && o.CommandID == ""
}

type Project struct {
	ID                    string
	WorkspaceID           string
	Name                  string
	Slug                  string
	Stack                 site.Stack
	Status                Status
	WorkingRevisionID     string
	PublishedDeploymentID string
	DesignSystemID        string
	ThumbnailMediaID      string
	ThumbnailURL          string
	HasUnpublishedChanges bool
	PrimaryURL            string
	Origin                Origin
	Version               int64
	CreatedBy             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type Author string

const (
	personPrefix = "person:"
	eloPrefix    = "elo:"
)

var authorRef = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func PersonAuthor(userID string) Author {
	return Author(personPrefix + userID)
}

func EloAuthor(threadID string) Author {
	return Author(eloPrefix + threadID)
}

func (a Author) Valid() bool {
	for _, prefix := range []string{personPrefix, eloPrefix} {
		if ref, ok := strings.CutPrefix(string(a), prefix); ok {
			return authorRef.MatchString(ref)
		}
	}
	return false
}

func (a Author) Elo() bool {
	return strings.HasPrefix(string(a), eloPrefix)
}

type AuthorKind string

const (
	AuthorPerson AuthorKind = "person"
	AuthorElo    AuthorKind = "elo"
)

func (a Author) Parts() (AuthorKind, string) {
	if !a.Valid() {
		return "", ""
	}
	if ref, ok := strings.CutPrefix(string(a), eloPrefix); ok {
		return AuthorElo, ref
	}
	return AuthorPerson, strings.TrimPrefix(string(a), personPrefix)
}

type PublishState string

const (
	PublishDraft           PublishState = "draft"
	PublishLive            PublishState = "published"
	PublishLiveWithChanges PublishState = "published_with_changes"
	PublishSuspended       PublishState = "suspended"
	PublishArchived        PublishState = "archived"
)

func PublishStates() []PublishState {
	return []PublishState{PublishDraft, PublishLive, PublishLiveWithChanges, PublishSuspended, PublishArchived}
}

func (p Project) PublishState() PublishState {
	switch p.Status {
	case StatusPublished:
		if p.HasUnpublishedChanges {
			return PublishLiveWithChanges
		}
		return PublishLive
	case StatusSuspended:
		return PublishSuspended
	case StatusArchived:
		return PublishArchived
	}
	return PublishDraft
}

type RevisionKind string

const (
	RevisionAutosave   RevisionKind = "autosave"
	RevisionCheckpoint RevisionKind = "checkpoint"
	RevisionElo        RevisionKind = "elo"
	RevisionPublish    RevisionKind = "publish"
	RevisionRestore    RevisionKind = "restore"
)

type ManifestEntry struct {
	SHA256 string `json:"sha256"`
	Rev    uint64 `json:"rev"`
}

type Revision struct {
	ID          string
	WorkspaceID string
	ProjectID   string
	ParentID    string
	Manifest    map[string]ManifestEntry
	Author      Author
	Label       string
	Kind        RevisionKind
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Blob struct {
	SHA256 string
	Bytes  []byte
}

type StoredSettings struct {
	ProjectID   string
	WorkspaceID string
	Settings    site.Settings
	Version     int64
	UpdatedAt   time.Time
}

type DesignSystemRecord struct {
	ID          string
	WorkspaceID string
	System      site.DesignSystem
	IsDefault   bool
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ListQuery struct {
	WorkspaceID string
	Status      Status
	Search      string
	Limit       int
	Offset      int
}

type View struct {
	Project               Project
	Files                 map[string]site.File
	Settings              *StoredSettings
	Revision              Revision
	HasUnpublishedChanges bool
	PrimaryURL            string
	Changes               site.FileChanges
	liveRevisionID        string
}

func (v View) Source() site.Project {
	return site.Project{Stack: v.Project.Stack, Files: v.Files}
}

type Facts struct {
	Routes      []site.Route
	RoutesIssue string
	Tokens      []site.Token
	Knobs       []site.KnobState
	KnobsIssue  string
	Breakpoints []string
}

func (v View) Facts() Facts {
	p := v.Source()
	facts := Facts{Routes: []site.Route{}, Tokens: site.ThemeTokens(p), Knobs: []site.KnobState{}, Breakpoints: site.Breakpoints()}
	raw, _ := p.Content(site.ManifestPath)
	if manifest, err := site.DecodeManifest(p.Stack, []byte(raw)); err == nil {
		facts.Routes = manifest.Routes
	} else {
		facts.RoutesIssue = site.CodeManifestInvalid
	}
	if knobs, err := site.KnobStates(p); err == nil {
		facts.Knobs = knobs
	} else {
		facts.KnobsIssue = site.CodeKnobsInvalid
	}
	return facts
}
