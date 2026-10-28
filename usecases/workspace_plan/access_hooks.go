package workspace_plan_usecase

import (
	"context"
	"log"

	billing_usecase "vozko/usecases/billing"
)

func restoreAccess(hooks billing_usecase.AccessHooks, workspaceID string) {
	if hooks == nil {
		return
	}
	if err := hooks.OnAccessRestored(context.Background(), workspaceID); err != nil {
		log.Printf("[subscriptions] workspace %s: access restore failed, the sites sweep retries it: %v", workspaceID, err)
	}
}

func cutAccess(hooks billing_usecase.AccessHooks, workspaceID string) {
	if hooks == nil {
		return
	}
	if err := hooks.OnAccessCut(context.Background(), workspaceID); err != nil {
		log.Printf("[subscriptions] workspace %s: access cut failed, the sites sweep retries it: %v", workspaceID, err)
	}
}
