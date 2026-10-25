package queue

import (
	"fmt"
	"strings"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

const integrationExchange = "vozko_test_exchange"

func integrationQueue(t *testing.T, pool *ConnectionPool, topic string, args amqp.Table) *amqp.Channel {
	t.Helper()
	ch, release, err := pool.OpenChannel()
	if err != nil {
		t.Fatalf("open channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ch.QueueDelete(topic, false, false, false)
		_ = ch.Close()
		release()
	})
	_, _ = ch.QueueDelete(topic, false, false, false)
	if _, err := ch.QueueDeclare(topic, true, false, false, false, args); err != nil {
		t.Fatalf("declare %s: %v", topic, err)
	}
	if err := ch.QueueBind(topic, topic, integrationExchange, false, nil); err != nil {
		t.Fatalf("bind %s: %v", topic, err)
	}
	return ch
}

func TestIntegration_PublishBatchKeepsOrderAndHeaders(t *testing.T) {
	pool := integrationBroker(t)
	topic := "vozko.test.publish.batch.order"
	ch := integrationQueue(t, pool, topic, nil)
	pub := NewRabbitMQQueuePub(pool, integrationExchange).(*RabbitMQQueuePub)

	total := publishBatchWindow*2 + 7
	messages := make([][]byte, 0, total)
	for i := 0; i < total; i++ {
		messages = append(messages, []byte(fmt.Sprintf(`{"n":%d}`, i)))
	}

	if err := pub.PublishBatch(topic, messages); err != nil {
		t.Fatalf("publish batch: %v", err)
	}

	for i := 0; i < total; i++ {
		delivery, ok, err := ch.Get(topic, true)
		if err != nil || !ok {
			t.Fatalf("message %d missing: ok=%v err=%v", i, ok, err)
		}
		if string(delivery.Body) != string(messages[i]) {
			t.Fatalf("message %d = %s, want %s", i, delivery.Body, messages[i])
		}
		if delivery.ContentType != "application/json" || delivery.DeliveryMode != amqp.Persistent || delivery.Timestamp.IsZero() {
			t.Fatalf("message %d headers = (%q, %d, %v), want a persistent json message with a timestamp", i, delivery.ContentType, delivery.DeliveryMode, delivery.Timestamp)
		}
	}
	if _, ok, _ := ch.Get(topic, true); ok {
		t.Fatal("the batch published more messages than it was given")
	}
}

func TestIntegration_PublishBatchFailsWhenTheBrokerNacks(t *testing.T) {
	pool := integrationBroker(t)
	topic := "vozko.test.publish.batch.nack"
	integrationQueue(t, pool, topic, amqp.Table{"x-max-length": int32(1), "x-overflow": "reject-publish"})
	pub := NewRabbitMQQueuePub(pool, integrationExchange).(*RabbitMQQueuePub)
	pub.boundTopics[topic] = struct{}{}

	err := pub.PublishBatch(topic, [][]byte{[]byte(`{"n":0}`), []byte(`{"n":1}`), []byte(`{"n":2}`)})

	if err == nil || !strings.Contains(err.Error(), "nacked") {
		t.Fatalf("err = %v, want the broker nack to fail the batch", err)
	}

	recovered := "vozko.test.publish.batch.after.nack"
	integrationQueue(t, pool, recovered, nil)
	if err := pub.Publish(recovered, []byte(`{"after":"nack"}`)); err != nil {
		t.Fatalf("the publisher did not recover after a nacked batch: %v", err)
	}
}
