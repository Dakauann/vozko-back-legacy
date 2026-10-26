package inbox_assignment_usecase

import ia "vozko/domain/inbox_assignment"

func NewOpenClaimFixture() (*AssignmentService, ia.Repository) {
	repo := newStatefulRepo()
	return openClaimService(repo, &openClaimReceivers{}, false), repo
}
