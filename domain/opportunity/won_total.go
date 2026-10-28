package opportunity

import (
	"context"
	"time"
)

type WonTotal struct {
	Currency   string
	Count      int64
	ValueCents int64
}

type WonOfLeadsReader interface {
	WonDealsOfLeads(ctx context.Context, workspaceID string, leadIDs []string, from, to time.Time, scope DealScope) ([]WonTotal, error)
}
