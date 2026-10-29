package sitepublish

import "time"

type PreviewGrant struct {
	WorkspaceID string
	ProjectID   string
	Origin      string
	Expires     time.Time
}
