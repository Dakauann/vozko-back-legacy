package report_usecase

import (
	"errors"
	"testing"

	"vozko/domain/messaging"
	"vozko/domain/shared/deps"
)

type idleSubscriber struct{}

func (idleSubscriber) Subscribe(string, func([]byte, messaging.MessageAck)) error { return nil }
func (idleSubscriber) DeleteQueue(string) error                                   { return nil }
func (idleSubscriber) ValidateConnection() error                                  { return nil }
func (idleSubscriber) GetQueueLength(string) (int, error)                         { return 0, nil }

func reportWorker(service *Service) *Worker {
	worker, err := NewWorker(service, idleSubscriber{})
	if err != nil {
		panic(err)
	}
	return worker
}

func TestAReportWorkerRequiresItsSubscriber(t *testing.T) {
	service, _, _, _ := buildService(t, okRenderer())
	if worker, err := NewWorker(service, nil); worker != nil || !errors.Is(err, deps.ErrMissing) {
		t.Fatalf("worker = %v err = %v", worker, err)
	}
}
