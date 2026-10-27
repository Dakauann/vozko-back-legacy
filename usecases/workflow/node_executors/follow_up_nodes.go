package node_executors

import (
	"log"
	"strconv"
	"strings"

	"vozko/domain/actor"
	sm "vozko/domain/scheduled_message"
	"vozko/domain/workflow"
)

const (
	followUpScheduled = "sucesso"
	followUpRefused   = "erro"

	followUpWhenModeKey   = "when_mode"
	followUpWhenIn        = "in"
	followUpWhenDay       = "day"
	followUpWhenDate      = "date"
	followUpScopeKey      = "scope"
	followUpScopeID       = "id"
	followUpScopeMine     = "mine"
	followUpLocalLayout   = "02/01/2006 15:04"
	followUpKeyPrefix     = "follow-up:"
	followUpFallbackKey   = "fallback_template_id"
	followUpFallbackParam = "fallback_params"
)

var followUpUnitMinutes = map[string]int{"minutes": 1, "hours": 60, "days": 24 * 60}

type scheduleMessageExecutor struct {
	followUps sm.FollowUpUseCase
}

func NewScheduleMessageExecutor(followUps sm.FollowUpUseCase) workflow.NodeExecutor {
	return &scheduleMessageExecutor{followUps: followUps}
}

func (e *scheduleMessageExecutor) Definition() workflow.NodeDefinition {
	inMode := &workflow.FieldRule{Field: followUpWhenModeKey, Values: []string{followUpWhenIn}}
	dayMode := &workflow.FieldRule{Field: followUpWhenModeKey, Values: []string{followUpWhenDay}}
	dateMode := &workflow.FieldRule{Field: followUpWhenModeKey, Values: []string{followUpWhenDate}}
	clockModes := &workflow.FieldRule{Field: followUpWhenModeKey, Values: []string{followUpWhenDay, followUpWhenDate}}
	return workflow.NodeDefinition{
		Type:              workflow.NodeTypeActionScheduleMessage,
		NeedsConversation: true,
		TestBehavior:      workflow.NodeTestWritesData,
		Category:          workflow.NodeCategoryAction,
		Scopes:            []workflow.NodeScope{workflow.NodeScopeWhatsApp},
		Label:             "Agendar mensagem",
		Description:       "Agenda uma mensagem para mais tarde nesta conversa, como cobrar um comprovante. Aparece nas mensagens agendadas da conversa.",
		Icon:              "ClockCountdown",
		Guidance: workflow.NodeGuidance{
			When: "Para lembrar o contato de algo mais tarde (um comprovante, um documento, uma resposta) sem prender o fluxo esperando.",
			Behavior: "A mensagem sai no horário escolhido, no fuso do workspace, como enviada por este fluxo. Por padrão é cancelada se o contato responder antes. " +
				"Fora da janela de 24 horas do WhatsApp oficial usa o template de reserva; sem ele o agendamento vai para a saída 'erro'. " +
				"Antes de enviar o sistema confere de novo se uma pessoa assumiu a conversa e as regras de envio. Cada conversa guarda no máximo 3 mensagens pendentes agendadas por automação.",
			Examples: []string{
				"config: {\"text\":\"Oi {{lead_name}}, conseguiu enviar o comprovante?\",\"when_mode\":\"in\",\"delay_amount\":4,\"delay_unit\":\"hours\",\"cancel_on_reply\":true}",
				"config: {\"text\":\"Bom dia! Posso ajudar com o pagamento?\",\"when_mode\":\"day\",\"days_from_today\":1,\"time\":\"10:00\"}",
			},
		},
		Outputs: []workflow.HandleDefinition{
			{ID: followUpScheduled, Label: "Agendada"},
			{ID: followUpRefused, Label: "Erro", Optional: true},
		},
		OutputKeys: []workflow.OutputKeyDefinition{
			{Key: "follow_up_id", Description: "ID da mensagem agendada, para cancelar depois", Label: "ID do agendamento", Type: workflow.VariableTypeText},
			{Key: "scheduled_at", Description: "Horário do envio em UTC (ISO 8601)", Label: "Horário (UTC)", Type: workflow.VariableTypeText},
			{Key: "scheduled_for", Description: "Horário do envio no fuso do workspace (DD/MM/AAAA HH:MM)", Label: "Horário local", Type: workflow.VariableTypeText},
			{Key: "kind", Description: "text ou template", Label: "Tipo", Type: workflow.VariableTypeText},
			{Key: "error", Description: "Descrição da recusa", Label: "Recusa", Type: workflow.VariableTypeText},
			{Key: "error_code", Description: "Código da recusa", Label: "Código da recusa", Type: workflow.VariableTypeText},
		},
		DefaultConfig: map[string]interface{}{
			"text":              "",
			followUpWhenModeKey: followUpWhenIn,
			"delay_amount":      float64(4),
			"delay_unit":        "hours",
			"days_from_today":   float64(1),
			"time":              "10:00",
			"date":              "",
			followUpFallbackKey: "",
			"cancel_on_reply":   true,
		},
		ConfigSchema: []workflow.ConfigField{
			{Key: "text", Label: "Mensagem", Type: "textarea", Placeholder: "Oi {{lead_name}}, conseguiu enviar o comprovante?", Required: true},
			{Key: followUpWhenModeKey, Label: "Quando", Type: "select", Required: true, Options: []workflow.ConfigFieldOption{
				{Value: followUpWhenIn, Label: "Daqui a um tempo"},
				{Value: followUpWhenDay, Label: "Daqui a alguns dias, num horário"},
				{Value: followUpWhenDate, Label: "Numa data e horário"},
			}},
			{Key: "delay_amount", Label: "Quanto tempo", Type: "number", RequiredWhen: inMode, VisibleWhen: inMode},
			{Key: "delay_unit", Label: "Unidade", Type: "select", VisibleWhen: inMode, Options: []workflow.ConfigFieldOption{
				{Value: "minutes", Label: "Minutos"}, {Value: "hours", Label: "Horas"}, {Value: "days", Label: "Dias"},
			}},
			{Key: "days_from_today", Label: "Dias a partir de hoje", Type: "number", Description: "0 hoje, 1 amanhã.", VisibleWhen: dayMode},
			{Key: "date", Label: "Data", Type: "text", Placeholder: "AAAA-MM-DD ou {{var.data}}", RequiredWhen: dateMode, VisibleWhen: dateMode},
			{Key: "time", Label: "Horário", Type: "text", Placeholder: "HH:MM", Description: "No fuso do workspace.", RequiredWhen: clockModes, VisibleWhen: clockModes},
			{Key: followUpFallbackKey, Label: "Template de reserva", Type: "select", OptionsSource: "whatsapp_templates",
				Description: "Só no WhatsApp oficial: enviado quando o horário cair fora da janela de 24 horas."},
			{Key: followUpFallbackParam, Label: "Valores do template", Type: "tags", Description: "As variáveis do template de reserva, na ordem. Aceita variáveis do fluxo {{...}}."},
			{Key: "cancel_on_reply", Label: "Cancelar se o contato responder", Type: "boolean",
				Description: "Ligado: qualquer mensagem do contato antes do horário cancela o envio."},
		},
	}
}

