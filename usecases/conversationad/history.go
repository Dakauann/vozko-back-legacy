package conversationad

import (
	"context"

	"vozko/domain/conversation"
)

type adAwareHistory struct {
	inner    conversation.MessageHistoryManager
	recorder *Recorder
	pages    *PageRecorder
}

func WithAdOrigins(inner conversation.MessageHistoryManager, recorder *Recorder, pages *PageRecorder) conversation.MessageHistoryManager {
	if inner == nil || (recorder == nil && pages == nil) {
		return inner
	}
	return &adAwareHistory{inner: inner, recorder: recorder, pages: pages}
}

func (h *adAwareHistory) Record(ctx context.Context, record conversation.MessageHistoryRecord) error {
	if err := h.inner.Record(ctx, record); err != nil {
		return err
	}
	if h.recorder != nil && record.AdReferral != nil && record.SentBy.IsContact() {
		h.recorder.Record(ctx, record.GetEntryID(), record.EntryType, record.AdReferral)
	}
	if h.pages != nil {
		h.pages.Record(ctx, record)
	}
	return nil
}
