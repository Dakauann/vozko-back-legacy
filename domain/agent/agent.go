package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"vozko/domain/testinput"
)

const maxAgentVariables = 50

var (
	ErrAgentNameRequired            = errors.New("agent name is required")
	ErrAgentNameTooLong             = errors.New("agent name must not exceed 120 characters")
	ErrAgentMessagingPromptRequired = errors.New("agent messaging prompt is required")
	ErrAgentMessagingModelRequired  = errors.New("agent messaging model is required")
	ErrAgentMessagingModelTooLong   = errors.New("agent messaging model must not exceed 64 characters")
	ErrAgentProviderRequired        = errors.New("agent provider is required")
	ErrAgentProviderInvalid         = errors.New("agent provider is invalid")
	ErrAgentProviderUnsupported     = errors.New("agent provider is unsupported")
	ErrAgentProviderUnavailable     = errors.New("agent provider is unavailable")
	ErrAgentNotFound                = errors.New("agent not found")
	ErrAgentMetadataInvalid         = errors.New("agent metadata must be valid JSON object")
	ErrAgentInternalToolInvalid     = errors.New("agent internal tool is invalid")
	ErrAgentInternalToolsRequired   = errors.New("agent internal tools are required")
	ErrAgentKnowledgeBaseNoAccess   = errors.New("knowledge base does not belong to this workspace")
	ErrAgentToolVisibilityInvalid   = errors.New("agent tool visibility is invalid")
	ErrAgentVariableNameInvalid     = errors.New("agent variable names must contain only letters, digits, and underscores, up to 64")
	ErrAgentVariableNameReserved    = errors.New("agent variables cannot use lead_name, lead_number or campaign_name, which the platform fills")
	ErrAgentVariableDuplicate       = errors.New("agent variable names must be unique")
	ErrAgentVariableTooMany         = errors.New("agent must not have more than 50 variables")
	ErrAgentRequiredVariableMissing = errors.New("required agent variable is missing from entry metadata")
)

type ToolVisibility string

const (
	ToolVisibilityMessaging ToolVisibility = "messaging"
)

func (v ToolVisibility) IsValid() bool {
	return v == ToolVisibilityMessaging
}

