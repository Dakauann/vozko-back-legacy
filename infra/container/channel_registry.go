package container

import (
	"context"
	"log"

	conversation_domain "vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	conversation_usecase "vozko/usecases/conversation"
)

type channelConversations interface {
	conversation_usecase.EntryOwnerResolver
	ListEntryIDsByWorkspace(ctx context.Context, workspaceID string) ([]string, error)
	SetAutomationEnabled(ctx context.Context, entryID string, enabled *bool) error
	StatusForEntry(ctx context.Context, entryID string) (string, error)
	SetStatus(ctx context.Context, entryID string, write conversation_domain.StatusWrite) error
	CountByStatus(ctx context.Context, workspaceID, accountID string) (map[string]int64, error)
}

type conversationChannel struct {
	EntryType     shared.EntryType
	Adapter       conversation_domain.ChannelAdapter
	Conversations channelConversations
	Automation    func(ctx context.Context, entryID string) (*bool, error)
	Contacts      conversation_usecase.ContactIdentityLookup
}

type channelStatusStore struct{ conversations channelConversations }

func (s channelStatusStore) Status(ctx context.Context, entryID string) (string, error) {
	return s.conversations.StatusForEntry(ctx, entryID)
}

func (s channelStatusStore) SetStatus(ctx context.Context, entryID string, write conversation_domain.StatusWrite) error {
	return s.conversations.SetStatus(ctx, entryID, write)
}

type channelHistoryPorts interface {
	SetAutomationReader(shared.EntryType, func(context.Context, string) (*bool, error))
	SetContactIdentityLookup(shared.EntryType, conversation_usecase.ContactIdentityLookup)
}

type channelOwnerPorts interface {
	SetEntryOwnerResolver(shared.EntryType, conversation_usecase.EntryOwnerResolver)
}

func (c *Container) registerConversationChannel(ch conversationChannel) {
	if err := deps.Require("conversation channel "+string(ch.EntryType), map[string]any{
		"adapter":       ch.Adapter,
		"conversations": ch.Conversations,
		"automation":    ch.Automation,
		"contacts":      ch.Contacts,
	}); err != nil {
		log.Fatalf("[channels] %v", err)
	}
	if c.channelsSealed {
		log.Fatalf("[channels] %s registered after the channel table was sealed", ch.EntryType)
	}
	if c.registeredChannels == nil {
		c.registeredChannels = map[shared.EntryType]bool{}
	}
	if c.registeredChannels[ch.EntryType] {
		log.Fatalf("[channels] %s registered twice", ch.EntryType)
	}
	history, ok := c.services.conversationHistory.(channelHistoryPorts)
	if !ok {
		log.Fatalf("[channels] the history provider cannot take channel automation readers and contact lookups")
	}
	owners, ok := c.services.campaignWorkspaceResolver.(channelOwnerPorts)
	if !ok {
		log.Fatalf("[channels] the workspace resolver cannot take channel owner resolvers")
	}
	c.registerChannelAdapter(ch.Adapter)
	history.SetAutomationReader(ch.EntryType, ch.Automation)
	history.SetContactIdentityLookup(ch.EntryType, ch.Contacts)
	owners.SetEntryOwnerResolver(ch.EntryType, ch.Conversations)
	c.services.conversationAutomation.Register(ch.EntryType, ch.Conversations.SetAutomationEnabled)
	c.services.conversationAuthImpl.SetEntryAccessRepo(ch.EntryType, ch.Conversations)
	c.services.conversationStatusService.SetConversationStatusStore(ch.EntryType, channelStatusStore{conversations: ch.Conversations})
	c.services.conversationStatusService.SetConversationCounter(ch.EntryType, ch.Conversations.CountByStatus)
	c.registeredChannels[ch.EntryType] = true
}

func (c *Container) sealConversationChannels() {
	c.channelsSealed = true
}