func (e *scheduleMessageExecutor) Execute(ctx *workflow.NodeContext) (*workflow.NodeResult, error) {
	if ctx == nil || ctx.Run == nil || ctx.Node == nil {
		return nil, workflow.ErrNodeConfigMissing
	}
	if ctx.Run.OnLead() {
		return followUpNodeRefused(ctx, sm.ErrFollowUpNeedsConversation), nil
	}
	text, _ := ctx.Node.Config["text"].(string)
	if strings.TrimSpace(text) == "" {
		return nil, workflow.ErrNodeConfigMissing
	}
	when, missing := followUpWhenFrom(ctx)
	fallback, missingParams := followUpFallbackFrom(ctx)
	if missing = append(missing, missingParams...); len(missing) > 0 {
		return followUpNodeRefusedWith(ctx, sm.ErrFollowUpVariableMissing, "variáveis sem valor: "+strings.Join(missing, ", ")), nil
	}

	callCtx, release := ctx.CallContext()
	defer release()
	scheduled, err := e.followUps.Schedule(callCtx, sm.FollowUpInput{
		WorkspaceID:    ctx.Run.WorkspaceID,
		EntryID:        ctx.Run.EntryID,
		EntryType:      ctx.Run.EntryType,
		Author:         actor.FormatWorkflow(ctx.Run.WorkflowID),
		Text:           strings.TrimSpace(workflow.CustomerText(ctx, text)),
		When:           when,
		Fallback:       fallback,
		CancelOnReply:  configBool(ctx.Node.Config, "cancel_on_reply", true),
		IdempotencyKey: followUpKeyPrefix + ctx.Run.ID + ":" + ctx.Run.NodeVisit(),
	})
	if err != nil {
		return followUpNodeFailure(ctx, err)
	}
	m := scheduled.Message
	output := map[string]interface{}{
		"follow_up_id":  m.ID,
		"scheduled_at":  m.ScheduledAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"scheduled_for": scheduled.Local.Format(followUpLocalLayout),
		"kind":          string(m.Kind),
		"replayed":      scheduled.Replayed,
	}
	return routeOutput(&workflow.NodeResult{Output: output}, outgoingEdges(ctx), followUpScheduled), nil
}

