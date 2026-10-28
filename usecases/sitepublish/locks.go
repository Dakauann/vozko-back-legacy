package sitepublish_usecase

import (
	"fmt"
	"log"
	"time"
)

const (
	publishLockPrefix = "site:publish:lock:"
	PublishLockTTL    = 15 * time.Minute
)

func publishLockKey(projectID string) string {
	return publishLockPrefix + projectID
}

func (s *Service) exclusive(projectID string, fn func() error) error {
	key := publishLockKey(projectID)
	acquired, err := s.Locks.SetNX(key, s.NewID(), PublishLockTTL)
	if err != nil {
		return fmt.Errorf("sitepublish: the publish lock is unreadable: %w", err)
	}
	if !acquired {
		return ErrPublishInProgress
	}
	defer func() {
		if err := s.Locks.Del(key); err != nil {
			log.Printf("[sitepublish] project %s: the publish lock stays until it expires: %v", projectID, err)
		}
	}()
	return fn()
}
