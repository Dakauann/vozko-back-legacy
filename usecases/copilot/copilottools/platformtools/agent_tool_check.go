package platformtools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"vozko/domain/automationneeds"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/copilot"
	"vozko/domain/tools"
	"vozko/domain/workspace"
	"vozko/usecases/copilot/copilottools"
)

func (d AgentDeps) refuseBrokenActiveAgent(ctx context.Context, a *agent.Agent, draft []agent.ToolBinding) error {
	if d.ToolCheck == nil || a == nil || !a.IsActive || draft == nil {
		return nil
	}
	report, err := d.ToolCheck.Validate(ctx, a.ID, draft)
	if err != nil {
		return fmt.Errorf("%w: %v", copilottools.ErrInvalidArgs, err)
	}
	if report.IsGreen() {
		return nil
	}
	return fmt.Errorf("%w: %s", copilottools.ErrInvalidArgs, issuesText(report))
}

func blockedMessage(err error) (string, bool) {
	var blocked *agenttool.BlockedError
	if !errors.As(err, &blocked) {
		return "", false
	}
	return "o agente não foi salvo: " + issuesText(blocked.Report), true
}

func issuesText(report agenttool.Report) string {
	lines := make([]string, 0, len(report.Issues))
	for _, issue := range report.Blocking() {
		line := issue.Message
		if issue.Hint != "" {
			line += " (" + issue.Hint + ")"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "; ")
}

type AgentPublisher interface {
	Publish(ctx context.Context, agentID string, by workspace.Requester) (agent.Version, error)
}

const publishParamDescription = "true para publicar o rascunho logo depois de salvar. Editar muda só o rascunho: sem publicar, as conversas continuam com a versão publicada anterior (e um agente novo não atende até ser publicado)."

func publishParam() tools.Parameter {
	return tools.Parameter{Type: "boolean", Description: publishParamDescription}
}

func (d AgentDeps) publishAfterSave(ctx context.Context, cc copilot.Context, args map[string]interface{}, saved *agent.Agent) copilot.Result {
	if publish, _ := args["publish"].(bool); !publish || saved == nil {
		return copilot.Result{Status: copilot.StatusOK, Data: saved}
	}
	if d.Publishing == nil {
		return copilot.Result{Status: copilot.StatusError, Message: "o rascunho foi salvo, mas a publicação não está disponível agora"}
	}
	version, err := d.Publishing.Publish(ctx, saved.ID, copilottools.RequesterOf(cc))
	if refusal, ok := automationneeds.RefusalOf(err); ok {
		return copilot.Result{Status: copilot.StatusError, Message: "o rascunho foi salvo, mas não foi publicado: " + refusal.Sentence()}
	}
	var blocked *agenttool.BlockedError
	if errors.As(err, &blocked) {
		return copilot.Result{Status: copilot.StatusError, Message: "o rascunho foi salvo, mas não foi publicado: " + issuesText(blocked.Report)}
	}
	if err != nil {
		return copilot.Result{Status: copilot.StatusError, Message: "o rascunho foi salvo, mas não foi publicado: " + err.Error()}
	}
	return copilot.Result{Status: copilot.StatusOK, Data: map[string]interface{}{"agent": saved, "publishedVersion": version.Version}}
}

func publishField(args map[string]interface{}) []copilot.Field {
	if publish, _ := args["publish"].(bool); publish {
		return []copilot.Field{{Key: "publish", Value: "sim, logo depois de salvar"}}
	}
	return nil
}
