package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

var (
	ErrAgentNotPublished   = errors.New("agent has no published version")
	ErrAgentVersionMissing = errors.New("agent version not found")
)

type ToolPin struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
}

type Snapshot struct {
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	MessagingPrompt  string            `json:"messagingPrompt"`
	MessagingModel   string            `json:"messagingModel"`
	AvatarURL        string            `json:"avatarUrl"`
	Tags             []string          `json:"tags"`
	Metadata         map[string]string `json:"metadata"`
	KnowledgeBaseIDs []string          `json:"knowledgeBaseIds"`
	MCPCollectionIDs []string          `json:"mcpCollectionIds"`
	RAGEnabled       bool              `json:"ragEnabled"`
	RAGConfig        *RAGConfig        `json:"ragConfig,omitempty"`
	MediaIDs         []string          `json:"mediaIds"`
	Variables        []AgentVariable   `json:"variables"`
	Tools            []ToolPin         `json:"tools"`
}

type Version struct {
	AgentID     string    `json:"agentId"`
	Version     int       `json:"version"`
	Snapshot    Snapshot  `json:"snapshot"`
	PublishedBy string    `json:"publishedBy"`
	PublishedAt time.Time `json:"publishedAt"`
}

type Publication struct {
	PublishedVersion *int       `json:"publishedVersion"`
	PublishedAt      *time.Time `json:"publishedAt,omitempty"`
	PublishedBy      string     `json:"publishedBy,omitempty"`
	HasDraftChanges  bool       `json:"hasUnpublishedChanges"`
}

func SnapshotOf(a *Agent) Snapshot {
	s := Snapshot{
		Name:             a.Name,
		Description:      a.Description,
		MessagingPrompt:  a.MessagingPrompt,
		MessagingModel:   a.MessagingModel,
		AvatarURL:        a.AvatarURL,
		Tags:             nonNilStrings(a.Tags),
		Metadata:         a.Metadata,
		KnowledgeBaseIDs: nonNilStrings(a.KnowledgeBaseIDs),
		MCPCollectionIDs: nonNilStrings(a.MCPCollectionIDs),
		RAGEnabled:       a.RAGEnabled,
		RAGConfig:        a.RAGConfig,
		MediaIDs:         nonNilStrings(a.MediaIDs),
		Variables:        a.Variables,
		Tools:            make([]ToolPin, 0, len(a.InternalTools)),
	}
	if s.Metadata == nil {
		s.Metadata = map[string]string{}
	}
	if s.Variables == nil {
		s.Variables = []AgentVariable{}
	}
	for _, t := range a.InternalTools {
		s.Tools = append(s.Tools, ToolPin{ID: t.ID, Revision: t.Revision})
	}
	return s
}

func (s Snapshot) Same(other Snapshot) bool {
	left, errLeft := json.Marshal(s)
	right, errRight := json.Marshal(other)
	return errLeft == nil && errRight == nil && reflect.DeepEqual(left, right)
}

func (s Snapshot) Apply(live *Agent, tools []ToolBinding) *Agent {
	out := *live
	out.Name = s.Name
	out.Description = s.Description
	out.MessagingPrompt = s.MessagingPrompt
	out.MessagingModel = s.MessagingModel
	out.AvatarURL = s.AvatarURL
	out.Tags = s.Tags
	out.Metadata = s.Metadata
	out.KnowledgeBaseIDs = s.KnowledgeBaseIDs
	out.MCPCollectionIDs = s.MCPCollectionIDs
	out.RAGEnabled = s.RAGEnabled
	out.RAGConfig = s.RAGConfig
	out.MediaIDs = s.MediaIDs
	out.Variables = s.Variables
	out.InternalTools = tools
	return &out
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

type PublishedReader interface {
	FindPublished(agentID string) (*Agent, error)
}

type VersionRepository interface {
	PublishVersion(ctx context.Context, agentID string, snapshot Snapshot, publishedBy string) (Version, error)
	CurrentVersion(ctx context.Context, agentID string) (*Version, error)
	Versions(ctx context.Context, agentID string) ([]Version, error)
	VersionOf(ctx context.Context, agentID string, version int) (Version, error)
}

type UnpublishedScanner interface {
	UnpublishedAgentIDs(ctx context.Context, afterID string, limit int) ([]string, error)
}
