package automationneedsx

import (
	"errors"
	"net/http"

	"vozko/delivery/http/response"
	domain "vozko/domain/automationneeds"
	"vozko/domain/workflow"
)

type LinkResponse struct {
	Kind string `json:"kind" enums:"agent,workflow" example:"workflow"`
	ID   string `json:"id" example:"5b1f0c9e-2a4d-4c55-9d3e-1f2a3b4c5d6e"`
	Name string `json:"name" example:"Cobrança"`
}

type NeedTypeResponse struct {
	Kind    string   `json:"kind" example:"choice"`
	Shape   string   `json:"shape" enums:"text,number,date,choice,sheet" example:"choice"`
	Options []string `json:"options" example:"Manhã,Tarde"`
}

type NeedResponse struct {
	Name        string           `json:"name" example:"nome_cliente"`
	Label       string           `json:"label" example:"Nome do cliente"`
	Declared    bool             `json:"declared"`
	Type        NeedTypeResponse `json:"type"`
	Example     string           `json:"example,omitempty" example:"Ana Souza"`
	Description string           `json:"description,omitempty" example:"Nome do cliente na planilha"`
	Via         []LinkResponse   `json:"via"`
}

type SlotResponse struct {
	Kind string `json:"kind" enums:"whatsapp_number,unofficial_whatsapp,instagram,facebook,telegram,webchat,tiktok,mercadolivre,email,rcs,sms" example:"instagram"`
	ID   string `json:"id" example:"3f1c2d4e-5a6b-4c7d-8e9f-0a1b2c3d4e5f"`
	Name string `json:"name" example:"@loja"`
	Path string `json:"path" example:"/dashboard/instagram-accounts/3f1c2d4e-5a6b-4c7d-8e9f-0a1b2c3d4e5f"`
}

type SwitchOffResponse struct {
	Channel    SlotResponse   `json:"channel"`
	Automation LinkResponse   `json:"automation"`
	Variables  []NeedResponse `json:"variables"`
}

type ChannelResponse struct {
	Kind  string `json:"kind" enums:"whatsapp,unofficial_whatsapp,instagram,facebook,telegram,webchat,tiktok,mercadolivre,email,rcs,sms" example:"instagram"`
	Label string `json:"label" example:"Instagram"`
}

type ChannelStepResponse struct {
	Automation LinkResponse      `json:"automation"`
	NodeID     string            `json:"nodeId" example:"n3"`
	Step       string            `json:"step" example:"Enviar boleto"`
	Need       string            `json:"need" enums:"contact_phone,contact_email,phone_handle,whatsapp_templates,whatsapp_forms,interactive_choices" example:"whatsapp_templates"`
	NeedLabel  string            `json:"needLabel" example:"templates do WhatsApp oficial"`
	Channels   []ChannelResponse `json:"channels"`
}

type RefusalDetails struct {
	Automation   LinkResponse          `json:"automation"`
	Variables    []NeedResponse        `json:"variables"`
	Values       map[string]string     `json:"values,omitempty"`
	Impact       []SwitchOffResponse   `json:"impact,omitempty"`
	ChannelSteps []ChannelStepResponse `json:"channelSteps,omitempty"`
}

func Link(l domain.Link) LinkResponse {
	return LinkResponse{Kind: string(l.Kind), ID: l.ID, Name: l.Name}
}

func Needs(needs domain.Needs) []NeedResponse {
	out := make([]NeedResponse, 0, len(needs))
	for _, need := range needs {
		via := make([]LinkResponse, 0, len(need.Via))
		for _, link := range need.Via {
			via = append(via, Link(link))
		}
		options := need.Type.Options
		if options == nil {
			options = []string{}
		}
		kind, shape := string(need.Type.Kind), string(need.Shape)
		if kind == "" {
			kind, shape = "text", "text"
		}
		out = append(out, NeedResponse{
			Name: need.Name, Label: need.Display(), Declared: need.Declared,
			Type:        NeedTypeResponse{Kind: kind, Shape: shape, Options: options},
			Example:     need.Example,
			Description: need.Description, Via: via,
		})
	}
	return out
}

