package workspace_department_repository

import (
	"context"
	"time"

	"vozko/domain/cache"
	workspace_department "vozko/domain/workspace/workspace_department"
	redisCache "vozko/infra/cache"
)

const (
	scopeVersionPrefix = "dept_scope"
	scopeCacheTTL      = 60 * time.Second
)

func NewScopeVersions(shared cache.SharedState) cache.Versions {
	return redisCache.NewSharedStateVersions(shared, scopeVersionPrefix)
}

type scopeCachedRepository struct {
	workspace_department.Repository
	memo     cache.Memo
	versions cache.Versions
}

func NewScopeCachedRepository(inner workspace_department.Repository, memo cache.Memo, versions cache.Versions) workspace_department.Repository {
	return &scopeCachedRepository{Repository: inner, memo: memo, versions: versions}
}

func (r *scopeCachedRepository) HasDepartments(workspaceID string) (bool, error) {
	read := func(context.Context) (bool, error) {
		return r.Repository.HasDepartments(workspaceID)
	}
	key, ok := r.scopeKey(workspaceID, "has")
	if !ok {
		return read(context.Background())
	}
	return cache.Remember(context.Background(), r.memo, key, scopeCacheTTL, read)
}

func (r *scopeCachedRepository) GetMemberDepartmentIDs(workspaceID, userID string) ([]string, error) {
	read := func(context.Context) ([]string, error) {
		return r.Repository.GetMemberDepartmentIDs(workspaceID, userID)
	}
	if userID == "" {
		return read(context.Background())
	}
	key, ok := r.scopeKey(workspaceID, "member:"+userID)
	if !ok {
		return read(context.Background())
	}
	return cache.Remember(context.Background(), r.memo, key, scopeCacheTTL, read)
}

func (r *scopeCachedRepository) scopeKey(workspaceID, subject string) (string, bool) {
	if workspaceID == "" {
		return "", false
	}
	version, err := r.versions.Version(workspaceID)
	if err != nil {
		return "", false
	}
	return scopeVersionPrefix + ":" + workspaceID + ":" + version + ":" + subject, true
}
