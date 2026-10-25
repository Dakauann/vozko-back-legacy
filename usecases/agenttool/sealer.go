package agenttool_usecase

import (
	"context"
	"errors"
	"fmt"

	"vozko/domain/agenttool"
	"vozko/domain/workspacesecret"
)

const maxSecretNameAttempts = 50

var (
	ErrSecretStoreUnavailable = errors.New("agenttool: the tool config holds credentials and the workspace secret store is unavailable")
	ErrSecretNamesExhausted   = errors.New("agenttool: no free secret name for a tool credential")
)

type SecretStore interface {
	workspacesecret.Resolver
	Put(ctx context.Context, workspaceID, userID, name, value string) (workspacesecret.Summary, error)
}

type SealRequest struct {
	WorkspaceID string
	UserID      string
	AgentID     string
	ToolName    string
	Sensitive   []string
	Config      map[string]interface{}
	Previous    map[string]interface{}
}

type Sealer struct {
	store SecretStore
}

func NewSealer(store SecretStore) *Sealer {
	return &Sealer{store: store}
}

func (s *Sealer) Seal(ctx context.Context, req SealRequest) (map[string]interface{}, error) {
	config := agenttool.RestoreMasked(req.Sensitive, req.Config, req.Previous)
	creds := agenttool.PlainCredentials(req.Sensitive, config)
	if len(creds) == 0 {
		return config, nil
	}
	if s == nil || s.store == nil {
		return nil, ErrSecretStoreUnavailable
	}
	names := make([]string, len(creds))
	for i, c := range creds {
		name, err := s.place(ctx, req, c)
		if err != nil {
			return nil, fmt.Errorf("agenttool: sealing %s of %s: %w", c.Field(), req.ToolName, err)
		}
		names[i] = name
	}
	return agenttool.WithReferences(config, creds, names), nil
}

func (s *Sealer) place(ctx context.Context, req SealRequest, c agenttool.Credential) (string, error) {
	base := agenttool.SecretNameFor(req.AgentID, req.ToolName, c.Field())
	previous := agenttool.ValueAt(req.Previous, c.Param, c.Key)
	for attempt := 1; attempt <= maxSecretNameAttempts; attempt++ {
		name := base
		if attempt > 1 {
			name = agenttool.WithSuffix(base, attempt)
		}
		if previous == agenttool.Reference(name) {
			return name, s.put(ctx, req, name, c.Value)
		}
		existing, err := s.store.Resolve(ctx, req.WorkspaceID, []string{name})
		if err != nil {
			return "", err
		}
		current, taken := existing[name]
		if !taken {
			return name, s.put(ctx, req, name, c.Value)
		}
		if current == c.Value {
			return name, nil
		}
	}
	return "", ErrSecretNamesExhausted
}

func (s *Sealer) put(ctx context.Context, req SealRequest, name, value string) error {
	_, err := s.store.Put(ctx, req.WorkspaceID, req.UserID, name, value)
	return err
}
