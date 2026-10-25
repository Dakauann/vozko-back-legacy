package inbox_assignment_usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ia "vozko/domain/inbox_assignment"
)

type openClaimReceivers struct {
	stubReceivers
	members map[string]bool
}

func (r *openClaimReceivers) IsWorkspaceMember(userID, _ string) bool {
	if r.members == nil {
		return true
	}
	return r.members[userID]
}

type openClaimAccounts struct{ account string }

func (a openClaimAccounts) EntryAccountID(string, string) (string, error) { return a.account, nil }

func openClaimService(repo *statefulRepo, receivers *openClaimReceivers, skipAdmins bool) *AssignmentService {
	svc := newService(repo, defaultEligible(), defaultResolver("ws-1", ""), claimConfig(skipAdmins))
	svc.receivers = receivers
	svc.accounts = openClaimAccounts{account: "phone-1"}
	return svc
}

func TestOpeningAnUnassignedConversationClaimsItOnTheEntryAccount(t *testing.T) {
	repo := newStatefulRepo()
	svc := openClaimService(repo, &openClaimReceivers{}, false)

	claimed, err := svc.AssignOnOpen("entry-1", "whatsapp", "ana", false)

	require.NoError(t, err)
	assert.True(t, claimed)
	got := repo.assignments[assignmentKey("ws-1", "entry-1", "whatsapp")]
	require.NotNil(t, got)
	assert.Equal(t, "ana", got.AssignedUserID)
	assert.Equal(t, "phone-1", got.BusinessPhoneID)
}

func TestOpeningAnOwnedConversationKeepsItsOwner(t *testing.T) {
	repo := newStatefulRepo()
	_ = repo.Assign(&ia.InboxAssignment{WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: "whatsapp", AssignedUserID: "bia"})
	svc := openClaimService(repo, &openClaimReceivers{}, false)

	claimed, err := svc.AssignOnOpen("entry-1", "whatsapp", "ana", false)

	require.NoError(t, err)
	assert.False(t, claimed)
	assert.Equal(t, "bia", repo.assignments[assignmentKey("ws-1", "entry-1", "whatsapp")].AssignedUserID)
}

func TestWhoMayClaimAConversationByOpeningIt(t *testing.T) {
	cases := []struct {
		name        string
		receivers   *openClaimReceivers
		skipAdmins  bool
		systemAdmin bool
		claims      bool
	}{
		{name: "a system admin outside the workspace", receivers: &openClaimReceivers{members: map[string]bool{"ana": false}}, systemAdmin: true},
		{name: "a system admin inside the workspace", receivers: &openClaimReceivers{members: map[string]bool{"ana": true}}, systemAdmin: true, claims: true},
		{name: "an owner while admins are skipped", receivers: &openClaimReceivers{stubReceivers: stubReceivers{admins: map[string]bool{"ana": true}}}, skipAdmins: true},
		{name: "an owner while admins are dealt", receivers: &openClaimReceivers{stubReceivers: stubReceivers{admins: map[string]bool{"ana": true}}}, claims: true},
		{name: "a member without the roulette permission", receivers: &openClaimReceivers{stubReceivers: stubReceivers{noRoulette: map[string]bool{"ana": true}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newStatefulRepo()
			svc := openClaimService(repo, tc.receivers, tc.skipAdmins)

			claimed, err := svc.AssignOnOpen("entry-1", "whatsapp", "ana", tc.systemAdmin)

			require.NoError(t, err)
			assert.Equal(t, tc.claims, claimed)
			assert.Equal(t, tc.claims, repo.assignments[assignmentKey("ws-1", "entry-1", "whatsapp")] != nil)
		})
	}
}

func TestAConversationWithoutAWorkspaceIsNeverClaimedByOpening(t *testing.T) {
	repo := newStatefulRepo()
	svc := openClaimService(repo, &openClaimReceivers{}, false)
	svc.workspaceResolver = defaultResolver("", "")

	claimed, err := svc.AssignOnOpen("entry-1", "whatsapp", "ana", false)

	require.NoError(t, err)
	assert.False(t, claimed)
	assert.Empty(t, repo.assignments)
}
