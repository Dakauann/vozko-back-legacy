package conversation_usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"vozko/domain/conversation"
	ia "vozko/domain/inbox_assignment"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
)

var errOwnerWorkspaceUnknown = errors.New("automation gate: the conversation has no workspace, so its owner cannot be read")

type AutomationOwners interface {
	FindByEntry(workspaceID, entryID, entryType string) (*ia.InboxAssignment, error)
}

type AutomationGateDeps struct {
	Owners      AutomationOwners
	Delegations conversation.DelegationRepository
	Profiles    conversation.EntryAutomationReader
}

type AutomationGate struct {
	owners      AutomationOwners
	delegations conversation.DelegationRepository
	profiles    conversation.EntryAutomationReader
}

func NewAutomationGate(d AutomationGateDeps) *AutomationGate {
	if err := deps.Require("automation gate", map[string]any{
		"conversation owners": d.Owners,
		"delegations":         d.Delegations,
		"automation profiles": d.Profiles,
	}); err != nil {
		panic(err.Error())
	}
	return &AutomationGate{owners: d.Owners, delegations: d.Delegations, profiles: d.Profiles}
}

type AutomationAdmission struct {
	WorkspaceID string
	EntryID     string
	EntryType   shared.EntryType
	Enabled     *bool
	Channel     conversation.ChannelAutomation
}

func (g *AutomationGate) Admit(ctx context.Context, in AutomationAdmission) (conversation.ChannelAutomation, bool) {
	delegation, err := g.delegations.Find(ctx, in.EntryID, in.EntryType)
	if err != nil {
		logUnreadableStanding(in.WorkspaceID, in.EntryID, in.EntryType, fmt.Errorf("delegation: %w", err))
		return in.Channel, false
	}
	config, standing := in.Channel, conversation.AutomationStanding{Enabled: in.Enabled}
	if delegation != nil && delegation.Automation.Valid() {
		config, standing.Delegated = config.DelegatedTo(delegation.Automation), true
	}
	mayAct, err := g.decide(in.WorkspaceID, in.EntryID, in.EntryType, standing)
	if err != nil {
		logUnreadableStanding(in.WorkspaceID, in.EntryID, in.EntryType, err)
		return config, false
	}
	return config, mayAct
}

func (g *AutomationGate) MayAct(workspaceID, entryID string, entryType shared.EntryType) (bool, error) {
	_, mayAct, err := g.stored(workspaceID, entryID, entryType)
	return mayAct, err
}

func (g *AutomationGate) Governing(workspaceID, entryID string, entryType shared.EntryType) (conversation.Automation, bool, error) {
	profile, mayAct, err := g.stored(workspaceID, entryID, entryType)
	if err != nil || !mayAct {
		return conversation.Automation{}, false, err
	}
	governing, governed := profile.Governing()
	return governing, governed, nil
}

func (g *AutomationGate) stored(workspaceID, entryID string, entryType shared.EntryType) (conversation.AutomationProfile, bool, error) {
	profile, err := g.profiles.EntryAutomation(entryID, string(entryType))
	if err != nil {
		return conversation.AutomationProfile{}, false, fmt.Errorf("automation of %s %s: %w", entryType, entryID, err)
	}
	mayAct, err := g.decide(workspaceID, entryID, entryType, profile.Standing())
	return profile, mayAct, err
}

func (g *AutomationGate) decide(workspaceID, entryID string, entryType shared.EntryType, standing conversation.AutomationStanding) (bool, error) {
	if standing.OwnerDecides() {
		owner, err := g.ownerOf(workspaceID, entryID, entryType)
		if err != nil {
			return false, err
		}
		standing.Owner = owner
	}
	if !standing.MayAct() && standing.Owner != "" {
		slog.Info("conversation.automation.person_owns",
			slog.String("workspace_id", workspaceID), slog.String("entry_id", entryID),
			slog.String("entry_type", string(entryType)), slog.String("owner", standing.Owner))
	}
	return standing.MayAct(), nil
}

func (g *AutomationGate) ownerOf(workspaceID, entryID string, entryType shared.EntryType) (string, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return "", errOwnerWorkspaceUnknown
	}
	current, err := g.owners.FindByEntry(workspaceID, entryID, string(entryType))
	if err != nil {
		return "", fmt.Errorf("owner of %s %s: %w", entryType, entryID, err)
	}
	return current.Owner(), nil
}

func logUnreadableStanding(workspaceID, entryID string, entryType shared.EntryType, err error) {
	slog.Warn("conversation.automation.unreadable",
		slog.String("workspace_id", workspaceID), slog.String("entry_id", entryID),
		slog.String("entry_type", string(entryType)), slog.String("outcome", "no_automation"),
		slog.String("error", err.Error()))
}
