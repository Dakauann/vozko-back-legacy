package automationneedsx

import (
	"errors"
	"net/http"

	"vozko/delivery/http/response"
	domain "vozko/domain/automationneeds"
)

type LinkResponse struct {
	Kind string `json:"kind" enums:"agent,workflow" example:"workflow"`
	ID   string `json:"id" example:"5b1f0c9e-2a4d-4c55-9d3e-1f2a3b4c5d6e"`
	Name string `json:"name" example:"Cobrança"`
}

type NeedResponse struct {
	Name        string         `json:"name" example:"nome_cliente"`
	Description string         `json:"description,omitempty" example:"Nome do cliente na planilha"`
	Via         []LinkResponse `json:"via"`
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

type RefusalDetails struct {
	Automation LinkResponse        `json:"automation"`
	Variables  []NeedResponse      `json:"variables"`
	Values     map[string]string   `json:"values,omitempty"`
	Impact     []SwitchOffResponse `json:"impact,omitempty"`
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
		out = append(out, NeedResponse{Name: need.Name, Description: need.Description, Via: via})
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
	"automation_needs_not_found":      http.StatusUnprocessableEntity,
	"automation_needs_unreadable":     http.StatusServiceUnavailable,
}

var messages = map[string]string{
	"automation_variables_unmet":      "Esta automação precisa de variáveis que conversas receptivas não trazem.",
	"automation_variables_missing":    "Esta conversa não tem valor para variáveis que a automação precisa.",
	"automation_publish_switches_off": "Publicar desliga a automação em canais receptivos. Confirme para seguir.",
	"automation_voice_variables":      "Ligações não trazem variáveis, então um fluxo de voz não pode precisar delas.",
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
	return RefusalDetails{Automation: Link(r.Automation), Variables: Needs(r.Needs), Values: r.Values, Impact: Impact(r.Impact)}
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
