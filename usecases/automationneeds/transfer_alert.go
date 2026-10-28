package automationneeds

import (
	"context"
	"strings"

	domain "vozko/domain/automationneeds"
)

func (a *Alerts) TransferUnmet(ctx context.Context, workspaceID string, refusal *domain.Refusal, fromName string) error {
	link := refusal.Automation
	noun := domain.AutomationNoun(link.Automation)
	from := strings.TrimSpace(fromName)
	if from == "" {
		from = "Um agente"
	}
	return a.publishUnmet(ctx, workspaceID, link,
		capitalized(noun)+" "+nameOf(link)+" não recebeu uma transferência: falta "+variablesText(refusal.Needs),
		from+" tentou transferir uma conversa para "+noun+" "+nameOf(link)+", mas a conversa não tem valor para "+variablesText(refusal.Needs)+
			". A conversa continuou com "+from+". Dê um valor padrão a esse dado ou preencha o valor na conversa.")
}
