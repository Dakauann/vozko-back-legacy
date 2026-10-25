package conversation_usecase

import (
	"fmt"
	"log"

	conversation "vozko/domain/conversation"
	"vozko/domain/shared/deps"
)

type inboxService struct {
	historyProvider      conversation.HistoryProvider
	StageProvider        conversation.StageProvider
	labelProvider        conversation.LabelProvider
	workspaceResolver    conversation.CampaignWorkspaceResolver
	InitialStageAssigner conversation.InitialStageAssigner
	authorizer           conversation.ConversationAuthorizer
	templateSender       conversation.TemplateSender
	analysisProvider     conversation.AnalysisProvider
	analysisSchedule     conversation.AnalysisScheduleReader
	liveReads            conversation.LiveReadProvider
	statusProvider       conversation.ConversationStatusUpdater
}

func (s *inboxService) attachLiveReads(workspaceID string, entries []conversation.InboxEntry, groups map[string][]int) {
	if workspaceID == "" {
		return
	}
	for entryType, indices := range groups {
		entryIDs := make([]string, len(indices))
		for j, idx := range indices {
			entryIDs[j] = entries[idx].EntryID
		}
		views, err := s.liveReads.LiveReadViews(workspaceID, entryIDs, entryType)
		if err != nil {
			log.Printf("[InboxService] reading live reads for %s failed: %v", entryType, err)
			continue
		}
		for _, idx := range indices {
			if view := views[entries[idx].EntryID]; view != nil {
				entries[idx].LiveRead = view
			}
		}
	}
}

type InboxSources struct {
	Schedules conversation.AnalysisScheduleReader
	Templates conversation.TemplateSender
	LiveReads conversation.LiveReadProvider
}

func NewInboxService(
	historyProvider conversation.HistoryProvider,
	StageProvider conversation.StageProvider,
	labelProvider conversation.LabelProvider,
	workspaceResolver conversation.CampaignWorkspaceResolver,
	InitialStageAssigner conversation.InitialStageAssigner,
	authorizer conversation.ConversationAuthorizer,
	analysisProvider conversation.AnalysisProvider,
	statusProvider conversation.ConversationStatusUpdater,
	sources InboxSources,
) conversation.InboxService {
	if err := deps.Require("inbox", map[string]any{
		"authorizer":         authorizer,
		"analysis schedules": sources.Schedules,
		"templates":          sources.Templates,
		"live reads":         sources.LiveReads,
	}); err != nil {
		panic(err.Error())
	}
	return &inboxService{
		analysisSchedule:     sources.Schedules,
		templateSender:       sources.Templates,
		liveReads:            sources.LiveReads,
		historyProvider:      historyProvider,
		StageProvider:        StageProvider,
		labelProvider:        labelProvider,
		workspaceResolver:    workspaceResolver,
		InitialStageAssigner: InitialStageAssigner,
		authorizer:           authorizer,
		analysisProvider:     analysisProvider,
		statusProvider:       statusProvider,
	}
}

func (s *inboxService) GetInboxPage(userID, campaignID, campaignType, campaignWorkspaceID string, page, pageSize int, isAdmin bool) ([]conversation.InboxEntry, int64, error) {
	if s.historyProvider == nil {
		return nil, 0, nil
	}

	return s.SearchInbox(userID, conversation.SearchInboxInput{
		CampaignID:     campaignID,
		CampaignType:   campaignType,
		WorkspaceID:    campaignWorkspaceID,
		AssignedUserID: userID,
		IsAdmin:        isAdmin,
		Page:           page,
		PageSize:       pageSize,
	})
}

func (s *inboxService) GetInboxPageByWorkspace(userID, workspaceID, entryTypeFilter, whatsappCampaignType string, page, pageSize int, isAdmin bool) ([]conversation.InboxEntry, int64, error) {
	if s.historyProvider == nil {
		return nil, 0, nil
	}

	return s.SearchInbox(userID, conversation.SearchInboxInput{
		CampaignType:         entryTypeFilter,
		WhatsAppCampaignType: whatsappCampaignType,
		WorkspaceID:          workspaceID,
		AssignedUserID:       userID,
		IsAdmin:              isAdmin,
		Page:                 page,
		PageSize:             pageSize,
	})
}

func (s *inboxService) SearchInbox(userID string, input conversation.SearchInboxInput) ([]conversation.InboxEntry, int64, error) {
	if s.historyProvider == nil {
		return nil, 0, nil
	}
	input, err := s.resolveInboxScope(userID, input)
	if err != nil {
		return nil, 0, err
	}
	stageWorkspaceID := input.StageWorkspaceID

	if s.StageProvider != nil && (input.StageID != "" || input.StageName != "" || len(input.StageIDs) > 0) {
		if input.StageID != "" {

			input.StageIDs = []string{input.StageID}
		} else if input.StageName != "" {
			resolved, err := s.StageProvider.FindStageIDsByName(stageWorkspaceID, input.StageName)
			if err != nil {
				return nil, 0, nil
			}
			input.StageIDs = resolved
		}

	}

	entries, totalItems, err := s.historyProvider.SearchInboxEntries(input)
	if err != nil {
		return nil, 0, err
	}

	s.assignInitialStages(entries, input.WorkspaceID)
	s.decorateEntries(entries, stageWorkspaceID)

	return entries, totalItems, nil
}