type ToolBinding struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name"`
	Function    string                 `json:"function,omitempty"`
	Description string                 `json:"description,omitempty"`
	Visibility  []ToolVisibility       `json:"visibility,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Revision    int                    `json:"revision,omitempty"`
	TargetKey   string                 `json:"-"`
	TestInputs  []testinput.TestInput  `json:"testInputs,omitempty"`
}

type AgentVariable struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	DefaultValue string `json:"defaultValue,omitempty"`
}

func (tb ToolBinding) IsVisibleIn(v ToolVisibility) bool {
	if len(tb.Visibility) == 0 {
		return true
	}
	for _, vis := range tb.Visibility {
		if vis == v {
			return true
		}
	}
	return false
}

func (tb ToolBinding) GetConfig(key string) (interface{}, bool) {
	if tb.Config == nil {
		return nil, false
	}
	val, ok := tb.Config[key]
	return val, ok
}

func (tb ToolBinding) GetConfigString(key string) string {
	if val, ok := tb.GetConfig(key); ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (tb ToolBinding) GetConfigMap(key string) map[string]interface{} {
	if val, ok := tb.GetConfig(key); ok {
		if m, ok := val.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

type AgentProvider string

const (
	AgentProviderAI AgentProvider = "platform"
)

func (p AgentProvider) IsValid() bool {
	switch p {
	case AgentProviderAI, "internal":
		return true
	default:
		return false
	}
}

type RAGConfig struct {
	MaxCharacters int `json:"maxCharacters,omitempty"`

	MaxChunks int `json:"maxChunks,omitempty"`

	MinScore float64 `json:"minScore,omitempty"`

	NumCandidates int `json:"numCandidates,omitempty"`

	ContextWindow int `json:"contextWindow,omitempty"`

	MaxChunksPerDoc int `json:"maxChunksPerDoc,omitempty"`
}

func (c *RAGConfig) WithDefaults() RAGConfig {
	out := RAGConfig{
		MaxCharacters:   10000,
		MaxChunks:       5,
		MinScore:        0.3,
		NumCandidates:   10,
		ContextWindow:   1,
		MaxChunksPerDoc: 3,
	}
	if c == nil {
		return out
	}
	if c.MaxCharacters > 0 {
		out.MaxCharacters = c.MaxCharacters
	}
	if c.MaxChunks > 0 {
		out.MaxChunks = c.MaxChunks
	}
	if c.MinScore > 0 {
		out.MinScore = c.MinScore
	}
	if c.NumCandidates > 0 {
		out.NumCandidates = c.NumCandidates
	}

	if c.ContextWindow > 0 {
		out.ContextWindow = c.ContextWindow
	}
	if c.MaxChunksPerDoc > 0 {
		out.MaxChunksPerDoc = c.MaxChunksPerDoc
	}
	return out
}

type Agent struct {
	ID               string   `json:"id"`
	WorkspaceID      string   `json:"workspaceId"`
	DepartmentID     string   `json:"departmentId,omitempty"`
	KnowledgeBaseIDs []string `json:"knowledgeBaseIds,omitempty"`
	MCPCollectionIDs []string `json:"mcpCollectionIds,omitempty"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`

	MessagingPrompt string            `json:"messagingPrompt"`
	MessagingModel  string            `json:"messagingModel"`
	AvatarURL       string            `json:"avatarUrl"`
	Tags            []string          `json:"tags"`
	Metadata        map[string]string `json:"metadata"`
	InternalTools   []ToolBinding     `json:"internalTools"`
	Provider        AgentProvider     `json:"provider"`
	ExternalID      string            `json:"externalId"`
	RAGEnabled      bool              `json:"ragEnabled"`
	RAGConfig       *RAGConfig        `json:"ragConfig,omitempty"`
	IsActive        bool              `json:"isActive"`
	Archived        bool              `json:"archived"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
	MediaIDs        []string          `json:"mediaIds"`
	Variables       []AgentVariable   `json:"variables,omitempty"`
	ChangedBy       string            `json:"-"`
	ExpectedTools   map[string]int    `json:"-"`
	RunningVersion  int               `json:"-"`
}

func (a *Agent) Normalize() {
	a.DepartmentID = strings.TrimSpace(a.DepartmentID)
	a.Name = strings.TrimSpace(a.Name)
	a.Description = strings.TrimSpace(a.Description)
	a.MessagingPrompt = strings.TrimSpace(a.MessagingPrompt)
	a.MessagingModel = strings.TrimSpace(a.MessagingModel)
	a.AvatarURL = strings.TrimSpace(a.AvatarURL)
	a.ExternalID = strings.TrimSpace(a.ExternalID)

	prov := strings.TrimSpace(strings.ToLower(string(a.Provider)))
	a.Provider = AgentProvider(prov)

	if len(a.Tags) > 0 {
		seen := make(map[string]struct{}, len(a.Tags))
		uniq := make([]string, 0, len(a.Tags))
		for _, tag := range a.Tags {
			t := strings.TrimSpace(strings.ToLower(tag))
			if t == "" {
				continue
			}
			if _, exists := seen[t]; exists {
				continue
			}
			seen[t] = struct{}{}
			uniq = append(uniq, t)
		}
		a.Tags = uniq
	}

	if len(a.InternalTools) > 0 {
		seen := make(map[string]struct{}, len(a.InternalTools))
		uniq := make([]ToolBinding, 0, len(a.InternalTools))
		for _, binding := range a.InternalTools {
			t := strings.TrimSpace(strings.ToLower(binding.Name))
			if t == "" {
				continue
			}
			key := bindingKey(t, binding)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}

			normalizedVisibility := make([]ToolVisibility, 0, len(binding.Visibility))
			for _, v := range binding.Visibility {
				vNorm := ToolVisibility(strings.TrimSpace(strings.ToLower(string(v))))
				if vNorm.IsValid() {
					normalizedVisibility = append(normalizedVisibility, vNorm)
				}
			}
			uniq = append(uniq, ToolBinding{
				ID:          strings.TrimSpace(binding.ID),
				Name:        t,
				Function:    strings.TrimSpace(binding.Function),
				Description: strings.TrimSpace(binding.Description),
				Visibility:  normalizedVisibility,
				Config:      binding.Config,
				Revision:    binding.Revision,
				TestInputs:  binding.TestInputs,
			})
		}
		a.InternalTools = uniq
	} else if a.InternalTools != nil && len(a.InternalTools) == 0 {
		a.InternalTools = []ToolBinding{}
	}

	if a.Metadata == nil {
		a.Metadata = make(map[string]string)
	}

	if a.Variables != nil {
		trimmed := make([]AgentVariable, len(a.Variables))
		for i, v := range a.Variables {
			trimmed[i] = AgentVariable{
				Name:         strings.TrimSpace(v.Name),
				Description:  strings.TrimSpace(v.Description),
				DefaultValue: strings.TrimSpace(v.DefaultValue),
			}
		}
		a.Variables = trimmed
	}
}

