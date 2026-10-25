package container

import (
	"log"

	"vozko/usecases/conversation/loopguard"
)

func (c *Container) loopGuard() loopguard.Guard {
	if c.services.loopGuard == nil {
		guard, err := loopguard.NewGuard(
			c.redisProvider.SharedState(),
			loopguard.DefaultConfig(),
			c.services.metrics,
		)
		if err != nil {
			log.Fatalf("[loop-guard] %v", err)
		}
		c.services.loopGuard = guard
	}
	return c.services.loopGuard
}
