package queue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeConfirm struct {
	acked bool
	hangs bool
}

func (f fakeConfirm) WaitContext(ctx context.Context) (bool, error) {
	if f.hangs {
		<-ctx.Done()
		return false, ctx.Err()
	}
	return f.acked, nil
}

func TestAwaitConfirms_EveryAckConfirmsTheBatch(t *testing.T) {
	pending := []pendingConfirm{fakeConfirm{acked: true}, fakeConfirm{acked: true}, fakeConfirm{acked: true}}

	if err := awaitConfirms("topic", pending, time.Second); err != nil {
		t.Fatalf("a fully acked batch failed: %v", err)
	}
}

func TestAwaitConfirms_ANackFailsTheBatch(t *testing.T) {
	pending := []pendingConfirm{fakeConfirm{acked: true}, fakeConfirm{acked: false}, fakeConfirm{acked: true}}

	err := awaitConfirms("topic", pending, time.Second)

	if err == nil || !strings.Contains(err.Error(), "nacked") {
		t.Fatalf("err = %v, want the nack to fail the batch", err)
	}
}

func TestAwaitConfirms_ATimeoutFailsTheBatchWithinOneDeadline(t *testing.T) {
	pending := []pendingConfirm{fakeConfirm{acked: true}, fakeConfirm{hangs: true}, fakeConfirm{hangs: true}}
	const timeout = 50 * time.Millisecond

	started := time.Now()
	err := awaitConfirms("topic", pending, timeout)
	elapsed := time.Since(started)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the confirm timeout", err)
	}
	if elapsed >= 2*timeout {
		t.Fatalf("waited %s, want one shared deadline of %s for the whole batch", elapsed, timeout)
	}
}

func TestPublishBatch_EmptyBatchTouchesNoChannel(t *testing.T) {
	h := newTestPub()

	if err := h.pub.PublishBatch("topic", nil); err != nil {
		t.Fatalf("an empty batch failed: %v", err)
	}
	if h.lastConnector() != nil {
		t.Fatal("an empty batch opened a connection")
	}
}