func (s *inboxService) CountInboxByStages(userID string, input conversation.SearchInboxInput, stageIDs []string) (map[string]int64, error) {
	if s.historyProvider == nil {
		return nil, nil
	}
	input, err := s.resolveInboxScope(userID, input)
	if err != nil {
		return nil, err
	}
	input.StageID, input.StageName, input.StageIDs = "", "", nil
	if s.StageProvider == nil {
		input.StageWorkspaceID = ""
	}
	return s.historyProvider.CountInboxEntriesByStages(input, stageIDs)
}

func (s *inboxService) resolveInboxScope(userID string, input conversation.SearchInboxInput) (conversation.SearchInboxInput, error) {
	input.UserID = userID

	workspaceForOwnerCheck := input.WorkspaceID
	if workspaceForOwnerCheck == "" && input.CampaignID != "" && s.workspaceResolver != nil {
		workspaceForOwnerCheck, _ = s.workspaceResolver.GetCampaignWorkspaceID(input.CampaignID, input.CampaignType)
	}

	if input.WorkspaceID == "" {
		input.WorkspaceID = workspaceForOwnerCheck
	}
	if input.CampaignID != "" && input.CampaignType != "" {
		if !s.authorizer.CanAccessCampaign(userID, workspaceForOwnerCheck, input.CampaignID, input.CampaignType, input.IsAdmin) {
			return input, fmt.Errorf("%w: you don't have access to this inbox", conversation.ErrUnauthorized)
		}
	}
	if input.WorkspaceID != "" {
		scope, allowed := s.authorizer.GetDepartmentScope(userID, input.WorkspaceID, input.IsAdmin)
		if !allowed {
			return input, fmt.Errorf("%w: you don't have access to this inbox", conversation.ErrUnauthorized)
		}
		read, err := scope.ReadScope(userID, input.IsAdmin, input.SelectedDepartmentID)
		if err != nil {
			return input, fmt.Errorf("%w: you don't have access to this department", conversation.ErrUnauthorized)
		}
		input.DepartmentIDs = read.DepartmentIDs
		input.RestrictDepartments = read.RestrictDepartments
		input.AssigneeOverrideUserID = read.AssigneeOverrideUserID
	}
	if input.IsAdmin || (workspaceForOwnerCheck != "" && conversation.SeesOthers(s.authorizer, userID, workspaceForOwnerCheck)) {
		input.AssignedUserID = ""
	}

	var stageWorkspaceID string
	if input.WorkspaceID != "" {
		stageWorkspaceID = input.WorkspaceID
	} else if s.workspaceResolver != nil && input.CampaignID != "" {
		stageWorkspaceID, _ = s.workspaceResolver.GetCampaignWorkspaceID(input.CampaignID, input.CampaignType)
	}
	input.StageWorkspaceID = stageWorkspaceID

	return input, nil
}

func (s *inboxService) BuildInboxEntry(entryID, entryType string) (*conversation.InboxEntry, error) {
	if s.historyProvider == nil {
		return nil, fmt.Errorf("history provider not configured")
	}
	entry, err := s.historyProvider.GetInboxEntry(entryID, entryType)
	if err != nil || entry == nil {
		return entry, err
	}
	workspaceID := ""
	if s.workspaceResolver != nil {
		workspaceID, _ = s.workspaceResolver.GetEntryWorkspaceID(entryID, entryType)
	}
	batch := []conversation.InboxEntry{*entry}
	s.assignInitialStages(batch, workspaceID)
	s.decorateEntries(batch, workspaceID)
	*entry = batch[0]
	return entry, nil
}

func (s *inboxService) SendTemplateForEntry(entryID, entryType, templateID string, parameters []string, userID, workspaceID string, isAdmin bool) (string, error) {
	if !s.authorizer.CanAccessEntry(userID, workspaceID, entryID, entryType, isAdmin) {
		return "", fmt.Errorf("unauthorized: you don't have access to this entry")
	}
	return s.templateSender.SendTemplate(entryID, entryType, templateID, parameters, nil, userID, workspaceID)
}

func (s *inboxService) attachAvailableStages(workspaceID string, entries []conversation.InboxEntry) {
	if s.StageProvider == nil || workspaceID == "" || len(entries) == 0 {
		return
	}
	placements := make([]conversation.StagePlacement, len(entries))
	for i, entry := range entries {
		placements[i] = conversation.StagePlacement{CampaignID: entry.CampaignID, CampaignType: entry.EntryType}
		if entry.Stage != nil {
			placements[i].CurrentStageID = entry.Stage.StageID
		}
	}
	available, err := s.StageProvider.GetAvailableStages(workspaceID, placements)
	if err != nil {
		log.Printf("[InboxService] Error resolving the stages each conversation can move to: %v", err)
		return
	}
	for i := range entries {
		entries[i].AvailableStages = available[i]
	}
}

