package tools_usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/automationneeds"
	"vozko/domain/automationtransfer"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	"vozko/domain/shared"
	"vozko/domain/tools"
	"vozko/domain/workflow"
	"vozko/usecases/agentctx"
	automationtransfer_usecase "vozko/usecases/automationtransfer"
)

const (
	TransferToAutomationToolName = "transfer_to_automation"
	transferDestinationParam     = "destination"
	transferNoteParam            = "note"
	transferVariablesParam       = "variables"
)

const (
	transferUnavailable  = "Nenhum destino de transferência está disponível agora. Não chame esta ferramenta."
	transferNoChat       = "Não foi possível identificar a conversa atual. Esta ferramenta só funciona durante um atendimento."
	transferLoop         = "Esta conversa já foi transferida várias vezes nas últimas 24 horas. Não transfira de novo: continue o atendimento."
	transferUnknown      = "Destino desconhecido. Escolha um dos destinos listados no parâmetro destination."
	transferNoNote       = "Escreva em note o motivo da transferência e um resumo do que já foi conversado, e chame de novo."
	transferNotYours     = "Esta conversa não está mais com você, então não pode transferi-la. Não chame esta ferramenta de novo nesta conversa."
	transferTargetDown   = "Este destino está indisponível agora. Continue o atendimento e não tente este destino de novo."
	transferStageInvalid = "A etapa deste destino não existe mais. Continue o atendimento e não tente este destino de novo."
	transferFailed       = "Não foi possível transferir a conversa agora. Continue o atendimento."
)

type AutomationTransfers interface {
	Offer(workspaceID, selfAgentID string, config map[string]interface{}) ([]automationtransfer_usecase.Offer, error)
	CheckBinding(workspaceID, selfAgentID string, config map[string]interface{}) error
	Transfer(ctx context.Context, in automationtransfer_usecase.Request) (automationtransfer_usecase.Done, error)
}

type transferToAutomationTool struct {
	transfers AutomationTransfers
}

func NewTransferToAutomationTool(transfers AutomationTransfers) tools.Handler {
	if transfers == nil {
		return nil
	}
	return &transferToAutomationTool{transfers: transfers}
}

func (t *transferToAutomationTool) Definition() tools.Definition {
	return tools.Definition{
		Name:               TransferToAutomationToolName,
		DisplayName:        "Transferir para outra automação",
		DisplayDescription: "Passa a conversa para um agente ou fluxo da lista de destinos, que continua o atendimento com o histórico e a nota de quem transferiu. Cada destino pode levar a conversa para uma etapa de outro funil.",
		Description: `Transfere a conversa atual para outra automação (um agente ou um fluxo) da lista de destinos, para ela continuar o atendimento.

Escolha o destino pela descrição de cada um e escreva em note o motivo e um resumo do que já foi conversado: quem recebe a conversa lê essa nota. Depois de transferir você não atende mais esta conversa.`,
		Parameters: map[string]tools.Parameter{
			transferDestinationParam: {
				Type:               "string",
				Description:        "Destino da transferência.",
				DisplayName:        "Destino",
				DisplayDescription: "O destino escolhido pela IA",
			},
			transferNoteParam: {
				Type:               "string",
				Description:        "Motivo da transferência e resumo do que já foi conversado, em português, para quem recebe a conversa continuar sem repetir perguntas.",
				DisplayName:        "Nota da transferência",
				DisplayDescription: "Motivo e resumo que o destino recebe",
			},
			transferVariablesParam: {
				Type:               "object",
				Description:        "Valores que o destino escolhido pede, quando a lista de destinos diz o que informar. Use o nome de cada valor como chave.",
				DisplayName:        "Dados para o destino",
				DisplayDescription: "Valores que a IA informa ao transferir",
			},
		},
		Required: []string{transferDestinationParam, transferNoteParam},
		ConfigSchema: map[string]tools.ConfigParameter{
			automationtransfer.DestinationsKey: {
				Type:               "array",
				Description:        "Destinos: lista de {kind: agent|workflow, automation_id, stage_id opcional, description opcional}.",
				DisplayName:        "Destinos",
				DisplayDescription: "Agentes e fluxos que podem receber a conversa, cada um com uma etapa opcional e a descrição de quando usar.",
				Required:           true,
			},
		},
		RequiredConfig: []string{automationtransfer.DestinationsKey},
		RequiresConfig: true,
		Visibility:     []tools.ToolVisibility{tools.VisibilityMessaging},
		Category:       tools.CategoryAgentAction,
	}
}

