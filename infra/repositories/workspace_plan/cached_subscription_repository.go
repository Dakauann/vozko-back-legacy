package workspace_plan_repository

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"vozko/domain/cache"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

const (
	SubscriptionCachePrefix = "ws:subscription"
	subscriptionCacheTTL    = 60 * time.Second
	storedTimePrecision     = time.Microsecond
)

type CachedSubscriptionRepository struct {
	workspace_plan.SubscriptionRepository
	shared   cache.SharedState
	versions cache.Versions
}

func NewCachedSubscriptionRepository(inner workspace_plan.SubscriptionRepository, shared cache.SharedState, versions cache.Versions) workspace_plan.SubscriptionRepository {
	return &CachedSubscriptionRepository{SubscriptionRepository: inner, shared: shared, versions: versions}
}

type currentSubscription struct {
	Subscription *workspace_plan.WorkspaceSubscription `json:"subscription,omitempty"`
	MissingSince *time.Time                            `json:"missingSince,omitempty"`
}

func currentSubscriptionKey(workspaceID, version string) string {
	return SubscriptionCachePrefix + ":current:" + workspaceID + ":" + version
}

func (c currentSubscription) covers(at time.Time) bool {
	return c.Subscription != nil || (c.MissingSince != nil && !at.Before(*c.MissingSince))
}

func (c currentSubscription) answer(at time.Time) (*workspace_plan.WorkspaceSubscription, error) {
	if c.Subscription != nil && c.Subscription.IsCurrent(at.Truncate(storedTimePrecision)) {
		return c.Subscription, nil
	}
	return nil, workspace_plan.ErrSubscriptionNotCurrent
}

func (r *CachedSubscriptionRepository) GetCurrentByWorkspaceID(workspaceID string, at time.Time) (*workspace_plan.WorkspaceSubscription, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return r.SubscriptionRepository.GetCurrentByWorkspaceID(workspaceID, at)
	}
	version, err := r.versions.Version(workspaceID)
	if err != nil {
		return r.SubscriptionRepository.GetCurrentByWorkspaceID(workspaceID, at)
	}
	key := currentSubscriptionKey(workspaceID, version)
	if cached, ok := r.read(key); ok && cached.covers(at) {
		return cached.answer(at)
	}
	sub, err := r.SubscriptionRepository.GetCurrentByWorkspaceID(workspaceID, at)
	switch {
	case err == nil && sub != nil:
		r.remember(key, currentSubscription{Subscription: sub})
	case errors.Is(err, workspace_plan.ErrSubscriptionNotCurrent):
		r.remember(key, currentSubscription{MissingSince: &at})
	}
	return sub, err
}

func (r *CachedSubscriptionRepository) read(key string) (currentSubscription, bool) {
	raw, err := r.shared.GetString(key)
	if err != nil || raw == "" {
		return currentSubscription{}, false
	}
	var cached currentSubscription
	if json.Unmarshal([]byte(raw), &cached) != nil {
		return currentSubscription{}, false
	}
	return cached, true
}

func (r *CachedSubscriptionRepository) remember(key string, value currentSubscription) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_ = r.shared.SetString(key, string(data), subscriptionCacheTTL)
}

func (r *CachedSubscriptionRepository) moved(workspaceIDs ...string) error {
	seen := make(map[string]bool, len(workspaceIDs))
	var errs []error
	for _, workspaceID := range workspaceIDs {
		if strings.TrimSpace(workspaceID) == "" || seen[workspaceID] {
			continue
		}
		seen[workspaceID] = true
		errs = append(errs, r.versions.Bump(workspaceID))
	}
	return errors.Join(errs...)
}

func (r *CachedSubscriptionRepository) Create(subscription *workspace_plan.WorkspaceSubscription) error {
	err := r.SubscriptionRepository.Create(subscription)
	if subscription == nil {
		return err
	}
	return errors.Join(err, r.moved(subscription.WorkspaceID))
}

func (r *CachedSubscriptionRepository) Update(subscription *workspace_plan.WorkspaceSubscription) error {
	err := r.SubscriptionRepository.Update(subscription)
	if subscription == nil {
		return err
	}
	return errors.Join(err, r.moved(subscription.WorkspaceID))
}

func (r *CachedSubscriptionRepository) ExpireOverdue(at time.Time, batchSize int) ([]string, error) {
	workspaceIDs, err := r.SubscriptionRepository.ExpireOverdue(at, batchSize)
	return workspaceIDs, errors.Join(err, r.moved(workspaceIDs...))
}
