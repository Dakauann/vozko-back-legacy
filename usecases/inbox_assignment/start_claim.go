package inbox_assignment_usecase

import (
	"context"
	"errors"
	"fmt"
	"log"

	"vozko/domain/actor"
	aa "vozko/domain/ai_attendance"
	ia "vozko/domain/inbox_assignment"
	"vozko/domain/shared"
)

var _ ia.StartClaimer = (*AssignmentService)(nil)

func (s *AssignmentService) HoldForStarter(in ia.ConversationStart) (ia.StartHold, error) {
	if !in.ByPerson() {
		return ia.StartHold{Start: in}, nil
	}
	current, err := s.repo.FindByEntry(in.WorkspaceID, in.EntryID, in.EntryType)
	if err != nil {
		return ia.StartHold{}, ia.StarterNotAssigned(fmt.Errorf("owner of %s (%s): %w", in.EntryID, in.EntryType, err))
	}
	profile, err := s.automationProfiles.EntryAutomation(in.EntryID, in.EntryType)
	if err != nil {
		return ia.StartHold{}, ia.StarterNotAssigned(fmt.Errorf("automation of %s (%s): %w", in.EntryID, in.EntryType, err))
	}
	hold := ia.StartHold{
		Start:            in,
		Previous:         current,
		Ownership:        ia.StartOwnershipFor(in.StarterID, current, profile),
		AutomationBefore: profile.AutomationEnabled,
	}
	if hold.Ownership.Takes {
		if err := s.repo.Assign(&ia.InboxAssignment{
			WorkspaceID:     in.WorkspaceID,
			BusinessPhoneID: in.AccountID,
			EntryID:         in.EntryID,
			EntryType:       in.EntryType,
			AssignedUserID:  in.StarterID,
		}); err != nil {
			return ia.StartHold{}, ia.StarterNotAssigned(fmt.Errorf("assign %s (%s) to %s: %w", in.EntryID, in.EntryType, in.StarterID, err))
		}
	}
	if hold.Ownership.Pauses {
		off := false
		if err := s.pauser.SetAutomation(context.Background(), in.EntryID, shared.EntryType(in.EntryType), &off); err != nil {
			pauseErr := fmt.Errorf("pause the automation of %s (%s): %w", in.EntryID, in.EntryType, err)
			return ia.StartHold{}, ia.StarterNotAssigned(errors.Join(pauseErr, s.restoreOwner(hold)))
		}
	}
	return hold, nil
}

func (s *AssignmentService) ConfirmStart(hold ia.StartHold) {
	if !hold.Ownership.Takes {
		return
	}
	in, previous := hold.Start, hold.PreviousOwner()
	if actor.IsAutomation(previous) {
		s.endAISession(in.WorkspaceID, in.EntryID, in.EntryType, in.StarterID, in.StarterID, aa.EndReasonManualAssignment)
	}
	s.recordOwnerChange(in.EntryID, in.EntryType, in.AccountID, in.WorkspaceID, in.StarterID, previous, in.StarterID, ia.TriggerOutreachSent)
	s.announceOwner(in.WorkspaceID, in.EntryID, in.EntryType, previous)
}

func (s *AssignmentService) ReleaseStart(hold ia.StartHold) error {
	var restoreErr error
	if hold.Ownership.Pauses {
		in := hold.Start
		if err := s.pauser.SetAutomation(context.Background(), in.EntryID, shared.EntryType(in.EntryType), hold.AutomationBefore); err != nil {
			restoreErr = fmt.Errorf("restore the automation of %s (%s): %w", in.EntryID, in.EntryType, err)
		}
	}
	return errors.Join(restoreErr, s.restoreOwner(hold))
}

func (s *AssignmentService) ClaimForStarter(in ia.ConversationStart) error {
	hold, err := s.HoldForStarter(in)
	if err != nil {
		return err
	}
	s.ConfirmStart(hold)
	return nil
}

func (s *AssignmentService) restoreOwner(hold ia.StartHold) error {
	if !hold.Ownership.Takes {
		return nil
	}
	in := hold.Start
	var err error
	if hold.Previous == nil {
		err = s.repo.Unassign(in.WorkspaceID, in.EntryID, in.EntryType)
	} else {
		previous := *hold.Previous
		err = s.repo.Assign(&previous)
	}
	if err != nil {
		log.Printf("[InboxAssignment] start of %s (%s) by %s was undone but its owner %q could not be restored: %v", in.EntryID, in.EntryType, in.StarterID, hold.PreviousOwner(), err)
		return fmt.Errorf("restore the owner of %s (%s): %w", in.EntryID, in.EntryType, err)
	}
	return nil
}