func followUpWhenFrom(ctx *workflow.NodeContext) (sm.FollowUpWhen, []string) {
	config := ctx.Node.Config
	mode, _ := config[followUpWhenModeKey].(string)
	switch mode {
	case followUpWhenDay:
		clock, missing := strictConfigText(ctx, "time")
		days := configInt(config, "days_from_today")
		return sm.FollowUpWhen{DaysFromToday: &days, Time: clock}, missing
	case followUpWhenDate:
		date, missingDate := strictConfigText(ctx, "date")
		clock, missingTime := strictConfigText(ctx, "time")
		return sm.FollowUpWhen{Date: date, Time: clock}, append(missingDate, missingTime...)
	}
	unit, _ := config["delay_unit"].(string)
	perUnit, ok := followUpUnitMinutes[unit]
	if !ok {
		perUnit = 1
	}
	return sm.FollowUpWhen{InMinutes: configInt(config, "delay_amount") * perUnit}, nil
}

func followUpFallbackFrom(ctx *workflow.NodeContext) (*sm.TemplateContent, []string) {
	id, _ := ctx.Node.Config[followUpFallbackKey].(string)
	if id = strings.TrimSpace(id); id == "" {
		return nil, nil
	}
	raw, _ := ctx.Node.Config[followUpFallbackParam].([]interface{})
	values := make([]string, 0, len(raw))
	var missing []string
	for _, item := range raw {
		resolved, unresolved := workflow.InterpolateStrict(workflow.TemplateText(item), ctx.State, nil)
		missing = append(missing, unresolved...)
		values = append(values, strings.TrimSpace(resolved))
	}
	return &sm.TemplateContent{ID: id, BodyParams: values}, missing
}

func strictConfigText(ctx *workflow.NodeContext, key string) (string, []string) {
	raw, _ := ctx.Node.Config[key].(string)
	resolved, unresolved := workflow.InterpolateStrict(raw, ctx.State, nil)
	return strings.TrimSpace(resolved), unresolved
}

func configInt(config map[string]interface{}, key string) int {
	switch v := config[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return n
		}
	}
	return 0
}

func configBool(config map[string]interface{}, key string, fallback bool) bool {
	if value, ok := config[key].(bool); ok {
		return value
	}
	return fallback
}

type cancelScheduledMessagesExecutor struct {
	followUps sm.FollowUpUseCase
}

func NewCancelScheduledMessagesExecutor(followUps sm.FollowUpUseCase) workflow.NodeExecutor {
	return &cancelScheduledMessagesExecutor{followUps: followUps}
}

