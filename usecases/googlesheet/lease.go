package googlesheet

import (
	"context"
	"errors"
	"log"

	"vozko/domain/cache"
)

func sheetLeaseKey(sheetID string) string { return "gs:sheet:" + sheetID }

func acquireSheet(ctx context.Context, leases cache.Locker, sheetID string) (release func(), busy bool, err error) {
	probe, cancel := context.WithTimeout(ctx, leaseProbe)
	held, err := leases.Acquire(probe, sheetLeaseKey(sheetID))
	cancel()
	if err != nil {
		if ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
			return nil, true, nil
		}
		return nil, false, err
	}
	return func() {
		if _, err := held(); err != nil {
			log.Printf("[googlesheet] lease of sheet %s not released, it expires on its own: %v", sheetID, err)
		}
	}, false, nil
}
