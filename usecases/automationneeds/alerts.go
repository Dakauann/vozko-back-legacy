package automationneeds

import (
	"context"
	"errors"
	"strings"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/notification"
	"vozko/domain/workspace"
)

var errAlertsUnwired = errors.New("automation needs: alerts need the notification publisher")

type Alerts struct {
	publisher notification.EpisodePublisher
}

func NewAlerts(publisher notification.EpisodePublisher) (*Alerts, error) {
	if publisher == nil {
		return nil, errAlertsUnwired
	}
	return &Alerts{publisher: publisher}, nil
}

func UnmetEpisodeKey(a conversation.Automation) string {
	return "automation_variables_unmet:" + string(a.Kind) + ":" + a.ID
}

func SwitchOffEpisodeKey(kind domain.SlotKind, slotID string) string {
	return "automation_switch_off:" + string(kind) + ":" + slotID
}

func audienceOf(a conversation.Automation) notification.Audience {
	if a.Kind == conversation.AutomationWorkflow {
		return notification.Holding(workspace.ResourceWorkflows, workspace.ActionUpdate)
	}
	return notification.Holding(workspace.ResourceAgents, workspace.ActionUpdate)
}

func nameOf(link domain.Link) string {
	if strings.TrimSpace(link.Name) != "" {
		return link.Name
	}
	return link.ID
}

func variablesText(needs domain.Needs) string {
	return strings.Join(needs.Labels(), ", ")
}

func (a *Alerts) Unmet(ctx context.Context, workspaceID string, refusal *domain.Refusal) error {
	link := refusal.Automation
	noun := domain.AutomationNoun(link.Automation)
	return a.publishUnmet(ctx, workspaceID, link,
		capitalized(noun)+" "+nameOf(link)+" deixou de responder: falta "+variablesText(refusal.Needs),
		"Uma conversa não tem valor para "+variablesText(refusal.Needs)+", então "+noun+" "+nameOf(link)+
			" não respondeu, em vez de mandar um texto incompleto. Dê um valor padrão a esse dado ou preencha o valor na conversa.")
}

func (a *Alerts) publishUnmet(ctx context.Context, workspaceID string, link domain.Link, title, body string) error {
	noun := domain.AutomationNoun(link.Automation)
	_, err := a.publisher.Publish(ctx, notification.Draft{
		WorkspaceID: workspaceID,
		Category:    notification.CategoryAutomations,
		Severity:    notification.SeverityWarning,
		Title:       notification.ClipTitle(title),
		Body:        body,
		ActionURL:   domain.AutomationPath(link.Automation),
		ActionLabel: "Abrir " + strings.TrimPrefix(strings.TrimPrefix(noun, "o "), "a "),
		Audience:    audienceOf(link.Automation),
		Email:       true,
		EpisodeKey:  UnmetEpisodeKey(link.Automation),
		Source:      notification.SourceAutomationVars,
	})
	return err
}

func (a *Alerts) SwitchedOff(ctx context.Context, workspaceID string, off domain.SwitchOff) error {
	link := off.Automation
	noun := domain.AutomationNoun(link.Automation)
	where := domain.SlotLabel(off.Slot.Kind) + " " + off.Slot.Name
	_, err := a.publisher.Publish(ctx, notification.Draft{
		WorkspaceID: workspaceID,
		Category:    notification.CategoryAutomations,
		Severity:    notification.SeverityWarning,
		Title:       notification.ClipTitle(capitalized(noun) + " " + nameOf(link) + " foi desligado no " + where),
		Body: capitalized(noun) + " " + nameOf(link) + " agora exige " + variablesText(off.Needs) +
			", e conversas que o cliente começa não trazem esses valores. Para não responder com textos incompletos, ele foi desligado no " + where +
			". Dê um valor padrão a esse dado e ligue de novo no canal.",
		ActionURL:   domain.SlotPath(off.Slot.Kind, off.Slot.ID),
		ActionLabel: "Abrir o canal",
		Audience:    audienceOf(link.Automation),
		Email:       true,
		EpisodeKey:  SwitchOffEpisodeKey(off.Slot.Kind, off.Slot.ID),
		Source:      notification.SourceAutomationVars,
	})
	return err
}

func (a *Alerts) SwitchOffResolved(ctx context.Context, workspaceID string, kind domain.SlotKind, slotID string) error {
	return a.publisher.Resolve(ctx, workspaceID, SwitchOffEpisodeKey(kind, slotID))
}

func (a *Alerts) UnmetResolved(ctx context.Context, workspaceID string, automation conversation.Automation) error {
	return a.publisher.Resolve(ctx, workspaceID, UnmetEpisodeKey(automation))
}

func capitalized(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
