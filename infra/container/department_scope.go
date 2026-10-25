package container

import (
	"time"

	domainCache "vozko/domain/cache"
	workspace_department_domain "vozko/domain/workspace/workspace_department"
	redisCache "vozko/infra/cache"
	workspace_department_repository "vozko/infra/repositories/workspace_department"
)

const departmentScopeComputeTimeout = 5 * time.Second

func (c *Container) departmentScopeVersions() domainCache.Versions {
	return workspace_department_repository.NewScopeVersions(c.redisProvider.SharedState())
}

func (c *Container) cachedDepartmentScope() workspace_department_domain.Repository {
	return workspace_department_repository.NewScopeCachedRepository(
		c.repositories.workspaceDepartment,
		redisCache.NewSharedStateMemo(c.redisProvider.SharedState(), departmentScopeComputeTimeout),
		c.departmentScopeVersions(),
	)
}