func (e *cancelScheduledMessagesExecutor) Definition() workflow.NodeDefinition {
	byID := &workflow.FieldRule{Field: followUpScopeKey, Values: []string{followUpScopeID}}
	return workflow.NodeDefinition{
		Type:              workflow.NodeTypeActionCancelScheduledMessages,
		NeedsConversation: true,
		TestBehavior:      workflow.NodeTestWritesData,
		Category:          workflow.NodeCategoryAction,
		Scopes:            []workflow.NodeScope{workflow.NodeScopeWhatsApp},
		Label:             "Cancelar mensagens agendadas",
		Description:       "Cancela mensagens que este fluxo agendou nesta conversa e que ainda não foram enviadas.",
		Icon:              "X",
		Guidance: workflow.NodeGuidance{
			When: "Quando o motivo de um agendamento deixou de existir, por exemplo quando o comprovante chegou por outro caminho.",
			Behavior: "Escolha uma mensagem pelo ID devolvido pelo nó Agendar mensagem ({{node.<id>.follow_up_id}}) ou todas as pendentes que este fluxo agendou nesta conversa. " +
				"Uma mensagem já enviada não é cancelada.",
			Examples: []string{
				"config: {\"scope\":\"id\",\"follow_up_id\":\"{{node.n4.follow_up_id}}\"}",
				"config: {\"scope\":\"mine\"}",
			},
		},
		Outputs: []workflow.HandleDefinition{
			{ID: followUpScheduled, Label: "Cancelada"},
			{ID: followUpRefused, Label: "Erro", Optional: true},
		},
		OutputKeys: []workflow.OutputKeyDefinition{
			{Key: "cancelled", Description: "Quantas mensagens foram canceladas", Label: "Canceladas", Type: workflow.VariableTypeNumber},
			{Key: "cancelled_ids", Description: "IDs das mensagens canceladas", Label: "IDs cancelados", Type: workflow.VariableTypeList},
			{Key: "error", Description: "Descrição da recusa", Label: "Recusa", Type: workflow.VariableTypeText},
			{Key: "error_code", Description: "Código da recusa", Label: "Código da recusa", Type: workflow.VariableTypeText},
		},
		DefaultConfig: map[string]interface{}{followUpScopeKey: followUpScopeMine, "follow_up_id": ""},
		ConfigSchema: []workflow.ConfigField{
			{Key: followUpScopeKey, Label: "O que cancelar", Type: "select", Required: true, Options: []workflow.ConfigFieldOption{
				{Value: followUpScopeMine, Label: "Todas que este fluxo agendou nesta conversa"},
				{Value: followUpScopeID, Label: "Uma mensagem pelo ID"},
			}},
			{Key: "follow_up_id", Label: "ID da mensagem agendada", Type: "text", Placeholder: "{{node.n4.follow_up_id}}", RequiredWhen: byID, VisibleWhen: byID},
		},
	}
}

func (e *cancelScheduledMessagesExecutor) Execute(ctx *workflow.NodeContext) (*workflow.NodeResult, error) {
	if ctx == nil || ctx.Run == nil || ctx.Node == nil {
		return nil, workflow.ErrNodeConfigMissing
	}
	if ctx.Run.OnLead() {
		return followUpNodeRefused(ctx, sm.ErrFollowUpNeedsConversation), nil
	}
	id := ""
	if scope, _ := ctx.Node.Config[followUpScopeKey].(string); scope == followUpScopeID {
		resolved, missing := strictConfigText(ctx, "follow_up_id")
		if len(missing) > 0 || resolved == "" {
			return followUpNodeRefusedWith(ctx, sm.ErrFollowUpVariableMissing, "o ID da mensagem agendada está vazio"), nil
		}
		id = resolved
	}

	callCtx, release := ctx.CallContext()
	defer release()
	cancelled, err := e.followUps.Cancel(callCtx, sm.FollowUpCancel{
		WorkspaceID: ctx.Run.WorkspaceID,
		EntryID:     ctx.Run.EntryID,
		EntryType:   ctx.Run.EntryType,
		Author:      actor.FormatWorkflow(ctx.Run.WorkflowID),
		ID:          id,
	})
	if err != nil {
		return followUpNodeFailure(ctx, err)
	}
	ids := make([]interface{}, 0, len(cancelled))
	for _, m := range cancelled {
		ids = append(ids, m.ID)
	}
	output := map[string]interface{}{"cancelled": len(ids), "cancelled_ids": ids}
	return routeOutput(&workflow.NodeResult{Output: output}, outgoingEdges(ctx), followUpScheduled), nil
}

func followUpNodeFailure(ctx *workflow.NodeContext, err error) (*workflow.NodeResult, error) {
	if sm.FollowUpErrorCode(err) == "follow_up_unavailable" && canRetryNode(ctx) {
		log.Printf("[workflow][node:%s][run:%s] %s will retry with the same key: %v", ctx.Node.ID, ctx.Run.ID, ctx.Node.Type, err)
		return nil, err
	}
	return followUpNodeRefused(ctx, err), nil
}

func followUpNodeRefused(ctx *workflow.NodeContext, err error) *workflow.NodeResult {
	return followUpNodeRefusedWith(ctx, err, err.Error())
}

func followUpNodeRefusedWith(ctx *workflow.NodeContext, err error, message string) *workflow.NodeResult {
	output := map[string]interface{}{"error": message, "error_code": sm.FollowUpErrorCode(err)}
	return routeFailure(&workflow.NodeResult{Output: output}, outgoingEdges(ctx), followUpRefused)
}
