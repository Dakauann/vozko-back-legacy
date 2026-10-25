package container

import (
	"context"
	"log"

	"vozko/domain/shared/deps"
	uw "vozko/domain/unofficial_whatsapp"
	workspace_addon "vozko/domain/workspace/workspace_addon"
	uwuc "vozko/usecases/unofficial_whatsapp"
)

func (c *Container) uwEntitlementSourceHandle() *deps.Handle[uwuc.WorkspaceEntitlementReader] {
	if c.uwEntitlementSource == nil {
		c.uwEntitlementSource = deps.Tracked[uwuc.WorkspaceEntitlementReader](&c.handles, "unofficial whatsapp workspace entitlements")
	}
	return c.uwEntitlementSource
}

type lateWorkspaceEntitlements struct {
	handle *deps.Handle[uwuc.WorkspaceEntitlementReader]
}

func (l lateWorkspaceEntitlements) Execute(workspaceID string) ([]workspace_addon.WorkspaceEntitlement, error) {
	source, err := l.handle.Get()
	if err != nil {
		return nil, err
	}
	return source.Execute(workspaceID)
}

func (c *Container) uwConnectionListenerHandle() *deps.Handle[uwuc.ConnectionListener] {
	if c.uwConnectionListener == nil {
		c.uwConnectionListener = deps.Tracked[uwuc.ConnectionListener](&c.handles, "unofficial whatsapp connection listener")
	}
	return c.uwConnectionListener
}

type lateConnectionListener struct {
	handle *deps.Handle[uwuc.ConnectionListener]
}

func (l lateConnectionListener) InstanceConnected(ctx context.Context, instance *uw.Instance) {
	listener, err := l.handle.Get()
	if err != nil {
		log.Printf("[unofficial-whatsapp] instance %s connected but its history import was not started: %v", instance.ID, err)
		return
	}
	listener.InstanceConnected(ctx, instance)
}
