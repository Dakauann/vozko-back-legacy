package conversation_usecase

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"vozko/domain/conversation"
)

type stageCountScenario struct {
	name       string
	authorizer *inboxServiceTestAuthorizer
	userID     string
	input      conversation.SearchInboxInput
	denied     bool
}

func stageCountScenarios() []stageCountScenario {
	restricted := conversation.DepartmentAccessScope{DepartmentIDs: []string{"dept-1", "dept-2"}, Restrict: true}
	return []stageCountScenario{
		{
			name:       "campaign the operator cannot open",
			authorizer: &inboxServiceTestAuthorizer{canAccessCampaign: false, departmentScope: restricted, departmentScopeOkay: true},
			userID:     "user-1",
			input:      conversation.SearchInboxInput{CampaignID: "campaign-1", CampaignType: "whatsapp", AssignedUserID: "user-1"},
			denied:     true,
		},
		{
			name:       "no department scope",
			authorizer: &inboxServiceTestAuthorizer{canAccessCampaign: true, departmentScopeOkay: false},
			userID:     "user-1",
			input:      conversation.SearchInboxInput{WorkspaceID: "ws-1", AssignedUserID: "user-1"},
			denied:     true,
		},
		{
			name:       "selected department outside the operator's own",
			authorizer: &inboxServiceTestAuthorizer{canAccessCampaign: true, departmentScope: restricted, departmentScopeOkay: true},
			userID:     "user-1",
			input:      conversation.SearchInboxInput{WorkspaceID: "ws-1", SelectedDepartmentID: "dept-other", AssignedUserID: "user-1"},
			denied:     true,
		},
		{
			name:       "restricted department narrowed to the selected one",
			authorizer: &inboxServiceTestAuthorizer{canAccessCampaign: true, departmentScope: restricted, departmentScopeOkay: true},
			userID:     "user-1",
			input:      conversation.SearchInboxInput{WorkspaceID: "ws-1", SelectedDepartmentID: "dept-2", AssignedUserID: "user-1"},
		},
		{
			name:       "member without view-others keeps unassigned or mine",
			authorizer: &inboxServiceTestAuthorizer{canAccessCampaign: true, departmentScopeOkay: true},
			userID:     "user-1",
			input:      conversation.SearchInboxInput{WorkspaceID: "ws-1", AssignedUserID: "user-1"},
		},
		{
			name:       "view-others clears the assignee rule",
			authorizer: &inboxServiceTestAuthorizer{canAccessCampaign: true, departmentScopeOkay: true, viewOthers: true},
			userID:     "user-1",
			input:      conversation.SearchInboxInput{WorkspaceID: "ws-1", AssignedUserID: "user-1"},
		},
		{
			name:       "campaign inbox resolves its workspace for stages",
			authorizer: &inboxServiceTestAuthorizer{canAccessCampaign: true, departmentScope: restricted, departmentScopeOkay: true},
			userID:     "user-1",
			input:      conversation.SearchInboxInput{CampaignID: "campaign-1", CampaignType: "whatsapp", AssignedUserID: "user-1", IsAdmin: false},
		},
		{
			name:       "admin",
			authorizer: &inboxServiceTestAuthorizer{canAccessCampaign: true, departmentScope: conversation.DepartmentAccessScope{WorkspaceHasDepartments: true}, departmentScopeOkay: true, ownerOrAdmin: true},
			userID:     "admin-1",
			input:      conversation.SearchInboxInput{WorkspaceID: "ws-1", SelectedDepartmentID: "dept-sales", AssignedUserID: "admin-1", IsAdmin: true},
		},
	}
}

func TestCountInboxByStagesResolvesTheSameScopeAsSearchInbox(t *testing.T) {
	for _, sc := range stageCountScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			history := &inboxServiceTestHistoryProvider{stageCounts: map[string]int64{"stage-a": 3}}
			service := NewInboxService(history, parityStages{}, nil, &inboxServiceTestResolver{workspaceID: "ws-1"}, nil, sc.authorizer, nil, nil, inboxSources())

			_, _, searchErr := service.SearchInbox(sc.userID, sc.input)
			counts, countErr := service.CountInboxByStages(sc.userID, sc.input, []string{"stage-a", "stage-b"})

			if sc.denied {
				require.ErrorIs(t, searchErr, conversation.ErrUnauthorized)
				require.ErrorIs(t, countErr, conversation.ErrUnauthorized)
				require.Equal(t, searchErr.Error(), countErr.Error())
				require.Nil(t, counts)
				require.Zero(t, history.countCalls)
				return
			}
			require.NoError(t, searchErr)
			require.NoError(t, countErr)
			require.Equal(t, 1, history.countCalls)
			require.Equal(t, history.lastSearch, history.lastCount, "the count must see the input the page search sees")
			require.Equal(t, []string{"stage-a", "stage-b"}, history.lastStageIDs)
			require.Equal(t, map[string]int64{"stage-a": 3}, counts)
			require.Equal(t, "ws-1", history.lastCount.StageWorkspaceID)
		})
	}
}

