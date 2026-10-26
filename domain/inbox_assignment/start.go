package inbox_assignment

import (
	"errors"
	"fmt"

	"vozko/domain/actor"
	"vozko/domain/conversation"
)

const CodeStarterNotAssigned = "starter_not_assigned"

var ErrStarterNotAssigned = errors.New("inbox assignment: the conversation could not be given to the person who started it")

func StarterNotAssigned(cause error) error {
	return fmt.Errorf("%w: %w", ErrStarterNotAssigned, cause)
}

type ConversationStart struct {
	WorkspaceID string
	EntryID     string
	EntryType   string
	AccountID   string
	StarterID   string
}

func (s ConversationStart) ByPerson() bool {
	return actor.IsPerson(s.StarterID)
}

type StartOwnership struct {
	Owner  string
	Takes  bool
	Pauses bool
}

func StartOwnershipFor(starter string, current *InboxAssignment, profile conversation.AutomationProfile) StartOwnership {
	owner := current.Owner()
	if profile.Delegated() || actor.IsPerson(owner) {
		return StartOwnership{Owner: owner}
	}
	return StartOwnership{Owner: starter, Takes: true, Pauses: conversation.AutomationAllowed(profile.AutomationEnabled)}
}

type StartHold struct {
	Start            ConversationStart
	Previous         *InboxAssignment
	Ownership        StartOwnership
	AutomationBefore *bool
}

func (h StartHold) PreviousOwner() string {
	return h.Previous.Owner()
}

type StartClaimer interface {
	HoldForStarter(in ConversationStart) (StartHold, error)
	ConfirmStart(hold StartHold)
	ReleaseStart(hold StartHold) error
	ClaimForStarter(in ConversationStart) error
}