func bindingConfigKey(config map[string]interface{}) string {
	if len(config) == 0 {
		return ""
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func (a *Agent) Validate() error {
	if a.Name == "" {
		return ErrAgentNameRequired
	}
	if len(a.Name) > 120 {
		return ErrAgentNameTooLong
	}

	if a.MessagingPrompt == "" {
		return ErrAgentMessagingPromptRequired
	}
	if a.Provider == "" {
		return ErrAgentProviderRequired
	}
	if !a.Provider.IsValid() {
		return ErrAgentProviderInvalid
	}
	if a.MessagingModel == "" {
		return ErrAgentMessagingModelRequired
	}
	if len(a.MessagingModel) > 64 {
		return ErrAgentMessagingModelTooLong
	}
	if a.Metadata != nil {
		if _, err := json.Marshal(a.Metadata); err != nil {
			return ErrAgentMetadataInvalid
		}
	}
	if a.InternalTools == nil {
		return ErrAgentInternalToolsRequired
	}
	if len(a.Variables) > maxAgentVariables {
		return ErrAgentVariableTooMany
	}
	return firstVariableProblem(a.Variables)
}

func (a *Agent) HasTool(toolName string) bool {
	if a == nil {
		return false
	}
	for _, tb := range a.InternalTools {
		if tb.Name == toolName {
			return true
		}
	}
	return false
}

func (a *Agent) HasKnowledgeBases() bool {
	return a != nil && len(a.KnowledgeBaseIDs) > 0
}

func (a *Agent) IsRAGEnabled() bool {
	return a != nil && a.RAGEnabled && len(a.KnowledgeBaseIDs) > 0
}

func (a *Agent) AddKnowledgeBase(kbID string) {
	if a == nil || kbID == "" {
		return
	}
	for _, id := range a.KnowledgeBaseIDs {
		if id == kbID {
			return
		}
	}
	a.KnowledgeBaseIDs = append(a.KnowledgeBaseIDs, kbID)
}

func (a *Agent) RemoveKnowledgeBase(kbID string) {
	if a == nil || kbID == "" {
		return
	}
	filtered := make([]string, 0, len(a.KnowledgeBaseIDs))
	for _, id := range a.KnowledgeBaseIDs {
		if id != kbID {
			filtered = append(filtered, id)
		}
	}
	a.KnowledgeBaseIDs = filtered
}

func (a *Agent) HasKnowledgeBase(kbID string) bool {
	if a == nil {
		return false
	}
	for _, id := range a.KnowledgeBaseIDs {
		if id == kbID {
			return true
		}
	}
	return false
}

func (a *Agent) HasMCPCollections() bool {
	return a != nil && len(a.MCPCollectionIDs) > 0
}

func (a *Agent) HasMCPCollection(id string) bool {
	if a == nil {
		return false
	}
	for _, cid := range a.MCPCollectionIDs {
		if cid == id {
			return true
		}
	}
	return false
}

func bindingKey(toolType string, binding ToolBinding) string {
	if id := strings.TrimSpace(binding.ID); id != "" {
		return "id\x00" + id
	}
	return toolType + "\x00" + strings.TrimSpace(binding.Function) + "\x00" + bindingConfigKey(binding.Config)
}