func (s *inboxService) decorateEntries(entries []conversation.InboxEntry, campaignWorkspaceID string) {
	if len(entries) == 0 {
		return
	}

	entryTypeGroups := make(map[string][]int)
	for i := range entries {
		if entries[i].EntryID == "" || entries[i].EntryType == "" {
			continue
		}
		entryTypeGroups[entries[i].EntryType] = append(entryTypeGroups[entries[i].EntryType], i)
	}

	if s.StageProvider != nil && campaignWorkspaceID != "" {
		for entryType, indices := range entryTypeGroups {
			entryIDs := make([]string, len(indices))
			for j, idx := range indices {
				entryIDs[j] = entries[idx].EntryID
			}
			batchStages, err := s.StageProvider.GetBatchEntryStages(entryIDs, entryType, campaignWorkspaceID)
			if err == nil {
				for _, idx := range indices {
					if tag, ok := batchStages[entries[idx].EntryID]; ok {
						entries[idx].Stage = tag
					}
				}
			} else {
				log.Printf("[InboxService] Error batch-fetching tags for type %s: %v", entryType, err)
			}
		}
	}

	if s.labelProvider != nil && campaignWorkspaceID != "" {
		for entryType, indices := range entryTypeGroups {
			entryIDs := make([]string, len(indices))
			for j, idx := range indices {
				entryIDs[j] = entries[idx].EntryID
			}
			batchLabels, err := s.labelProvider.GetBatchEntryLabels(entryIDs, entryType, campaignWorkspaceID)
			if err == nil {
				for _, idx := range indices {
					if labels, ok := batchLabels[entries[idx].EntryID]; ok {
						entries[idx].Labels = labels
					}
				}
			} else {
				log.Printf("[InboxService] Error batch-fetching labels for type %s: %v", entryType, err)
			}
		}
	}

	s.attachAvailableStages(campaignWorkspaceID, entries)

	if s.analysisProvider != nil {
		for entryType, indices := range entryTypeGroups {
			entryIDs := make([]string, len(indices))
			for j, idx := range indices {
				entryIDs[j] = entries[idx].EntryID
			}
			batchAnalysis, err := s.analysisProvider.GetBatchLatestAnalysis(entryIDs, entryType)
			if err == nil {
				for _, idx := range indices {
					if a, ok := batchAnalysis[entries[idx].EntryID]; ok {
						entries[idx].LatestAnalysis = a
					}
				}
			}
			pending, err := s.analysisProvider.GetBatchAnalysisPending(entryIDs, entryType)
			if err == nil {
				for _, idx := range indices {
					if pending[entries[idx].EntryID] {
						entries[idx].AnalysisPhase = conversation.AnalysisPhaseQueued
					}
				}
			}
			awaiting, err := s.analysisSchedule.AwaitingAnalysis(entryIDs, entryType)
			if err == nil {
				for _, idx := range indices {
					if awaiting[entries[idx].EntryID] && entries[idx].AnalysisPhase == conversation.AnalysisPhaseNone {
						entries[idx].AnalysisPhase = conversation.AnalysisPhaseAwaiting
					}
				}
			}
		}
	}
	s.attachLiveReads(campaignWorkspaceID, entries, entryTypeGroups)
}

func (s *inboxService) assignInitialStages(entries []conversation.InboxEntry, campaignWorkspaceID string) {
	if s.InitialStageAssigner == nil || campaignWorkspaceID == "" {
		return
	}

	for i := range entries {
		if entries[i].EntryID == "" || entries[i].EntryType == "" {
			continue
		}
		containerID := conversation.ContainerIDOf(entries[i])
		if containerID == "" {
			continue
		}
		s.InitialStageAssigner.AutoAssignInitialStage(campaignWorkspaceID, containerID, entries[i].EntryType, entries[i].EntryID, entries[i].EntryType)
	}
}

func (s *inboxService) GetInboxStageCounts(workspaceID, campaignID, entryType string) (map[string]int64, error) {
	if s.StageProvider == nil {
		return nil, nil
	}
	if campaignID != "" {
		return s.StageProvider.GetStageCountsForCampaign(workspaceID, campaignID, entryType)
	}
	return s.StageProvider.GetStageCountsForWorkspace(workspaceID, entryType)
}

func (s *inboxService) GetConversationStatusCounts(workspaceID, campaignID, entryType string) (map[string]int64, error) {
	if s.statusProvider == nil {
		return nil, nil
	}
	return s.statusProvider.GetStatusCounts(workspaceID, campaignID, entryType)
}

func (s *inboxService) GetAvailableLabels(workspaceID string) ([]conversation.InboxEntryLabel, error) {
	if s.labelProvider == nil {
		return nil, nil
	}
	return s.labelProvider.GetAvailableLabels(workspaceID)
}
