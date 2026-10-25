package inbox_assignment

import (
	"testing"
	"time"
)

type intervalLog struct {
	closed   []time.Time
	appended []AssignmentHistory
}

func (l *intervalLog) CloseOpen(_, _, _ string, endedAt time.Time) error {
	l.closed = append(l.closed, endedAt)
	return nil
}

func (l *intervalLog) Append(h *AssignmentHistory) error {
	l.appended = append(l.appended, *h)
	return nil
}

func TestAnIntervalClosesTheOpenOneAndOpensTheNextOwner(t *testing.T) {
	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	log := &intervalLog{}
	if err := RecordInterval(log, AssignmentHistory{WorkspaceID: "ws", EntryID: "e", EntryType: "whatsapp", AssignedActorID: "ana", StartedAt: at}); err != nil {
		t.Fatal(err)
	}
	if len(log.closed) != 1 || !log.closed[0].Equal(at) || len(log.appended) != 1 || log.appended[0].AssignedActorID != "ana" {
		t.Fatalf("closed = %v appended = %+v", log.closed, log.appended)
	}
}

func TestAnUnassignmentOnlyClosesTheOpenInterval(t *testing.T) {
	log := &intervalLog{}
	if err := RecordInterval(log, AssignmentHistory{WorkspaceID: "ws", EntryID: "e", EntryType: "whatsapp"}); err != nil {
		t.Fatal(err)
	}
	if len(log.closed) != 1 || len(log.appended) != 0 {
		t.Fatalf("closed = %v appended = %+v", log.closed, log.appended)
	}
}
