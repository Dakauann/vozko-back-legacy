package conversationad

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

type pageBook struct {
	rows    map[string]conversation.PageOrigin
	getErr  error
	claimed int
}

func (b *pageBook) Claim(_ context.Context, origin conversation.PageOrigin) (bool, error) {
	if b.rows == nil {
		b.rows = map[string]conversation.PageOrigin{}
	}
	key := origin.EntryID + string(origin.EntryType)
	if _, ok := b.rows[key]; ok {
		return false, nil
	}
	b.rows[key] = origin
	b.claimed++
	return true, nil
}

func (b *pageBook) Get(_ context.Context, entryID string, entryType shared.EntryType) (*conversation.PageOrigin, error) {
	if b.getErr != nil {
		return nil, b.getErr
	}
	if row, ok := b.rows[entryID+string(entryType)]; ok {
		return &row, nil
	}
	return nil, nil
}

type referenceBook map[string]conversation.PageReferenceTarget

func (r referenceBook) Target(_ context.Context, code string) (*conversation.PageReferenceTarget, error) {
	if code == "BROKE" {
		return nil, errors.New("db down")
	}
	if target, ok := r[code]; ok {
		return &target, nil
	}
	return nil, nil
}

type entryWorkspaces map[string]string

func (e entryWorkspaces) GetEntryWorkspaceID(entryID, _ string) (string, error) {
	if ws, ok := e[entryID]; ok {
		return ws, nil
	}
	return "", errors.New("entry not found")
}

type inboundCount struct {
	n   int64
	err error
}

func (c inboundCount) CountInboundByEntry(string, shared.EntryType) (int64, error) {
	return c.n, c.err
}

var pageNow = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func pageRecorder(book *pageBook, count inboundCount) *PageRecorder {
	refs := referenceBook{"LP7K2": {WorkspaceID: "w1", ProjectID: "p1", Route: "/oferta", NumberRef: "n1"}}
	return NewPageRecorder(book, refs, entryWorkspaces{"e1": "w1", "e2": "w2"}, count, func() time.Time { return pageNow })
}

func contactSays(entryID, text string) conversation.MessageHistoryRecord {
	return conversation.MessageHistoryRecord{SentBy: conversation.SentByContact("5584"), EntryID: entryID, EntryType: shared.EntryTypeWhatsApp, Text: text}
}

func TestTheFirstMessageWithAPageReferenceRecordsThePageOrigin(t *testing.T) {
	book := &pageBook{}
	pageRecorder(book, inboundCount{n: 1}).Record(context.Background(), contactSays("e1", "Quero saber mais (ref LP7K2)"))
	got, _ := book.Get(context.Background(), "e1", shared.EntryTypeWhatsApp)
	want := conversation.PageOrigin{EntryID: "e1", EntryType: shared.EntryTypeWhatsApp, WorkspaceID: "w1", ProjectID: "p1", Route: "/oferta", Reference: "LP7K2", ArrivedAt: pageNow}
	if got == nil || *got != want {
		t.Fatalf("origin = %+v, want %+v", got, want)
	}
}

func TestAPageOriginIsOnlyRecordedWhenEverythingChecksOut(t *testing.T) {
	cases := map[string]struct {
		record conversation.MessageHistoryRecord
		count  inboundCount
	}{
		"a later message":           {contactSays("e1", "(ref LP7K2)"), inboundCount{n: 2}},
		"count unreadable":          {contactSays("e1", "(ref LP7K2)"), inboundCount{err: errors.New("db down")}},
		"no reference":              {contactSays("e1", "Oi"), inboundCount{n: 1}},
		"unknown reference":         {contactSays("e1", "(ref ZZZZZ)"), inboundCount{n: 1}},
		"reference lookup failed":   {contactSays("e1", "(ref BROKE)"), inboundCount{n: 1}},
		"another workspace's page":  {contactSays("e2", "(ref LP7K2)"), inboundCount{n: 1}},
		"entry workspace unknown":   {contactSays("e9", "(ref LP7K2)"), inboundCount{n: 1}},
		"sent by the business":      {conversation.MessageHistoryRecord{SentBy: conversation.SentByPerson("u1"), EntryID: "e1", EntryType: shared.EntryTypeWhatsApp, Text: "(ref LP7K2)"}, inboundCount{n: 1}},
		"no conversation reference": {contactSays("", "(ref LP7K2)"), inboundCount{n: 1}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			book := &pageBook{}
			pageRecorder(book, tc.count).Record(context.Background(), tc.record)
			if book.claimed != 0 {
				t.Fatalf("recorded %+v", book.rows)
			}
		})
	}
}

func TestAnExistingPageOriginIsNeverReplaced(t *testing.T) {
	book := &pageBook{rows: map[string]conversation.PageOrigin{"e1whatsapp": {EntryID: "e1", Reference: "AAAAA"}}}
	pageRecorder(book, inboundCount{n: 1}).Record(context.Background(), contactSays("e1", "(ref LP7K2)"))
	if book.rows["e1whatsapp"].Reference != "AAAAA" || book.claimed != 0 {
		t.Fatalf("rows = %+v", book.rows)
	}
	failing := &pageBook{getErr: errors.New("db down")}
	pageRecorder(failing, inboundCount{n: 1}).Record(context.Background(), contactSays("e1", "(ref LP7K2)"))
	if failing.claimed != 0 {
		t.Fatal("an unreadable origin must not be claimed")
	}
}

func TestTheHistoryRecordsThePageOriginAfterTheMessageIsSaved(t *testing.T) {
	inner, book := &recordedHistory{}, &pageBook{}
	history := WithAdOrigins(inner, recorderWith(&originBook{}, &mediaShelf{}, &fetcher{}, &broadcasts{}), pageRecorder(book, inboundCount{n: 1}))
	if err := history.Record(context.Background(), contactSays("e1", "Oi (ref LP7K2)")); err != nil || len(inner.records) != 1 {
		t.Fatalf("err=%v records=%d", err, len(inner.records))
	}
	if book.claimed != 1 || inner.records[0].Text != "Oi (ref LP7K2)" {
		t.Fatalf("claimed=%d stored text=%q", book.claimed, inner.records[0].Text)
	}
	failed := &pageBook{}
	_ = WithAdOrigins(&recordedHistory{fail: errors.New("db down")}, nil, pageRecorder(failed, inboundCount{n: 1})).Record(context.Background(), contactSays("e1", "(ref LP7K2)"))
	if failed.claimed != 0 {
		t.Fatal("no page origin without the message")
	}
}
