package automationneedshttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"vozko/delivery/http/automationneedsx"
	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	domain "vozko/domain/automationneeds"
	"vozko/domain/campaign"
	"vozko/domain/campaignaudience"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	"vozko/domain/shared"
	"vozko/domain/user"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
	automationneeds_usecase "vozko/usecases/automationneeds"
)

type Queries interface {
	Eligibility(workspaceID string, asked []conversation.Automation) ([]automationneeds_usecase.Eligibility, error)
	PublishImpact(ctx context.Context, workspaceID string, a conversation.Automation) ([]domain.SwitchOff, error)
	OpenSwitchOff(ctx context.Context, workspaceID string, kind domain.SlotKind, slotID string) (*automationneeds_usecase.OpenSwitchOff, error)
	VariablesForm(ctx context.Context, in automationneeds_usecase.ConversationRef, target *conversation.Automation) (automationneeds_usecase.VariablesForm, error)
	SaveConversationValues(ctx context.Context, in automationneeds_usecase.SaveValues) error
}

type Handler struct {
	queries Queries
}

func NewHandler(queries Queries) *Handler {
	if queries == nil {
		panic("automationneedshttp: the handler needs the queries")
	}
	return &Handler{queries: queries}
}

type EligibilityResponse struct {
	Automation        automationneedsx.LinkResponse   `json:"automation"`
	Variables         []automationneedsx.NeedResponse `json:"variables"`
	ReceptiveEligible bool                            `json:"receptiveEligible"`
	Readable          bool                            `json:"readable"`
}

type EligibilityListResponse struct {
	Items []EligibilityResponse `json:"items"`
}

type ImpactResponse struct {
	Impact []automationneedsx.SwitchOffResponse `json:"impact"`
}

type SwitchOffNoticeResponse struct {
	Channel       automationneedsx.SlotResponse   `json:"channel"`
	Automation    automationneedsx.LinkResponse   `json:"automation"`
	Variables     []string                        `json:"variables"`
	StillNeeded   []automationneedsx.NeedResponse `json:"stillNeeded"`
	SwitchedOffAt time.Time                       `json:"switchedOffAt"`
	EligibleNow   bool                            `json:"eligibleNow"`
}

type SwitchOffResponse struct {
	Notice *SwitchOffNoticeResponse `json:"notice"`
}

type AutomationRequest struct {
	Kind string `json:"kind" enums:"agent,workflow" example:"agent"`
	ID   string `json:"id" example:"5b1f0c9e-2a4d-4c55-9d3e-1f2a3b4c5d6e"`
}

type VariableBindingRequest struct {
	Name   string `json:"name" example:"matricula"`
	Source string `json:"source" example:"lead.custom:matricula"`
	Value  string `json:"value,omitempty" example:"2026-17"`
}

type ConversationVariablesRequest struct {
	Automation AutomationRequest        `json:"automation"`
	Values     map[string]string        `json:"values,omitempty"`
	Bindings   []VariableBindingRequest `json:"bindings,omitempty"`
}

type VariableRowResponse struct {
	Name        string                          `json:"name" example:"matricula"`
	Description string                          `json:"description,omitempty" example:"Matrícula do aluno"`
	Via         []automationneedsx.LinkResponse `json:"via"`
	Current     string                          `json:"current,omitempty" example:"2026-17"`
	Suggested   string                          `json:"suggested,omitempty" example:"lead.custom:matricula"`
}

type ConversationFieldResponse struct {
	campaignaudience.Field
	Value string `json:"value" example:"Dakauann"`
}

type ConversationVariablesResponse struct {
	Automation *automationneedsx.LinkResponse `json:"automation"`
	Variables  []VariableRowResponse          `json:"variables"`
	Fields     []ConversationFieldResponse    `json:"fields"`
}

type VariablesErrorDetails struct {
	Variables []string `json:"variables" example:"matricula"`
}

