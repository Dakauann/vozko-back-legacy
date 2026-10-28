package conversationad

import (
	"context"
	"log"
	"strings"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

type EntryWorkspaces interface {
	GetEntryWorkspaceID(entryID, entryType string) (string, error)
}

type InboundCounter interface {
	CountInboundByEntry(entryID string, entryType shared.EntryType) (int64, error)
}

type PageRecorder struct {
	origins    conversation.PageOriginRepository
	references conversation.PageReferences
	entries    EntryWorkspaces
	inbound    InboundCounter
	now        func() time.Time
}

func NewPageRecorder(origins conversation.PageOriginRepository, references conversation.PageReferences, entries EntryWorkspaces, inbound InboundCounter, now func() time.Time) *PageRecorder {
	return &PageRecorder{origins: origins, references: references, entries: entries, inbound: inbound, now: now}
}

func (r *PageRecorder) Record(ctx context.Context, record conversation.MessageHistoryRecord) {
	entryID := strings.TrimSpace(record.GetEntryID())
	if !record.SentBy.IsContact() || entryID == "" {
		return
	}
	code, ok := conversation.FindPageReference(record.Text)
	if !ok {
		return
	}
	count, err := r.inbound.CountInboundByEntry(entryID, record.EntryType)
	if err != nil || count != 1 {
		return
	}
	existing, err := r.origins.Get(ctx, entryID, record.EntryType)
	if err != nil {
		log.Printf("[page-origin] lookup for %s:%s failed: %v", record.EntryType, entryID, err)
		return
	}
	if existing != nil {
		return
	}
	target, err := r.references.Target(ctx, code)
	if err != nil {
		log.Printf("[page-origin] reference %s for %s:%s unreadable: %v", code, record.EntryType, entryID, err)
		return
	}
	if target == nil {
		return
	}
	workspaceID, err := r.entries.GetEntryWorkspaceID(entryID, string(record.EntryType))
	if err != nil || workspaceID != target.WorkspaceID {
		return
	}
	origin := conversation.PageOrigin{
		EntryID: entryID, EntryType: record.EntryType, WorkspaceID: target.WorkspaceID,
		ProjectID: target.ProjectID, Route: target.Route, Reference: code, ArrivedAt: r.now().UTC(),
	}
	if _, err := r.origins.Claim(ctx, origin); err != nil {
		log.Printf("[page-origin] %s:%s not recorded: %v", record.EntryType, entryID, err)
	}
}