func (t *transferToAutomationTool) DefinitionWithContext(ctx tools.ToolContext) tools.Definition {
	def := t.Definition()
	offers, err := t.transfers.Offer(ctx.WorkspaceID, agentOfToolContext(ctx), ctx.Config)
	if err != nil || len(offers) == 0 {
		def.Description = transferUnavailable
		return def
	}
	ids := make([]string, 0, len(offers))
	lines := make([]string, 0, len(offers))
	for _, offer := range offers {
		ids = append(ids, offer.Destination.ID)
		line := offer.Destination.ID + ": " + offer.Label
		if offer.Description != "" {
			line += ". " + offer.Description
		}
		if informs := informsLine(offer.Informs); informs != "" {
			line += ". " + informs
		}
		lines = append(lines, "- "+line)
	}
	param := def.Parameters[transferDestinationParam]
	param.Enum = ids
	param.Description = "Destino da transferência. Destinos:\n" + strings.Join(lines, "\n")
	def.Parameters[transferDestinationParam] = param
	return def
}

func informsLine(informs []automationtransfer.VariableSource) string {
	if len(informs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(informs))
	for _, v := range informs {
		part := v.Name
		if v.Description != "" {
			part += " (" + v.Description + ")"
		}
		parts = append(parts, part)
	}
	return "Informe em variables: " + strings.Join(parts, "; ") + "."
}

func providedValues(raw interface{}) map[string]string {
	fields, _ := raw.(map[string]interface{})
	out := make(map[string]string, len(fields))
	for name, value := range fields {
		switch v := value.(type) {
		case string:
			out[name] = v
		case float64, bool:
			out[name] = fmt.Sprint(v)
		}
	}
	return out
}

func agentOfToolContext(ctx tools.ToolContext) string {
	if a, ok := ctx.Agent.(*agent.Agent); ok && a != nil {
		return a.ID
	}
	return ""
}

func (t *transferToAutomationTool) Execute(ctx context.Context, params map[string]interface{}) (tools.ExecutionResult, error) {
	return t.ExecuteWithConfig(ctx, nil, params)
}

func (t *transferToAutomationTool) ExecuteWithConfig(ctx context.Context, config map[string]interface{}, params map[string]interface{}) (tools.ExecutionResult, error) {
	agentID := agentIDOf(ctx, config)
	destination, _ := params[transferDestinationParam].(string)
	note, _ := params[transferNoteParam].(string)
	done, err := t.transfers.Transfer(ctx, automationtransfer_usecase.Request{
		WorkspaceID:   configString(config, "__workspace_id"),
		EntryID:       configString(config, "__entry_id"),
		EntryType:     shared.EntryType(configString(config, "__entry_type")),
		AgentID:       agentID,
		AgentName:     agentNameOf(ctx, agentID),
		Config:        config,
		DestinationID: destination,
		Note:          note,
		Provided:      providedValues(params[transferVariablesParam]),
	})
	if err != nil {
		slog.Warn("tools.transfer_to_automation.refused",
			slog.String("entry_id", configString(config, "__entry_id")), slog.String("agent_id", agentID),
			slog.String("destination", destination), slog.String("error", err.Error()))
		return refusal(transferRefusal(err)), nil
	}
	if done.Record.To.Kind == conversation.AutomationWorkflow {
		return tools.ExecutionResult{Result: fmt.Sprintf("Conversa transferida para o fluxo %s, que já assumiu o atendimento. Não envie mais mensagens nesta conversa.", done.Label)}, nil
	}
	return tools.ExecutionResult{Result: fmt.Sprintf("Conversa transferida para %s, que responde a partir da próxima mensagem do cliente e recebe a sua nota. Se fizer sentido, avise o cliente em uma frase curta; não continue o atendimento.", done.Label)}, nil
}

func transferRefusal(err error) string {
	var named *conversationvars.VariablesError
	if errors.As(err, &named) {
		labels := make([]string, 0, len(named.Names))
		for _, name := range named.Names {
			labels = append(labels, named.LabelOf(name)+" ("+name+")")
		}
		names := strings.Join(labels, ", ")
		switch {
		case errors.Is(err, automationtransfer.ErrModelValueMissing):
			return fmt.Sprintf("Para transferir para este destino, informe em variables: %s. Se não souber, pergunte ao cliente e chame de novo.", names)
		case errors.Is(err, conversationvars.ErrValueEmpty):
			return fmt.Sprintf("Esta conversa não tem valor para %s, que este destino precisa. Pergunte ao cliente ou continue o atendimento.", names)
		case errors.Is(err, conversationvars.ErrTypeInvalid):
			return fmt.Sprintf("O valor de %s não está no formato esperado (número, data ou uma das opções informadas). Corrija e chame de novo.", names)
		case errors.Is(err, conversationvars.ErrValueInvalid):
			return fmt.Sprintf("O valor de %s precisa ter de 1 a 2000 caracteres. Corrija e chame de novo.", names)
		}
	}
	if refusal, unmet := automationneeds.RefusalOf(err); unmet {
		return fmt.Sprintf("Este destino precisa de %s e esta conversa não tem esses dados. A conversa continua com você e a equipe foi avisada: continue o atendimento.", strings.Join(refusal.Needs.Labels(), ", "))
	}
	switch {
	case errors.Is(err, automationtransfer.ErrNoConversation):
		return transferNoChat
	case errors.Is(err, automationtransfer.ErrLoop):
		return transferLoop
	case errors.Is(err, automationtransfer.ErrUnknownDestination):
		return transferUnknown
	case errors.Is(err, automationtransfer.ErrNoteMissing):
		return transferNoNote
	case errors.Is(err, automationtransfer.ErrNotGoverning):
		return transferNotYours
	case errors.Is(err, automationtransfer.ErrStageInvalid):
		return transferStageInvalid
	case errors.Is(err, automationtransfer.ErrTargetUnusable), errors.Is(err, conversation.ErrAutomationUnusable),
		errors.Is(err, automationtransfer.ErrSelf), errors.Is(err, automationtransfer.ErrConfigUnreadable),
		errors.Is(err, workflow.ErrWorkflowNotPublished), errors.Is(err, workflow.ErrWorkflowNotActive):
		return transferTargetDown
	}
	return transferFailed
}

