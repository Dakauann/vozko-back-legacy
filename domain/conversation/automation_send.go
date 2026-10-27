package conversation

import (
	"context"
	"errors"

	"vozko/domain/actor"
)

var (
	ErrAutomationAuthorRequired = errors.New("conversation: an automation send needs an agent or a workflow as its author")
	ErrRecipientUnreadable      = errors.New("conversation: the contact of this conversation could not be read, so its send rules cannot be checked")
)

type AutomationSendInput struct {
	EntryID     string
	EntryType   string
	WorkspaceID string
	Author      string
	Text        string
}

type AutomationSendUseCase interface {
	Execute(ctx context.Context, in AutomationSendInput) (*Message, error)
}

func SentByAutomation(actorID string) (SentBy, bool) {
	switch {
	case actor.IsAI(actorID) && actor.ParseAI(actorID) != "":
		return SentByAI(actor.ParseAI(actorID)), true
	case actor.IsWorkflow(actorID) && actor.ParseWorkflow(actorID) != "":
		return SentByWorkflow(actor.ParseWorkflow(actorID)), true
	}
	return SentBy{}, false
}

func (o SendOrigin) Author() SentBy {
	if o.Automation.Valid() {
		return o.Automation
	}
	return SentByPerson(o.SenderUserID)
}

func (o SendOrigin) ByPerson() bool {
	return o.Author().Kind() == SenderHuman
}

func (o SendOrigin) MessageType() MessageType {
	if o.ByPerson() {
		return MessageTypeOperator
	}
	return MessageTypeAIResponse
}
