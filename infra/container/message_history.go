package container

import (
	conversation_domain "vozko/domain/conversation"
	conversation_usecase "vozko/usecases/conversation"
	"vozko/usecases/conversationad"
)

func (c *Container) messageHistoryManager() conversation_domain.MessageHistoryManager {
	if c.services.messageHistory == nil {
		c.services.messageHistory = conversationad.WithAdOrigins(
			conversation_usecase.NewMessageHistoryManagerWithHub(c.repositories.conversation, c.services.messageArrivals),
			c.adOriginRecorder(),
		)
	}
	return c.services.messageHistory
}
