package ws

import (
	"sync"
	"time"
)

type trailingCoalescer struct {
	pending sync.Map
}

func (c *trailingCoalescer) schedule(key string, window time.Duration, run func()) {
	if _, waiting := c.pending.LoadOrStore(key, struct{}{}); waiting {
		return
	}
	time.AfterFunc(window, func() {
		c.pending.Delete(key)
		run()
	})
}
