package webchat

import (
	"context"
	"testing"

	"vozko/domain/conversation"
	wcdomain "vozko/domain/webchat"
)

type ownerAwareTranscript struct {
	assignments   *fakeAssignments
	ownedAtRecord []bool
}

func (t *ownerAwareTranscript) Record(_ context.Context, r conversation.MessageHistoryRecord) error {
	owned := false
	for _, entryID := range t.assignments.ensured {
		owned = owned || entryID == r.EntryID
	}
	t.ownedAtRecord = append(t.ownedAtRecord, owned)
	return nil
}

func (t *ownerAwareTranscript) everyRecordWasOwned() bool {
	for _, owned := range t.ownedAtRecord {
		if !owned {
			return false
		}
	}
	return len(t.ownedAtRecord) > 0
}

func TestAVisitorMessageReachesTheInboxOnlyAfterItsConversationHasAnOwner(t *testing.T) {
	h := newHarness(t, nil)
	transcript := &ownerAwareTranscript{assignments: h.assignments}
	h.svc.Transcript = transcript

	if _, err := h.svc.Send(context.Background(), h.session(t), SendInput{ClientMessageID: "client-0001", Text: "oi"}); err != nil {
		t.Fatal(err)
	}

	if !transcript.everyRecordWasOwned() {
		t.Fatalf("owned at record = %v, the message was broadcast before the conversation had an owner", transcript.ownedAtRecord)
	}
}

func TestAVisitorUploadReachesTheInboxOnlyAfterItsConversationHasAnOwner(t *testing.T) {
	h := newHarness(t, func(w *wcdomain.Widget) { w.AllowAttachments = true })
	transcript := &ownerAwareTranscript{assignments: h.assignments}
	h.svc.Transcript = transcript
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)

	if _, err := h.svc.Upload(context.Background(), h.session(t), UploadInput{ClientMessageID: "client-0001", FileName: "foto.png", Data: png}); err != nil {
		t.Fatal(err)
	}

	if !transcript.everyRecordWasOwned() {
		t.Fatalf("owned at record = %v, the upload was broadcast before the conversation had an owner", transcript.ownedAtRecord)
	}
}
