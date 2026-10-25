package inbox_assignment_usecase

import (
	"context"
	"log"

	"vozko/domain/conversation"
)

type AssigneeAlerter interface {
	AlertAssignee(workspaceID, entryID, entryType, userID string)
}

var _ conversation.MessageAttentionReader = (*AssignmentService)(nil)

func (s *AssignmentService) alertAssignee(in recordInput) {
	if !conversation.AlertsAssignee(in.AssignedUserID, in.AssignedByActorID) {
		return
	}
	s.assigneeAlerts.AlertAssignee(in.WorkspaceID, in.EntryID, in.EntryType, in.AssignedUserID)
}

func (s *AssignmentService) MessageAttention(workspaceID, entryID, entryType, knownOwner string) conversation.MessageAttention {
	failClosed := conversation.MessageAttention{Owner: knownOwner}
	if workspaceID == "" {
		log.Printf("[InboxAssignment] attention for entry %s (%s): no workspace, alerting only its owner", entryID, entryType)
		return failClosed
	}
	existing, err := s.repo.FindByEntry(workspaceID, entryID, entryType)
	if err != nil {
		log.Printf("[InboxAssignment] attention for entry %s (%s): owner unreadable, alerting only the owner the entry named: %v", entryID, entryType, err)
		return failClosed
	}
	if owner := existing.Owner(); owner != "" {
		return conversation.MessageAttention{Owner: owner}
	}
	return s.queueAttention(workspaceID, entryID, entryType)
}

func (s *AssignmentService) queueAttention(workspaceID, entryID, entryType string) conversation.MessageAttention {
	cfg, err := s.workspaceConfig.GetByWorkspaceID(context.Background(), workspaceID)
	if err != nil {
		log.Printf("[InboxAssignment] attention for entry %s (%s): workspace settings unreadable, alerting nobody: %v", entryID, entryType, err)
		return conversation.MessageAttention{}
	}
	if cfg.EffectiveRouletteStrategy().DealsAutomatically() {
		return conversation.MessageAttention{Distribution: conversation.DistributionAutomatic}
	}
	_, governed, err := s.governingAutomation(entryID, entryType)
	if err != nil {
		log.Printf("[InboxAssignment] attention for entry %s (%s): automation unreadable, alerting nobody: %v", entryID, entryType, err)
		return conversation.MessageAttention{}
	}
	if governed {
		return conversation.MessageAttention{Distribution: conversation.DistributionAutomatic}
	}
	departmentID, err := s.workspaceResolver.GetEntryDepartmentID(entryID, entryType)
	if err != nil {
		log.Printf("[InboxAssignment] attention for entry %s (%s): department unreadable, alerting nobody: %v", entryID, entryType, err)
		return conversation.MessageAttention{}
	}
	skipAdmins := cfg != nil && cfg.SkipAdminAssignment
	return conversation.MessageAttention{
		Distribution: conversation.DistributionManual,
		Takers:       s.candidates.onlineUsers(workspaceID, departmentID, skipAdmins),
	}
}
