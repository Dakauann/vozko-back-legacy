package messaging

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

type sequentialPub struct {
	topics   []string
	bodies   []string
	failFrom int
}

func (p *sequentialPub) Publish(topic string, message []byte) error {
	if p.failFrom > 0 && len(p.bodies)+1 >= p.failFrom {
		return errors.New("broker down")
	}
	p.topics = append(p.topics, topic)
	p.bodies = append(p.bodies, string(message))
	return nil
}

func (p *sequentialPub) PublishWithDelay(string, []byte, time.Duration) error { return nil }
func (p *sequentialPub) ValidateConnection() error                            { return nil }

type batchingPub struct {
	sequentialPub
	batches [][]string
	topic   string
	err     error
}

func (p *batchingPub) PublishBatch(topic string, messages [][]byte) error {
	p.topic = topic
	batch := make([]string, 0, len(messages))
	for _, m := range messages {
		batch = append(batch, string(m))
	}
	p.batches = append(p.batches, batch)
	return p.err
}

func TestPublishAllHandsTheWholeListToABatchPublisher(t *testing.T) {
	pub := &batchingPub{}

	if err := PublishAll(pub, "topic", [][]byte{[]byte("a"), []byte("b"), []byte("c")}); err != nil {
		t.Fatal(err)
	}

	if len(pub.batches) != 1 || !reflect.DeepEqual(pub.batches[0], []string{"a", "b", "c"}) || pub.topic != "topic" {
		t.Fatalf("batches = %v on %q, want one batch [a b c] on topic", pub.batches, pub.topic)
	}
	if len(pub.bodies) != 0 {
		t.Fatalf("also published %v one by one", pub.bodies)
	}
}

func TestPublishAllSurfacesABatchFailure(t *testing.T) {
	pub := &batchingPub{err: errors.New("nacked")}

	if err := PublishAll(pub, "topic", [][]byte{[]byte("a")}); err == nil {
		t.Fatal("the batch failure did not surface")
	}
}

func TestPublishAllPublishesInOrderWithoutBatching(t *testing.T) {
	pub := &sequentialPub{}

	if err := PublishAll(pub, "topic", [][]byte{[]byte("a"), []byte("b"), []byte("c")}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(pub.bodies, []string{"a", "b", "c"}) || !reflect.DeepEqual(pub.topics, []string{"topic", "topic", "topic"}) {
		t.Fatalf("published %v on %v, want [a b c] on topic", pub.bodies, pub.topics)
	}
}

func TestPublishAllStopsAtTheFirstFailureWithoutBatching(t *testing.T) {
	pub := &sequentialPub{failFrom: 2}

	if err := PublishAll(pub, "topic", [][]byte{[]byte("a"), []byte("b"), []byte("c")}); err == nil {
		t.Fatal("the publish failure did not surface")
	}
	if !reflect.DeepEqual(pub.bodies, []string{"a"}) {
		t.Fatalf("published %v, want only [a] before the failure", pub.bodies)
	}
}