func Slot(s domain.Slot) SlotResponse {
	return SlotResponse{Kind: string(s.Kind), ID: s.ID, Name: s.Name, Path: domain.SlotPath(s.Kind, s.ID)}
}

func Impact(offs []domain.SwitchOff) []SwitchOffResponse {
	out := make([]SwitchOffResponse, 0, len(offs))
	for _, off := range offs {
		out = append(out, SwitchOffResponse{Channel: Slot(off.Slot), Automation: Link(off.Automation), Variables: Needs(off.Needs)})
	}
	return out
}

var statuses = map[string]int{
	"automation_variables_unmet":      http.StatusConflict,
	"automation_variables_missing":    http.StatusConflict,
	"automation_publish_switches_off": http.StatusConflict,
	"automation_voice_variables":      http.StatusUnprocessableEntity,
	"automation_test_values_missing":  http.StatusUnprocessableEntity,
	"automation_sheets_unmet":         http.StatusConflict,
	"automation_channel_unsupported":  http.StatusConflict,
	"automation_needs_not_found":      http.StatusUnprocessableEntity,
	"automation_needs_unreadable":     http.StatusServiceUnavailable,
}

var messages = map[string]string{
	"automation_variables_unmet":      "Esta automação precisa de variáveis que conversas receptivas não trazem.",
	"automation_variables_missing":    "Esta conversa não tem valor para variáveis que a automação precisa.",
	"automation_publish_switches_off": "Publicar desliga a automação em canais receptivos. Confirme para seguir.",
	"automation_voice_variables":      "Ligações não trazem variáveis, então um fluxo de voz não pode precisar delas.",
	"automation_test_values_missing":  "Preencha os dados do teste: o fluxo precisa dessas variáveis.",
	"automation_sheets_unmet":         "A planilha escolhida para esta conversa não serve para os passos que gravam nela.",
	"automation_channel_unsupported":  "Um passo desta automação nunca funciona no canal em que ela rodaria.",
	"automation_needs_not_found":      "Um agente ou fluxo usado por esta automação não existe mais neste workspace.",
	"automation_needs_unreadable":     "Não foi possível conferir as variáveis da automação agora. Tente de novo.",
}

func Status(code string) int {
	return statuses[code]
}

func WriteRefusal(w http.ResponseWriter, err error) bool {
	code := domain.ErrorCode(err)
	if code == "" {
		return false
	}
	refusal, detailed := domain.RefusalOf(err)
	if !detailed {
		response.WriteErrorWithCode(w, statuses[code], code, messages[code], nil)
		return true
	}
	response.WriteErrorWithDetails(w, statuses[code], code, messages[code], Details(refusal))
	return true
}

func Details(r *domain.Refusal) RefusalDetails {
	return RefusalDetails{Automation: Link(r.Automation), Variables: Needs(r.Needs), Values: r.Values, Impact: Impact(r.Impact), ChannelSteps: ChannelSteps(r.Channels)}
}

func ChannelSteps(blocks []domain.ChannelBlock) []ChannelStepResponse {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]ChannelStepResponse, 0, len(blocks))
	for _, block := range blocks {
		channels := make([]ChannelResponse, 0, len(block.Channels))
		for _, channel := range block.Channels {
			channels = append(channels, ChannelResponse{Kind: string(channel), Label: workflow.ChannelBranchLabel(channel)})
		}
		out = append(out, ChannelStepResponse{
			Automation: Link(block.Automation), NodeID: block.NodeID, Step: block.Step,
			Need: string(block.Fact), NeedLabel: block.Fact.Noun(), Channels: channels,
		})
	}
	return out
}

var errUnknownCode = errors.New("automation needs: unknown code")

func CheckCodes() error {
	for _, code := range domain.ErrorCodes() {
		if statuses[code] == 0 || messages[code] == "" {
			return errUnknownCode
		}
	}
	return nil
}
