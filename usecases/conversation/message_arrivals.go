package conversation_usecase

import (
	"log"

	"vozko/domain/conversation"
	"vozko/domain/shared/deps"
)

type MessageArrivalsDeps struct {
	Broadcaster conversation.EventBroadcaster
	Workspaces  conversation.CampaignWorkspaceResolver
	Stages      conversation.InitialStageAssigner
	Status      conversation.ConversationStatusUpdater
}

type messageArrivals struct {
	conversation.EventBroadcaster
	workspaces conversation.CampaignWorkspaceResolver
	stages     conversation.InitialStageAssigner
	status     conversation.ConversationStatusUpdater
}

func NewMessageArrivals(d MessageArrivalsDeps) (conversation.EventBroadcaster, error) {
	if err := deps.Require("message arrivals", map[string]any{
		"broadcaster": d.Broadcaster,
		"workspaces":  d.Workspaces,
		"stages":      d.Stages,
		"status":      d.Status,
	}); err != nil {
		return nil, err
	}
	return messageArrivals{EventBroadcaster: d.Broadcaster, workspaces: d.Workspaces, stages: d.Stages, status: d.Status}, nil
}

func (a messageArrivals) BroadcastNewMessage(entryID, entryType string, message *conversation.Message) {
	a.assignInitialStage(entryID, entryType)
	if message != nil {
		if err := a.status.TransitionOnMessage(entryID, entryType, message.MessageType, message.ResolvedDirection()); err != nil {
			log.Printf("[message-arrivals] status of %s (%s) did not move: %v", entryID, entryType, err)
		}
	}
	a.EventBroadcaster.BroadcastNewMessage(entryID, entryType, message)
}

func (a messageArrivals) assignInitialStage(entryID, entryType string) {
	workspaceID, err := a.workspaces.GetEntryWorkspaceID(entryID, entryType)
	if err != nil {
		log.Printf("[message-arrivals] workspace of %s (%s) unreadable, no initial stage: %v", entryID, entryType, err)
		return
	}
	campaignID, _ := a.workspaces.GetEntryCampaignID(entryID, entryType)
	if workspaceID == "" || campaignID == "" {
		return
	}
	a.stages.AutoAssignInitialStage(workspaceID, campaignID, entryType, entryID, entryType)
}
