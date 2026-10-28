package inbox_assignment_usecase

import "errors"

const leadRingAccount = "00000000-0000-0000-0000-000000000001"

var ErrNoLeadCandidate = errors.New("inbox assignment: nobody in the roulette can take this lead now")

func (s *AssignmentService) PickForLead(workspaceID, departmentID string) (string, error) {
	pool := s.humanPool(workspaceID, departmentID)
	if !pool.CanPick() {
		return "", ErrNoLeadCandidate
	}
	userID, _, err := s.claimNextInRing(workspaceID, leadRingAccount, departmentID, pool)
	if err != nil {
		return "", err
	}
	if userID == "" {
		return "", ErrNoLeadCandidate
	}
	return userID, nil
}
