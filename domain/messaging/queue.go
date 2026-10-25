package messaging

import "time"

const MaxRetries = 3

const DelayQueueSuffix = ".delay"

const DLQSuffix = ".dlq"

type MessageAck interface {
	Ack() error
	Nack(requeue bool) error

	DeliveryCount() int
}

type MessageQueueSub interface {
	Subscribe(topic string, handler func(message []byte, ack MessageAck)) error
	DeleteQueue(topic string) error
	ValidateConnection() error

	GetQueueLength(topic string) (int, error)
}

type MessageQueuePub interface {
	Publish(topic string, message []byte) error

	PublishWithDelay(topic string, message []byte, delay time.Duration) error
	ValidateConnection() error
}

type BatchPublisher interface {
	PublishBatch(topic string, messages [][]byte) error
}

func PublishAll(pub MessageQueuePub, topic string, messages [][]byte) error {
	if batch, ok := pub.(BatchPublisher); ok {
		return batch.PublishBatch(topic, messages)
	}
	for _, message := range messages {
		if err := pub.Publish(topic, message); err != nil {
			return err
		}
	}
	return nil
}
