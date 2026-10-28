package billing_usecase

import "context"

type AccessHooks interface {
	OnAccessCut(ctx context.Context, workspaceID string) error
	OnAccessRestored(ctx context.Context, workspaceID string) error
}
