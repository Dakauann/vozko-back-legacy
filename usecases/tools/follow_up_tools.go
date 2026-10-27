package tools_usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	sm "vozko/domain/scheduled_message"
	"vozko/domain/tools"
)

const (
	ScheduleFollowUpToolName = "schedule_follow_up"
	CancelFollowUpToolName   = "cancel_follow_up"

	followUpFallbackTemplateKey = "fallback_template_id"
	followUpLocalLayout         = "02/01/2006 15:04"
)

var errFollowUpParamType = errors.New("follow-up: a parameter has the wrong type")

type scheduleFollowUpTool struct {
	followUps sm.FollowUpUseCase
}

func NewScheduleFollowUpTool(followUps sm.FollowUpUseCase) tools.Handler {
	if followUps == nil {
		panic("schedule_follow_up: missing follow-up use case")
	}
	return &scheduleFollowUpTool{followUps: followUps}
}

func (t *scheduleFollowUpTool) Definition() tools.Definition {
	return tools.Definition{
		Name:               ScheduleFollowUpToolName,
		DisplayName:        "Agendar follow-up",
		DisplayDescription: "Agenda uma mensagem para mais tarde nesta conversa, como cobrar um comprovante. Por padrão é cancelada se o contato responder antes. Fora da janela de 24 horas do WhatsApp oficial usa o template de reserva, se houver.",
		Description: `Agenda uma mensagem para ser enviada mais tarde nesta conversa e devolve o follow_up_id.

QUANDO USAR:
- O cliente disse que vai fazer algo depois (enviar um comprovante, um documento, uma resposta) e você deve lembrá-lo se ele não voltar.
- O cliente pediu para ser chamado em outro dia ou horário.

QUANDO NÃO USAR:
- Para responder agora: responda normalmente.
- Quando já existe um follow-up pendente com o mesmo objetivo nesta conversa.

HORÁRIO (escolha uma forma):
- Daqui a um tempo: in_minutes (240 = 4 horas).
- Num dia e hora do fuso do workspace: time (HH:MM) com days_from_today (0 hoje, 1 amanhã) ou com date (AAAA-MM-DD).

Por padrão o follow-up é cancelado sozinho se o cliente responder antes. Guarde o follow_up_id para cancelar com cancel_follow_up quando ele deixar de fazer sentido. Recusas trazem um código e o motivo.`,
		Parameters: map[string]tools.Parameter{
			"text": {
				Type:               "string",
				Description:        "A mensagem que o cliente vai receber, escrita como se fosse enviada na hora.",
				DisplayName:        "Mensagem",
				DisplayDescription: "Texto enviado no horário agendado",
			},
			"in_minutes": {
				Type:               "integer",
				Description:        "Minutos a partir de agora. Não use junto com time.",
				DisplayName:        "Daqui a (minutos)",
				DisplayDescription: "Tempo até o envio",
			},
			"days_from_today": {
				Type:               "integer",
				Description:        "Dias a partir de hoje no fuso do workspace (0 hoje, 1 amanhã). Use com time.",
				DisplayName:        "Dias a partir de hoje",
				DisplayDescription: "0 hoje, 1 amanhã",
			},
			"date": {
				Type:               "date",
				Description:        "Data do envio no fuso do workspace. Use com time, no lugar de days_from_today.",
				DisplayName:        "Data",
				DisplayDescription: "Dia do envio",
			},
			"time": {
				Type:               "time",
				Description:        "Hora do envio no fuso do workspace.",
				DisplayName:        "Hora",
				DisplayDescription: "Hora do envio",
			},
			"cancel_if_contact_replies": {
				Type:               "boolean",
				Description:        "Cancelar se o cliente mandar qualquer mensagem antes do envio. Padrão true; use false só para um lembrete que vale mesmo se ele responder.",
				DisplayName:        "Cancelar se o contato responder",
				DisplayDescription: "Ligado por padrão",
			},
			"template_values": {
				Type:               "array",
				Items:              &tools.ParameterItems{Type: "string"},
				Description:        "Valores das variáveis do template de reserva, na ordem, usados só quando o envio cair fora da janela de 24 horas.",
				DisplayName:        "Valores do template",
				DisplayDescription: "Variáveis do template de reserva",
			},
		},
		Required: []string{"text"},
		ConfigSchema: map[string]tools.ConfigParameter{
			followUpFallbackTemplateKey: {
				Type:               "string",
				Description:        "Template aprovado do WhatsApp oficial enviado quando o horário cair fora da janela de 24 horas. Vazio: o follow-up fora da janela é recusado.",
				DisplayName:        "Template de reserva",
				DisplayDescription: "Só no WhatsApp oficial, para envios fora da janela de 24 horas.",
				OptionsSource:      "whatsapp_templates",
			},
		},
		Visibility: []tools.ToolVisibility{tools.VisibilityMessaging},
		Category:   tools.CategoryMessaging,
	}
}

func (t *scheduleFollowUpTool) Execute(ctx context.Context, params map[string]interface{}) (tools.ExecutionResult, error) {
	return t.ExecuteWithConfig(ctx, nil, params)
}

