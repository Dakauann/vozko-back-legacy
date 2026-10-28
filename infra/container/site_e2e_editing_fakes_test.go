package container

import (
	"context"
	"sync"
	"time"

	site_usecase "vozko/usecases/site"
)

func (m *e2eSiteStore) FileRevs(_ context.Context, workspaceID, projectID string, wants map[string][]uint64) (map[string]map[uint64]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.projects[projectID]; !ok || p.WorkspaceID != workspaceID {
		return nil, site_usecase.ErrProjectNotFound
	}
	return map[string]map[uint64]string{}, nil
}

type e2ePreviewCache struct {
	mu     sync.Mutex
	values map[string]string
}

func (c *e2ePreviewCache) GetString(key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values[key], nil
}

func (c *e2ePreviewCache) SetString(key, value string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.values == nil {
		c.values = map[string]string{}
	}
	c.values[key] = value
	return nil
}
