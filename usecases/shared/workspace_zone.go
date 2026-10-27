package shared

import (
	"context"
	"log"
	"strings"
	"time"
)

type WorkspaceZones interface {
	Location(ctx context.Context, workspaceID, departmentID string) (*time.Location, error)
}

func WorkspaceZone(ctx context.Context, zones WorkspaceZones, workspaceID string) *time.Location {
	if zones == nil || strings.TrimSpace(workspaceID) == "" {
		return time.UTC
	}
	loc, err := zones.Location(ctx, workspaceID, "")
	if err != nil {
		log.Printf("[workspace-zone] ws=%s the time zone could not be read, speaking UTC: %v", workspaceID, err)
		return time.UTC
	}
	if loc == nil {
		return time.UTC
	}
	return loc
}