var bindingCodes = []struct {
	err  error
	code string
}{
	{automationtransfer.ErrNoDestinations, agenttool.CodeTransferNoDestinations},
	{automationtransfer.ErrSelf, agenttool.CodeTransferSelf},
	{automationtransfer.ErrDuplicateDestination, agenttool.CodeTransferDuplicate},
	{workflow.ErrWorkflowNotPublished, agenttool.CodeTransferWorkflowDraft},
	{workflow.ErrWorkflowNotActive, agenttool.CodeTransferWorkflowPaused},
	{automationtransfer.ErrTargetUnusable, agenttool.CodeTransferTargetUnusable},
	{automationtransfer.ErrStageInvalid, agenttool.CodeTransferStageInvalid},
	{automationtransfer.ErrVariableUnmapped, agenttool.CodeTransferVariableUnmapped},
	{automationtransfer.ErrVariableNotNeeded, agenttool.CodeTransferVariableNotNeeded},
	{automationtransfer.ErrVariableSource, agenttool.CodeTransferVariableSource},
}

var variableMessages = map[string]string{
	agenttool.CodeTransferVariableUnmapped:  "o destino agora precisa de %s",
	agenttool.CodeTransferVariableNotNeeded: "o destino não precisa mais de %s",
	agenttool.CodeTransferVariableSource:    "a origem de %s não é válida",
}

var bindingMessages = map[string]string{
	agenttool.CodeTransferNoDestinations: "escolha pelo menos um destino",
	agenttool.CodeTransferSelf:           "o agente não pode transferir para si mesmo",
	agenttool.CodeTransferDuplicate:      "dois destinos levam à mesma automação e etapa",
	agenttool.CodeTransferWorkflowDraft:  "o fluxo não está publicado",
	agenttool.CodeTransferWorkflowPaused: "o fluxo está pausado",
	agenttool.CodeTransferTargetUnusable: "a automação não existe, está desligada ou é um fluxo de voz",
	agenttool.CodeTransferStageInvalid:   "a etapa não é uma etapa de funil de conversas deste workspace",
	agenttool.CodeConfigInvalid:          "os destinos não puderam ser lidos",
}

func (t *transferToAutomationTool) ValidateBinding(ctx context.Context, workspaceID string, config map[string]interface{}) error {
	self := ""
	if current, ok := agentctx.AgentFromContext(ctx); ok && current != nil {
		self = current.ID
	}
	err := t.transfers.CheckBinding(workspaceID, self, config)
	if err == nil || !automationtransfer_usecase.Problem(err) {
		return err
	}
	code := agenttool.CodeConfigInvalid
	for _, known := range bindingCodes {
		if errors.Is(err, known.err) {
			code = known.code
			break
		}
	}
	message := bindingMessages[code]
	var named *conversationvars.VariablesError
	if template, about := variableMessages[code]; about && errors.As(err, &named) {
		message = fmt.Sprintf(template, strings.Join(named.Names, ", "))
	}
	if position, ok := automationtransfer.PositionOf(err); ok {
		message = fmt.Sprintf("destino %d: %s", position, message)
	}
	return tools.Invalid(code, message)
}

func (t *transferToAutomationTool) NormalizeConfig(config map[string]interface{}) (map[string]interface{}, error) {
	normalized, err := automationtransfer.Normalize(config, func() string { return uuid.NewString()[:8] })
	if err != nil {
		return nil, err
	}
	for key, value := range config {
		if key != automationtransfer.DestinationsKey {
			normalized[key] = value
		}
	}
	return normalized, nil
}

var (
	_ tools.ContextualHandler = (*transferToAutomationTool)(nil)
	_ tools.BindingValidator  = (*transferToAutomationTool)(nil)
	_ tools.ConfigNormalizer  = (*transferToAutomationTool)(nil)
)
