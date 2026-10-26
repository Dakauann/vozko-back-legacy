package conversation

import (
	"context"

	"vozko/domain/shared"
)

type SentMessageRef struct {
	EntryType         shared.EntryType
	EntryID           string
	ProviderMessageID string
	SentBy            SentBy
}

type InteractiveShaper func(SendInteractiveRequest) InteractiveContent

type InteractiveBackfill interface {
	AttachMissingInteractive(ctx context.Context, ref SentMessageRef, content InteractiveContent) (bool, error)
}
