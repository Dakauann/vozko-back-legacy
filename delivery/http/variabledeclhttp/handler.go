package variabledeclhttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	domain "vozko/domain/variabledecl"
	"vozko/domain/workflow"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
)

type Declarations interface {
	List(ctx context.Context, workspaceID string) ([]domain.Declaration, error)
	Save(ctx context.Context, workspaceID, by string, d domain.Declaration) (domain.Declaration, error)
	Delete(ctx context.Context, workspaceID, name string) error
	Registry() *domain.Registry
}

type Handler struct {
	declarations Declarations
}

func NewHandler(declarations Declarations) *Handler {
	if declarations == nil {
		panic("variabledeclhttp: the handler needs the declarations")
	}
	return &Handler{declarations: declarations}
}

type TypeRequest struct {
	Kind     string            `json:"kind" example:"choice"`
	Options  []string          `json:"options,omitempty" example:"Manhã,Tarde"`
	Settings map[string]string `json:"settings,omitempty"`
}

type TypeResponse struct {
	Kind     string            `json:"kind" example:"choice"`
	Shape    string            `json:"shape" enums:"text,number,date,choice,sheet" example:"choice"`
	Options  []string          `json:"options" example:"Manhã,Tarde"`
	Settings map[string]string `json:"settings,omitempty"`
}

type DeclarationRequest struct {
	Label       string      `json:"label" example:"Turno"`
	Description string      `json:"description,omitempty" example:"Período da aula"`
	Example     string      `json:"example,omitempty" example:"Manhã"`
	Type        TypeRequest `json:"type"`
}

type DeclarationResponse struct {
	Name        string       `json:"name" example:"turno"`
	Ref         string       `json:"ref" example:"campvars.turno"`
	Label       string       `json:"label" example:"Turno"`
	Description string       `json:"description" example:"Período da aula"`
	Example     string       `json:"example" example:"Manhã"`
	Sample      string       `json:"sample" example:"Manhã"`
	Type        TypeResponse `json:"type"`
	UpdatedAt   time.Time    `json:"updatedAt"`
}

type SystemVariableResponse struct {
	Name     string `json:"name" example:"lead_name"`
	Label    string `json:"label" example:"Nome do contato"`
	LabelKey string `json:"labelKey" example:"conversation_lead_name"`
}

type KindResponse struct {
	Kind  string `json:"kind" example:"number"`
	Shape string `json:"shape" enums:"text,number,date,choice,sheet" example:"number"`
}

type ListResponse struct {
	Declarations []DeclarationResponse    `json:"declarations"`
	System       []SystemVariableResponse `json:"system"`
	Types        []KindResponse           `json:"types"`
}

func TypeOf(registry *domain.Registry, t domain.Type) TypeResponse {
	options := t.Options
	if options == nil {
		options = []string{}
	}
	return TypeResponse{Kind: string(t.Kind), Shape: string(registry.ShapeOf(t)), Options: options, Settings: t.Settings}
}

func declarationResponse(registry *domain.Registry, d domain.Declaration) DeclarationResponse {
	return DeclarationResponse{Name: d.Name, Ref: workflow.ConversationRef(d.Name), Label: d.Label, Description: d.Description, Example: d.Example, Sample: registry.ExampleOf(d), Type: TypeOf(registry, d.Type), UpdatedAt: d.UpdatedAt}
}