func TestCountInboxByStagesNeverCarriesASingleStageFilter(t *testing.T) {
	history := &inboxServiceTestHistoryProvider{}
	service := NewInboxService(history, parityStages{}, nil, &inboxServiceTestResolver{workspaceID: "ws-1"}, nil,
		&inboxServiceTestAuthorizer{canAccessCampaign: true, departmentScopeOkay: true}, nil, nil, inboxSources())

	_, err := service.CountInboxByStages("user-1", conversation.SearchInboxInput{WorkspaceID: "ws-1", StageID: "stage-x", StageIDs: []string{"stage-y"}, StageName: "z"}, []string{"stage-a"})

	require.NoError(t, err)
	require.Empty(t, history.lastCount.StageID)
	require.Empty(t, history.lastCount.StageIDs)
	require.Empty(t, history.lastCount.StageName)
}

func TestCountInboxByStagesWithoutAStageProviderCountsEveryConversation(t *testing.T) {
	history := &inboxServiceTestHistoryProvider{}
	service := NewInboxService(history, nil, nil, &inboxServiceTestResolver{workspaceID: "ws-1"}, nil,
		&inboxServiceTestAuthorizer{canAccessCampaign: true, departmentScopeOkay: true}, nil, nil, inboxSources())

	_, _, _ = service.SearchInbox("user-1", conversation.SearchInboxInput{WorkspaceID: "ws-1", StageID: "stage-a"})
	_, err := service.CountInboxByStages("user-1", conversation.SearchInboxInput{WorkspaceID: "ws-1"}, []string{"stage-a"})

	require.NoError(t, err)
	require.Empty(t, history.lastSearch.StageIDs, "without a stage provider the page search ignores the stage")
	require.Empty(t, history.lastCount.StageWorkspaceID, "so the count must not filter by stage either")
}

func TestCountInboxByStagesReturnsTheCountError(t *testing.T) {
	boom := errors.New("db down")
	history := &inboxServiceTestHistoryProvider{countErr: boom}
	service := NewInboxService(history, parityStages{}, nil, &inboxServiceTestResolver{workspaceID: "ws-1"}, nil,
		&inboxServiceTestAuthorizer{canAccessCampaign: true, departmentScopeOkay: true}, nil, nil, inboxSources())

	counts, err := service.CountInboxByStages("user-1", conversation.SearchInboxInput{WorkspaceID: "ws-1"}, []string{"stage-a"})

	require.ErrorIs(t, err, boom)
	require.Nil(t, counts)
}

type capturedStageCount struct {
	capturedSearch
	countInput conversation.SearchEntriesInput
	stageIDs   []string
}

func (c *capturedStageCount) CountEntriesByStages(input conversation.SearchEntriesInput, stageIDs []string) (map[string]int64, error) {
	c.countInput = input
	c.stageIDs = stageIDs
	return map[string]int64{"stage-a": 2}, nil
}

func TestHistoryStageCountMapsTheInputLikeTheSearch(t *testing.T) {
	cases := map[string]conversation.SearchInboxInput{
		"workspace":              {UserID: "user-1", WorkspaceID: "ws-1", CampaignType: "telegram", StageWorkspaceID: "ws-1", AssignedUserID: "user-1", DepartmentIDs: []string{"d-1"}, RestrictDepartments: true, AssigneeOverrideUserID: "user-1"},
		"campaign":               {UserID: "user-1", WorkspaceID: "ws-1", CampaignID: "c-1", CampaignType: "whatsapp", ContainerKind: conversation.ContainerKindCampaign, StageWorkspaceID: "ws-1"},
		"missing campaign type":  {UserID: "user-1", CampaignID: "c-1"},
		"unsupported container":  {UserID: "user-1", CampaignID: "c-1", CampaignType: "nonsense"},
		"global unsupported tab": {UserID: "user-1", WorkspaceID: "ws-1", CampaignType: "nonsense", StageWorkspaceID: "ws-1"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &capturedStageCount{}
			svc := historyWith(HistoryProviderService{messageRepo: repo})

			_, _, searchErr := svc.SearchInboxEntries(in)
			counts, countErr := svc.CountInboxEntriesByStages(in, []string{"stage-a", "stage-b"})

			if searchErr != nil {
				require.EqualError(t, countErr, searchErr.Error())
				require.Nil(t, counts)
				return
			}
			require.NoError(t, countErr)
			require.Equal(t, repo.input, repo.countInput)
			require.Equal(t, []string{"stage-a", "stage-b"}, repo.stageIDs)
			require.Equal(t, map[string]int64{"stage-a": 2}, counts)
		})
	}
}
