package automationneeds

import (
	"context"
	"errors"
	"log/slog"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

var errRuntimeUnwired = errors.New("automation needs: the runtime guard needs the service and the alerts")

type Runtime struct {
	service *Service
	alerts  *Alerts
}

func NewRuntime(service *Service, alerts *Alerts) (*Runtime, error) {
	if service == nil || alerts == nil {
		return nil, errRuntimeUnwired
	}
	return &Runtime{service: service, alerts: alerts}, nil
}

func (r *Runtime) AgentValues(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType, agentID string) (map[string]string, bool) {
	automation := conversation.Automation{Kind: conversation.AutomationAgent, ID: agentID}
	values, err := r.service.ValuesFor(ctx, workspaceID, entryID, entryType, automation)
	if err != nil {
		r.refused(ctx, workspaceID, entryID, string(entryType), automation, err)
		return nil, false
	}
	return values, true
}

func (r *Runtime) BeforeStep(run *workflow.WorkflowRun, node *workflow.Node) (*workflow.GatedStep, error) {
	if run == nil || node == nil || node.ID != run.TriggerNodeID {
		return nil, nil
	}
	ctx := context.Background()
	automation := conversation.Automation{Kind: conversation.AutomationWorkflow, ID: run.WorkflowID}
	if run.EntryID == "" || run.OnLead() {
		if err := r.withoutConversation(run.WorkspaceID, automation); err != nil {
			r.refused(ctx, run.WorkspaceID, run.EntryID, run.EntryType, automation, err)
			return nil, err
		}
		return nil, nil
	}
	values, err := r.service.ValuesFor(ctx, run.WorkspaceID, run.EntryID, shared.EntryType(run.EntryType), automation)
	if err != nil {
		r.refused(ctx, run.WorkspaceID, run.EntryID, run.EntryType, automation, err)
		return nil, err
	}
	if values != nil {
		run.State.Set(workflow.DataKeyCampaignVars, conversationvars.CampaignVars(values))
	}
	return nil, nil
}

func (r *Runtime) withoutConversation(workspaceID string, automation conversation.Automation) error {
	needs, err := r.service.Needs(workspaceID, automation)
	if err != nil {
		return err
	}
	if len(needs) > 0 {
		return &domain.Refusal{Cause: domain.ErrConversationMissing, Automation: rootOf(automation, needs), Needs: needs}
	}
	return nil
}

func (r *Runtime) refused(ctx context.Context, workspaceID, entryID, entryType string, automation conversation.Automation, err error) {
	attrs := []any{
		slog.String("workspace_id", workspaceID), slog.String("entry_id", entryID), slog.String("entry_type", entryType),
		slog.String("automation_kind", string(automation.Kind)), slog.String("automation_id", automation.ID),
		slog.String("outcome", "not_acted"), slog.String("error", err.Error()),
	}
	refusal, unmet := domain.RefusalOf(err)
	if !unmet {
		slog.Error("automation.variables.unreadable", attrs...)
		return
	}
	slog.Warn("automation.variables.unmet", append(attrs, slog.Any("variables", refusal.Needs.Names()))...)
	if alertErr := r.alerts.Unmet(ctx, workspaceID, refusal); alertErr != nil {
		slog.Error("automation.variables.alert_failed", append(attrs, slog.String("alert_error", alertErr.Error()))...)
	}
}
