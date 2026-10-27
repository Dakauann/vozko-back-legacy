package conversation_usecase

import (
	"log"
	"log/slog"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/shared/deps"
)

type ContactReplies interface {
	ContactReplied(entryID, entryType string, at time.Time) error
}

type MessageArrivalsDeps struct {
	Broadcaster conversation.EventBroadcaster
	Workspaces  conversation.CampaignWorkspaceResolver
	Stages      conversation.InitialStageAssigner
	Status      conversation.ConversationStatusUpdater
	Replies     ContactReplies
}

type messageArrivals struct {
	conversation.EventBroadcaster
	workspaces conversation.CampaignWorkspaceResolver
	stages     conversation.InitialStageAssigner
	status     conversation.ConversationStatusUpdater
	replies    ContactReplies
}

func NewMessageArrivals(d MessageArrivalsDeps) (conversation.EventBroadcaster, error) {
	if err := deps.Require("message arrivals", map[string]any{
		"broadcaster": d.Broadcaster,
		"workspaces":  d.Workspaces,
		"stages":      d.Stages,
		"status":      d.Status,
		"replies":     d.Replies,
	}); err != nil {
		return nil, err
	}
	return messageArrivals{EventBroadcaster: d.Broadcaster, workspaces: d.Workspaces, stages: d.Stages, status: d.Status, replies: d.Replies}, nil
}

func (a messageArrivals) BroadcastNewMessage(entryID, entryType string, message *conversation.Message) {
	a.cancelFollowUpsOnReply(entryID, entryType, message)
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

func (a messageArrivals) cancelFollowUpsOnReply(entryID, entryType string, message *conversation.Message) {
	if message == nil || message.ResolvedDirection() != conversation.MessageDirectionInbound {
		return
	}
	at := message.CreatedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if err := a.replies.ContactReplied(entryID, entryType, at); err != nil {
		slog.Error("conversation.follow_ups.reply_cancel_failed",
			slog.String("entry_id", entryID), slog.String("entry_type", entryType), slog.String("error", err.Error()))
	}
}
