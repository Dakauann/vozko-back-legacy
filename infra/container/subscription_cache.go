package container

import (
	workspace_plan_domain "vozko/domain/workspace/workspace_plan"
	redisCache "vozko/infra/cache"
	workspace_plan_repository "vozko/infra/repositories/workspace_plan"
)

func (c *Container) cachedSubscriptions() workspace_plan_domain.SubscriptionRepository {
	shared := c.redisProvider.SharedState()
	return workspace_plan_repository.NewCachedSubscriptionRepository(
		workspace_plan_repository.NewSubscriptionRepository(c.db),
		shared,
		redisCache.NewSharedStateVersions(shared, workspace_plan_repository.SubscriptionCachePrefix),
	)
}