func (t *scheduleFollowUpTool) ExecuteWithConfig(ctx context.Context, config map[string]interface{}, params map[string]interface{}) (tools.ExecutionResult, error) {
	where, ok := followUpConversation(ctx, config)
	if !ok {
		return followUpOutsideConversation(), nil
	}
	when, err := followUpWhenOf(params)
	if err != nil {
		return followUpRefusal(sm.ErrFollowUpWhenInvalid), nil
	}
	text, _ := params["text"].(string)
	in := sm.FollowUpInput{
		WorkspaceID:    where.workspaceID,
		EntryID:        where.entryID,
		EntryType:      where.entryType,
		Author:         where.author,
		Text:           strings.TrimSpace(text),
		When:           when,
		Fallback:       fallbackOf(config, params),
		CancelOnReply:  cancelsOnReply(params),
		IdempotencyKey: followUpKey(where, tools.CallIDFrom(ctx)),
	}
	scheduled, err := t.followUps.Schedule(ctx, in)
	if err != nil {
		return followUpRefusal(err), nil
	}
	return tools.ExecutionResult{Result: scheduledResult(scheduled)}, nil
}

func scheduledResult(s *sm.FollowUpScheduled) map[string]interface{} {
	m := s.Message
	return map[string]interface{}{
		"success":                   true,
		"follow_up_id":              m.ID,
		"scheduled_for":             s.Local.Format(followUpLocalLayout),
		"time_zone":                 s.Local.Location().String(),
		"kind":                      string(m.Kind),
		"cancel_if_contact_replies": m.CancelOnReply,
		"already_scheduled":         s.Replayed,
	}
}

type cancelFollowUpTool struct {
	followUps sm.FollowUpUseCase
}

func NewCancelFollowUpTool(followUps sm.FollowUpUseCase) tools.Handler {
	if followUps == nil {
		panic("cancel_follow_up: missing follow-up use case")
	}
	return &cancelFollowUpTool{followUps: followUps}
}

func (t *cancelFollowUpTool) Definition() tools.Definition {
	return tools.Definition{
		Name:               CancelFollowUpToolName,
		DisplayName:        "Cancelar follow-up",
		DisplayDescription: "Cancela um follow-up que este agente agendou na conversa, ou todos os pendentes dele, por exemplo quando o cliente já enviou o comprovante.",
		Description: `Cancela follow-ups que você agendou nesta conversa e que ainda não foram enviados.

QUANDO USAR:
- O motivo do follow-up deixou de existir (o cliente já enviou o comprovante, já respondeu, desistiu).
- O cliente pediu para não ser lembrado.

Informe follow_up_id para cancelar um só, ou all: true para cancelar todos os seus follow-ups pendentes nesta conversa.`,
		Parameters: map[string]tools.Parameter{
			"follow_up_id": {
				Type:               "string",
				Description:        "O follow_up_id devolvido por schedule_follow_up.",
				DisplayName:        "Follow-up",
				DisplayDescription: "Qual follow-up cancelar",
			},
			"all": {
				Type:               "boolean",
				Description:        "true para cancelar todos os seus follow-ups pendentes nesta conversa.",
				DisplayName:        "Todos",
				DisplayDescription: "Cancelar todos os pendentes deste agente",
			},
		},
		Required:   []string{},
		Visibility: []tools.ToolVisibility{tools.VisibilityMessaging},
		Category:   tools.CategoryMessaging,
	}
}

func (t *cancelFollowUpTool) Execute(ctx context.Context, params map[string]interface{}) (tools.ExecutionResult, error) {
	return t.ExecuteWithConfig(ctx, nil, params)
}

func (t *cancelFollowUpTool) ExecuteWithConfig(ctx context.Context, config map[string]interface{}, params map[string]interface{}) (tools.ExecutionResult, error) {
	where, ok := followUpConversation(ctx, config)
	if !ok {
		return followUpOutsideConversation(), nil
	}
	id, _ := params["follow_up_id"].(string)
	id = strings.TrimSpace(id)
	all, _ := params["all"].(bool)
	if id == "" && !all {
		return toolRefusal("Informe follow_up_id ou all: true."), nil
	}
	if id != "" {
		all = false
	}
	cancelled, err := t.followUps.Cancel(ctx, sm.FollowUpCancel{
		WorkspaceID: where.workspaceID, EntryID: where.entryID, EntryType: where.entryType, Author: where.author, ID: id,
	})
	if err != nil {
		return followUpRefusal(err), nil
	}
	ids := make([]string, 0, len(cancelled))
	for _, m := range cancelled {
		ids = append(ids, m.ID)
	}
	return tools.ExecutionResult{Result: map[string]interface{}{
		"success":       true,
		"cancelled_ids": ids,
		"cancelled":     len(ids),
	}}, nil
}

type followUpPlace struct {
	workspaceID string
	entryID     string
	entryType   string
	author      string
	agentID     string
}