func idsOf(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if id := strings.TrimSpace(part); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// @Summary		Variáveis que agentes e fluxos exigem
// @Description	Para cada agente e fluxo pedido, lista as variáveis que a automação precisa em cada conversa, já seguindo o agente que um nó de IA usa, os subfluxos e os fluxos que o agente chama. lead_name, lead_number e campaign_name nunca aparecem: a plataforma sempre preenche. receptiveEligible diz se a automação pode atender conversas receptivas (que o cliente começa, sem dados de campanha): só quando não exige nenhuma variável e pôde ser lida. Os seletores de automação dos canais usam este retorno para marcar as opções que não podem ser ligadas; a regra é só do backend.
// @Tags			Automações
// @Produce		json
// @Param			agents		query		string	false	"IDs de agentes separados por vírgula"
// @Param			workflows	query		string	false	"IDs de fluxos separados por vírgula"
// @Success		200			{object}	EligibilityListResponse
// @Failure		400			{object}	response.ErrorResponse	"automation_needs_too_many: no máximo 100 automações por pedido"
// @Security		BearerAuth
// @Router			/automation-needs [get]
func (h *Handler) Eligibility(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	var asked []conversation.Automation
	for _, id := range idsOf(r.URL.Query().Get("agents")) {
		asked = append(asked, conversation.Automation{Kind: conversation.AutomationAgent, ID: id})
	}
	for _, id := range idsOf(r.URL.Query().Get("workflows")) {
		asked = append(asked, conversation.Automation{Kind: conversation.AutomationWorkflow, ID: id})
	}
	items, err := h.queries.Eligibility(workspaceID, asked)
	if errors.Is(err, automationneeds_usecase.ErrTooMany) {
		response.WriteErrorWithCode(w, http.StatusBadRequest, "automation_needs_too_many", "Peça no máximo 100 automações de uma vez.", nil)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	out := EligibilityListResponse{Items: make([]EligibilityResponse, 0, len(items))}
	for _, item := range items {
		out.Items = append(out.Items, EligibilityResponse{
			Automation: automationneedsx.Link(item.Automation), Variables: automationneedsx.Needs(item.Needs),
			ReceptiveEligible: item.Receptive(), Readable: item.Readable,
		})
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

// @Summary		Canais que publicar a automação desligaria
// @Description	Lê o rascunho salvo do agente ou do fluxo como se fosse publicado agora e lista os canais receptivos (números do WhatsApp, contas e páginas, chat do site, caixas de email, RCS e SMS) que passariam a rodar uma automação com variáveis que conversas receptivas não trazem. Inclui os canais que rodam outra automação que depende desta (um fluxo cujo nó de IA usa o agente, um fluxo que chama este como subfluxo, um agente que chama este fluxo). Publicar sem confirmar quando a lista não está vazia é recusado com 409 automation_publish_switches_off; com POST /agents/{id}/publish?confirmSwitchOff=true ou POST /workflows/{id}/publish?confirmSwitchOff=true esses canais são desligados na mesma gravação da nova versão.
// @Tags			Automações
// @Produce		json
// @Param			kind	path		string	true	"agent ou workflow"	Enums(agent, workflow)
// @Param			id		path		string	true	"ID do agente ou do fluxo"
// @Success		200		{object}	ImpactResponse
// @Failure		422		{object}	response.ErrorResponse	"automation_needs_not_found: a automação não existe neste workspace"
// @Failure		503		{object}	response.ErrorResponse	"automation_needs_unreadable"
// @Security		BearerAuth
// @Router			/automation-needs/{kind}/{id}/publish-impact [get]
func (h *Handler) PublishImpact(w http.ResponseWriter, r *http.Request) {
	h.publishImpact(w, r, conversation.AutomationKind(mux.Vars(r)["kind"]))
}

func (h *Handler) publishImpact(w http.ResponseWriter, r *http.Request, kind conversation.AutomationKind) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	offs, err := h.queries.PublishImpact(r.Context(), workspaceID, conversation.Automation{Kind: kind, ID: mux.Vars(r)["id"]})
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, ImpactResponse{Impact: automationneedsx.Impact(offs)})
}

// @Summary		Aviso de automação desligada no canal
// @Description	Quando uma automação foi desligada num canal receptivo porque passou a exigir variáveis, devolve o aviso aberto: qual automação, quais variáveis, quando, e eligibleNow, que diz se a versão publicada hoje já pode ser ligada de novo. Ligar de novo é o mesmo salvamento da automação do canal; um salvamento que liga uma automação aceita ou escolhe outra fecha o aviso. Nunca religa sozinho. notice vem nulo quando não há aviso.
// @Tags			Automações
// @Produce		json
// @Param			channel	query		string	true	"Tipo do canal"	Enums(whatsapp_number, unofficial_whatsapp, instagram, facebook, telegram, webchat, tiktok, mercadolivre, email, rcs, sms)
// @Param			id		query		string	true	"ID do canal (para o WhatsApp oficial, o ID do número)"
// @Success		200		{object}	SwitchOffResponse
// @Failure		422		{object}	response.ErrorResponse	"automation_needs_not_found: tipo de canal desconhecido"
// @Security		BearerAuth
// @Router			/automation-switch-offs [get]
func (h *Handler) SwitchOff(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	open, err := h.queries.OpenSwitchOff(r.Context(), workspaceID, domain.SlotKind(q.Get("channel")), strings.TrimSpace(q.Get("id")))
	if err != nil {
		writeError(w, err)
		return
	}
	if open == nil {
		response.WriteSuccess(w, http.StatusOK, SwitchOffResponse{})
		return
	}
	rec := open.Record
	response.WriteSuccess(w, http.StatusOK, SwitchOffResponse{Notice: &SwitchOffNoticeResponse{
		Channel:       automationneedsx.Slot(domain.Slot{Kind: rec.SlotKind, ID: rec.SlotID, Name: rec.SlotName}),
		Automation:    automationneedsx.Link(rec.Automation),
		Variables:     rec.Variables,
		StillNeeded:   automationneedsx.Needs(open.Needs),
		SwitchedOffAt: rec.SwitchedOffAt,
		EligibleNow:   open.EligibleNow,
	}})
}

// @Summary		Variáveis da conversa para uma automação
// @Description	Antes de passar a conversa para um agente ou fluxo (ou religar a automação dela), devolve uma linha por variável que a automação exige nesta conversa, já seguindo o agente que um nó de IA usa, os subfluxos e os fluxos que o agente chama. Cada linha traz quem usa a variável (via), o valor que a conversa já tem (current: da campanha ou de uma passagem anterior; vazio quando não tem) e o campo do lead sugerido (suggested: o campo personalizado com a mesma chave). fields lista os campos do lead que podem preencher uma variável, na mesma ordem e com as mesmas regras da etapa de variáveis da campanha, com o valor do lead desta conversa (value vazio quando o lead não tem). Sem kind e id, a automação é a que a conversa volta a ter ao religar a automação; automation vem nula quando não há nenhuma. lead_name, lead_number e campaign_name nunca aparecem: a plataforma sempre preenche. Exige acesso à conversa.
// @Tags			Conversas
// @Produce		json
// @Param			entryType	path		string	true	"Tipo da entrada"
// @Param			entryId		path		string	true	"ID da entrada"
// @Param			kind		query		string	false	"agent ou workflow (junto com id)"	Enums(agent, workflow)
// @Param			id			query		string	false	"ID do agente ou do fluxo (junto com kind)"
// @Success		200			{object}	ConversationVariablesResponse
// @Failure		400			{object}	response.ErrorResponse	"conversation_variable_invalid: tipo de conversa desconhecido, ou kind sem id"
// @Failure		403			{object}	response.ErrorResponse	"forbidden"
// @Failure		422			{object}	response.ErrorResponse	"automation_needs_not_found"
// @Failure		503			{object}	response.ErrorResponse	"automation_needs_unreadable"
// @Security		BearerAuth
// @Router			/conversations/{entryType}/{entryId}/variables [get]
func (h *Handler) ConversationVariables(w http.ResponseWriter, r *http.Request) {
	ref, ok := conversationRefOf(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	kind, id := strings.TrimSpace(q.Get("kind")), strings.TrimSpace(q.Get("id"))
	var target *conversation.Automation
	if kind != "" || id != "" {
		if kind == "" || id == "" {
			response.WriteErrorWithCode(w, http.StatusBadRequest, "conversation_variable_invalid", "Informe kind e id juntos.", nil)
			return
		}
		target = &conversation.Automation{Kind: conversation.AutomationKind(kind), ID: id}
	}
	form, err := h.queries.VariablesForm(r.Context(), ref, target)
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, formResponse(form))
}

// @Summary		Preencher variáveis da conversa
// @Description	Guarda de onde vem cada variável que o agente ou o fluxo exige nesta conversa, com as mesmas origens da etapa de variáveis da campanha. Cada item de bindings tem name e source: um campo do lead (lead.first_name, lead.name, lead.nickname, lead.district, lead.city, lead.owner_name, lead.custom:<chave>), literal com value (texto fixo) ou conversation (manter o valor que a conversa já tem, da campanha ou de uma passagem anterior). O backend lê o campo do lead desta conversa, grava o valor resolvido, e os agentes e fluxos leem pelo caminho de sempre; conversation não grava nada. values ({nome: texto}) continua aceito e vale como texto fixo. Só aceita variáveis que a automação exige; lead_name, lead_number e campaign_name são da plataforma. Uma origem que sai vazia nesta conversa é recusada com 422 conversation_variable_empty e details.variables com os nomes, e nada é gravado. Os valores valem mais que os da campanha. Exige acesso à conversa.
// @Tags			Conversas
// @Accept			json
// @Produce		json
// @Param			entryType	path	string							true	"Tipo da entrada"
// @Param			entryId		path	string							true	"ID da entrada"
// @Param			request		body	ConversationVariablesRequest	true	"Automação e origem de cada variável"
// @Success		204
// @Failure		400	{object}	response.ErrorResponse{details=VariablesErrorDetails}	"conversation_variable_invalid, conversation_variable_source_invalid ou conversation_variable_not_needed"
// @Failure		403	{object}	response.ErrorResponse									"forbidden"
// @Failure		422	{object}	response.ErrorResponse{details=VariablesErrorDetails}	"conversation_variable_empty ou automation_needs_not_found"
// @Failure		503	{object}	response.ErrorResponse									"automation_needs_unreadable"
// @Security		BearerAuth
// @Router			/conversations/{entryType}/{entryId}/variables [put]
func (h *Handler) SaveConversationVariables(w http.ResponseWriter, r *http.Request) {
	ref, ok := conversationRefOf(w, r)
	if !ok {
		return
	}
	var req ConversationVariablesRequest
	if !httpx.DecodeStrictJSON(w, r, &req) {
		return
	}
	bindings := make([]conversationvars.Binding, 0, len(req.Bindings))
	for _, b := range req.Bindings {
		bindings = append(bindings, conversationvars.Binding{Name: b.Name, Source: campaign.BindingSource(b.Source), Value: b.Value})
	}
	err := h.queries.SaveConversationValues(r.Context(), automationneeds_usecase.SaveValues{
		ConversationRef: ref,
		Automation:      conversation.Automation{Kind: conversation.AutomationKind(req.Automation.Kind), ID: strings.TrimSpace(req.Automation.ID)},
		Values:          req.Values,
		Bindings:        bindings,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func conversationRefOf(w http.ResponseWriter, r *http.Request) (automationneeds_usecase.ConversationRef, bool) {
	claims := middleware.GetClaims(r)
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return automationneeds_usecase.ConversationRef{}, false
	}
	if claims == nil {
		response.WriteError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return automationneeds_usecase.ConversationRef{}, false
	}
	vars := mux.Vars(r)
	entryType := shared.EntryType(vars["entryType"])
	if !entryType.IsKnown() {
		response.WriteErrorWithCode(w, http.StatusBadRequest, "conversation_variable_invalid", "Tipo de conversa desconhecido.", nil)
		return automationneeds_usecase.ConversationRef{}, false
	}
	return automationneeds_usecase.ConversationRef{
		WorkspaceID: workspaceID, UserID: claims.UserID, IsAdmin: claims.Role == string(user.RoleAdmin),
		EntryID: strings.TrimSpace(vars["entryId"]), EntryType: entryType,
	}, true
}

func formResponse(form automationneeds_usecase.VariablesForm) ConversationVariablesResponse {
	out := ConversationVariablesResponse{Variables: make([]VariableRowResponse, 0, len(form.Rows)), Fields: make([]ConversationFieldResponse, 0, len(form.Fields))}
	if form.Automation != nil {
		link := automationneedsx.Link(*form.Automation)
		out.Automation = &link
	}
	for _, row := range form.Rows {
		need := automationneedsx.Needs(domain.Needs{row.Need})[0]
		out.Variables = append(out.Variables, VariableRowResponse{Name: need.Name, Description: need.Description, Via: need.Via, Current: row.Current, Suggested: string(row.Suggested)})
	}
	for _, f := range form.Fields {
		out.Fields = append(out.Fields, ConversationFieldResponse{Field: f.Field, Value: f.Value})
	}
	return out
}

var valueRefusals = []struct {
	err    error
	status int
	code   string
	text   string
}{
	{conversationvars.ErrNothingToSave, http.StatusBadRequest, "conversation_variable_invalid", "Preencha pelo menos um valor."},
	{conversationvars.ErrNameInvalid, http.StatusBadRequest, "conversation_variable_invalid", "O nome da variável tem só letras, números e _."},
	{conversationvars.ErrNameReserved, http.StatusBadRequest, "conversation_variable_invalid", "Essa variável é preenchida pela plataforma."},
	{conversationvars.ErrValueInvalid, http.StatusBadRequest, "conversation_variable_invalid", "Cada valor tem até 2000 caracteres."},
	{conversationvars.ErrSourceInvalid, http.StatusBadRequest, "conversation_variable_source_invalid", "Essa origem não pode preencher a variável."},
	{conversationvars.ErrNotNeeded, http.StatusBadRequest, "conversation_variable_not_needed", "Essa automação não usa essa variável."},
	{conversationvars.ErrValueEmpty, http.StatusUnprocessableEntity, "conversation_variable_empty", "Essa origem não tem valor nesta conversa."},
}

func ValueCodes() []string {
	out := []string{"automation_needs_too_many"}
	for _, r := range valueRefusals {
		out = append(out, r.code)
	}
	return out
}

func writeError(w http.ResponseWriter, err error) {
	if automationneedsx.WriteRefusal(w, err) {
		return
	}
	if errors.Is(err, automationneeds_usecase.ErrForbidden) {
		response.WriteErrorWithCode(w, http.StatusForbidden, "forbidden", "Sem acesso a esta conversa.", nil)
		return
	}
	for _, refusal := range valueRefusals {
		if !errors.Is(err, refusal.err) {
			continue
		}
		var named *conversationvars.VariablesError
		if errors.As(err, &named) {
			response.WriteErrorWithDetails(w, refusal.status, refusal.code, refusal.text, VariablesErrorDetails{Variables: named.Names})
			return
		}
		response.WriteErrorWithCode(w, refusal.status, refusal.code, refusal.text, nil)
		return
	}
	response.WriteErrorWithCode(w, http.StatusInternalServerError, "internal_error", "Não foi possível concluir agora.", nil)
}

func RegisterRoutes(
	protected *mux.Router,
	h *Handler,
	ac func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc,
) {
	protected.HandleFunc("/automation-needs", h.Eligibility).Methods(http.MethodGet)
	protected.HandleFunc("/automation-needs/agent/{id}/publish-impact", ac(workspace_domain.ResourceAgents, workspace_domain.ActionUpdate, func(w http.ResponseWriter, r *http.Request) {
		h.publishImpact(w, r, conversation.AutomationAgent)
	})).Methods(http.MethodGet)
	protected.HandleFunc("/automation-needs/workflow/{id}/publish-impact", ac(workspace_domain.ResourceWorkflows, workspace_domain.ActionUpdate, func(w http.ResponseWriter, r *http.Request) {
		h.publishImpact(w, r, conversation.AutomationWorkflow)
	})).Methods(http.MethodGet)
	protected.HandleFunc("/automation-switch-offs", h.SwitchOff).Methods(http.MethodGet)
	protected.HandleFunc("/conversations/{entryType}/{entryId}/variables", h.ConversationVariables).Methods(http.MethodGet)
	protected.HandleFunc("/conversations/{entryType}/{entryId}/variables", h.SaveConversationVariables).Methods(http.MethodPut)
}