// @Summary		Dados da conversa declarados
// @Description	Lista os dados da conversa que o workspace declarou (nome técnico, nome amigável, descrição e tipo), os dados que a plataforma sempre preenche (system, com labelKey para traduzir o nome) e os tipos disponíveis (types, com o formato de entrada shape). Agentes e fluxos leem esses dados como {{campvars.<nome>}}; o nome amigável é o que as telas mostram.
// @Tags			Dados da conversa
// @Produce		json
// @Success		200	{object}	ListResponse
// @Failure		503	{object}	response.ErrorResponse	"variable_declarations_unreadable"
// @Security		BearerAuth
// @Router			/conversation-variables [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	rows, err := h.declarations.List(r.Context(), workspaceID)
	if err != nil {
		writeError(w, err)
		return
	}
	registry := h.declarations.Registry()
	out := ListResponse{Declarations: make([]DeclarationResponse, 0, len(rows)), System: []SystemVariableResponse{}, Types: []KindResponse{}}
	for _, d := range rows {
		out.Declarations = append(out.Declarations, declarationResponse(registry, d))
	}
	for _, name := range domain.SystemKeys() {
		label, _ := domain.SystemLabel(name)
		out.System = append(out.System, SystemVariableResponse{Name: name, Label: label, LabelKey: domain.SystemLabelKey(name)})
	}
	for _, kind := range registry.Kinds() {
		rule, _ := registry.Rule(kind)
		out.Types = append(out.Types, KindResponse{Kind: string(kind), Shape: string(rule.Shape())})
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

// @Summary		Declarar ou editar um dado da conversa
// @Description	Cria ou edita a declaração de um dado da conversa: nome amigável (1 a 60 caracteres), descrição (até 500), exemplo opcional (validado pelo tipo; sem ele, sample traz a amostra do tipo) e tipo. Tipos: text (1 a 2000 caracteres), number (1234, 1234,5, 1.234,56 ou 1,234.56), date (DD/MM/AAAA ou AAAA-MM-DD), choice (exatamente uma das options, 1 a 50 opções de até 100 caracteres), sheet (Planilha: o valor é o id de um arquivo da lista de Planilhas do workspace, criado pela conta Google conectada e com todas as abas e colunas do modelo; settings.model é o arquivo modelo, obrigatório, e settings.default, opcional, é a planilha usada por conversas receptivas; settings.model_title vem do backend; sem exemplo, a amostra é o nome do modelo). O nome técnico vem do caminho: letras sem acento, números e _, até 64; lead_name, lead_number e campaign_name são da plataforma. Todo valor que entra (campanha, passagem da conversa, transferência, dados do teste, painel da conversa, nó Definir variável) é validado pelo tipo. Mudar o tipo não reescreve valores já guardados.
// @Tags			Dados da conversa
// @Accept			json
// @Produce		json
// @Param			name	path		string				true	"Nome técnico"
// @Param			request	body		DeclarationRequest	true	"Declaração"
// @Success		200		{object}	DeclarationResponse
// @Failure		400		{object}	response.ErrorResponse	"variable_name_invalid, variable_name_reserved, variable_label_invalid, variable_description_too_long, variable_type_unknown, variable_type_invalid ou variable_example_invalid"
// @Failure		503		{object}	response.ErrorResponse	"variable_declarations_unreadable"
// @Security		BearerAuth
// @Router			/conversation-variables/{name} [put]
func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	var req DeclarationRequest
	if !httpx.DecodeStrictJSON(w, r, &req) {
		return
	}
	by := ""
	if claims := middleware.GetClaims(r); claims != nil {
		by = claims.UserID
	}
	saved, err := h.declarations.Save(r.Context(), workspaceID, by, domain.Declaration{
		Name: mux.Vars(r)["name"], Label: req.Label, Description: req.Description, Example: req.Example,
		Type: domain.Type{Kind: domain.Kind(strings.TrimSpace(req.Type.Kind)), Options: req.Type.Options, Settings: req.Type.Settings},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, declarationResponse(h.declarations.Registry(), saved))
}

// @Summary		Remover a declaração de um dado da conversa
// @Description	Remove só a declaração (nome amigável e tipo). Textos que já usam {{campvars.<nome>}} continuam funcionando e o dado aparece como não declarado; valores já guardados nas conversas ficam.
// @Tags			Dados da conversa
// @Param			name	path	string	true	"Nome técnico"
// @Success		204
// @Failure		404	{object}	response.ErrorResponse	"variable_not_found"
// @Failure		503	{object}	response.ErrorResponse	"variable_declarations_unreadable"
// @Security		BearerAuth
// @Router			/conversation-variables/{name} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	if err := h.declarations.Delete(r.Context(), workspaceID, mux.Vars(r)["name"]); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var refusals = []struct {
	err    error
	status int
	code   string
	text   string
}{
	{domain.ErrNameInvalid, http.StatusBadRequest, "variable_name_invalid", "O nome técnico tem só letras sem acento, números e _, até 64."},
	{domain.ErrNameReserved, http.StatusBadRequest, "variable_name_reserved", "Esse dado é preenchido pela plataforma."},
	{domain.ErrLabelInvalid, http.StatusBadRequest, "variable_label_invalid", "O nome amigável tem de 1 a 60 caracteres."},
	{domain.ErrDescriptionTooLong, http.StatusBadRequest, "variable_description_too_long", "A descrição tem até 500 caracteres."},
	{domain.ErrKindUnknown, http.StatusBadRequest, "variable_type_unknown", "Esse tipo não existe."},
	{domain.ErrTypeInvalid, http.StatusBadRequest, "variable_type_invalid", "As opções do tipo são inválidas."},
	{domain.ErrExampleInvalid, http.StatusBadRequest, "variable_example_invalid", "O exemplo não está no formato do tipo escolhido."},
	{domain.ErrNotFound, http.StatusNotFound, "variable_not_found", "Esse dado não está declarado."},
	{domain.ErrUnreadable, http.StatusServiceUnavailable, "variable_declarations_unreadable", "Não foi possível ler os dados da conversa agora."},
}

func Codes() []string {
	out := make([]string, 0, len(refusals))
	for _, r := range refusals {
		out = append(out, r.code)
	}
	return out
}

func writeError(w http.ResponseWriter, err error) {
	for _, refusal := range refusals {
		if errors.Is(err, refusal.err) {
			response.WriteErrorWithCode(w, refusal.status, refusal.code, refusal.text, nil)
			return
		}
	}
	response.WriteErrorWithCode(w, http.StatusInternalServerError, "internal_error", "Não foi possível concluir agora.", nil)
}

func RegisterRoutes(
	protected *mux.Router,
	h *Handler,
	ac func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc,
) {
	protected.HandleFunc("/conversation-variables", ac(workspace_domain.ResourceWorkflows, workspace_domain.ActionRead, h.List)).Methods(http.MethodGet)
	protected.HandleFunc("/conversation-variables/{name}", ac(workspace_domain.ResourceWorkflows, workspace_domain.ActionUpdate, h.Save)).Methods(http.MethodPut)
	protected.HandleFunc("/conversation-variables/{name}", ac(workspace_domain.ResourceWorkflows, workspace_domain.ActionUpdate, h.Delete)).Methods(http.MethodDelete)
}