func followUpConversation(ctx context.Context, config map[string]interface{}) (followUpPlace, bool) {
	place := followUpPlace{
		workspaceID: configString(config, "__workspace_id"),
		entryID:     configString(config, "__entry_id"),
		entryType:   configString(config, "__entry_type"),
		agentID:     agentIDOf(ctx, config),
	}
	place.author = agentActor(ctx, config)
	ok := place.workspaceID != "" && place.entryID != "" && place.entryType != "" && sm.ValidAutomationAuthor(place.author)
	return place, ok
}

func followUpOutsideConversation() tools.ExecutionResult {
	return toolRefusal("Não foi possível identificar a conversa atual. Esta ferramenta só funciona quando um agente atende uma conversa.")
}

func followUpKey(where followUpPlace, callID string) string {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return ""
	}
	return "follow-up:" + where.agentID + ":" + callID
}

func followUpWhenOf(params map[string]interface{}) (sm.FollowUpWhen, error) {
	minutes, _, err := wholeNumber(params, "in_minutes")
	if err != nil {
		return sm.FollowUpWhen{}, err
	}
	when := sm.FollowUpWhen{InMinutes: minutes}
	days, present, err := wholeNumber(params, "days_from_today")
	if err != nil {
		return sm.FollowUpWhen{}, err
	}
	if present {
		when.DaysFromToday = &days
	}
	date, _ := params["date"].(string)
	clock, _ := params["time"].(string)
	when.Date, when.Time = strings.TrimSpace(date), strings.TrimSpace(clock)
	return when, nil
}

func wholeNumber(params map[string]interface{}, key string) (int, bool, error) {
	raw, ok := params[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	switch v := raw.(type) {
	case float64:
		if v != math.Trunc(v) {
			return 0, true, errFollowUpParamType
		}
		return int(v), true, nil
	case int:
		return v, true, nil
	}
	return 0, true, errFollowUpParamType
}

func cancelsOnReply(params map[string]interface{}) bool {
	if value, ok := params["cancel_if_contact_replies"].(bool); ok {
		return value
	}
	return true
}

func fallbackOf(config, params map[string]interface{}) *sm.TemplateContent {
	id := configString(config, followUpFallbackTemplateKey)
	if id == "" {
		return nil
	}
	return &sm.TemplateContent{ID: id, BodyParams: stringValues(params["template_values"])}
}

func stringValues(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, strings.TrimSpace(fmt.Sprint(item)))
	}
	return out
}

func followUpRefusal(err error) tools.ExecutionResult {
	code := sm.FollowUpErrorCode(err)
	result := toolRefusal(followUpAdvice(code))
	result.ErrorCode = code
	return result
}

var followUpAdvices = map[string]string{
	"follow_up_when_required":        "Informe in_minutes, ou time com days_from_today ou date.",
	"follow_up_when_ambiguous":       "Use só uma forma de horário: in_minutes, ou time com days_from_today, ou time com date.",
	"follow_up_when_invalid":         "Horário inválido: in_minutes e days_from_today são números inteiros, date é AAAA-MM-DD e time é HH:MM.",
	"follow_up_too_soon":             "O horário já passou ou está perto demais. Escolha um horário a partir de alguns minutos.",
	"follow_up_too_far":              "O horário passa de 30 dias. Escolha uma data mais próxima.",
	"follow_up_past_window":          "O horário cai depois do fim da janela de atendimento e não há template de reserva. Escolha um horário mais próximo.",
	"follow_up_window_closed":        "A janela de atendimento está fechada e não há template de reserva, então não dá para agendar.",
	"follow_up_limit_reached":        "Esta conversa já tem o máximo de follow-ups pendentes. Cancele um com cancel_follow_up antes de agendar outro.",
	"follow_up_not_allowed":          "Uma pessoa da equipe está cuidando desta conversa; não agende follow-ups.",
	"follow_up_needs_conversation":   "Só é possível agendar dentro de uma conversa.",
	"follow_up_variable_missing":     "Um valor usado no agendamento está vazio.",
	"follow_up_not_found":            "Esse follow_up_id não é de um follow-up seu nesta conversa.",
	"follow_up_not_pending":          "Esse follow-up já foi enviado ou cancelado.",
	"follow_up_text_required":        "Escreva a mensagem do follow-up em text.",
	"follow_up_contact_ineligible":   "Este contato não pode receber esta mensagem (bloqueado, pediu para não receber ou recebeu um template há pouco).",
	"follow_up_template_unavailable": "O template de reserva não pode ser usado nesta conversa ou os valores não batem com as variáveis dele.",
	"follow_up_unavailable":          "Não foi possível agendar agora. Siga a conversa sem o follow-up.",
}

func followUpAdvice(code string) string {
	if advice, ok := followUpAdvices[code]; ok {
		return advice + " (" + code + ")"
	}
	return followUpAdvices["follow_up_unavailable"] + " (" + code + ")"
}

var (
	_ tools.Handler = (*scheduleFollowUpTool)(nil)
	_ tools.Handler = (*cancelFollowUpTool)(nil)
)
