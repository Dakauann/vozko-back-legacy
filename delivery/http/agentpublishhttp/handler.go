package agentpublishhttp

import (
	"net/http"
	"strconv"
	"time"
	automationneeds_usecase "vozko/usecases/automationneeds"

	"github.com/gorilla/mux"

	"vozko/delivery/http/agenthttp"
	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/tools"
	workspace_domain "vozko/domain/workspace"
	agent_usecase "vozko/usecases/agent"
)

type Handler struct {
	publishing agent_usecase.AgentPublishing
	agents     agent.GetAgentUseCase
	registry   tools.Service
}

func NewHandler(publishing agent_usecase.AgentPublishing, agents agent.GetAgentUseCase, registry tools.Service) *Handler {
	return &Handler{publishing: publishing, agents: agents, registry: registry}
}

type PublicationResponse struct {
	PublishedVersion *int       `json:"publishedVersion"`
	PublishedAt      *time.Time `json:"publishedAt,omitempty"`
	PublishedBy      string     `json:"publishedBy,omitempty"`
	HasDraftChanges  bool       `json:"hasUnpublishedChanges"`
}

type ToolPinResponse struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
}

type VersionResponse struct {
	Version         int               `json:"version" example:"3"`
	PublishedBy     string            `json:"publishedBy"`
	PublishedAt     time.Time         `json:"publishedAt"`
	Name            string            `json:"name"`
	MessagingPrompt string            `json:"messagingPrompt"`
	MessagingModel  string            `json:"messagingModel"`
	Tools           []ToolPinResponse `json:"tools"`
}

type VersionListResponse struct {
	Versions []VersionResponse `json:"versions"`
}

func versionResponse(v agent.Version) VersionResponse {
	pins := make([]ToolPinResponse, 0, len(v.Snapshot.Tools))
	for _, p := range v.Snapshot.Tools {
		pins = append(pins, ToolPinResponse{ID: p.ID, Revision: p.Revision})
	}
	return VersionResponse{
		Version: v.Version, PublishedBy: v.PublishedBy, PublishedAt: v.PublishedAt,
		Name: v.Snapshot.Name, MessagingPrompt: v.Snapshot.MessagingPrompt, MessagingModel: v.Snapshot.MessagingModel, Tools: pins,
	}
}

// @Summary		Publicar o rascunho do agente
// @Description	Publica o rascunho atual do agente como uma nova versão imutável. Editar o agente ou as ferramentas muda só o rascunho; as conversas e os fluxos usam a versão publicada até a próxima publicação. Cada versão fixa a revisão de cada ferramenta.
// @Description
// @Description	Antes de publicar, o rascunho passa pelo mesmo validador do editor. Com algum problema bloqueante a resposta é 422 `agent_not_publishable` com a lista em `details.issues`. Um rascunho igual à versão publicada não cria versão nova.
// @Tags			Agentes
// @Produce		json
// @Param			id	path		string	true	"ID do agente"
// @Success		200	{object}	VersionResponse
// @Failure		403	{object}	response.ErrorResponse	"agent_forbidden"
// @Failure		422	{object}	response.ErrorResponse	"agent_not_publishable"
// @Security		BearerAuth
// @Param			confirmSwitchOff	query	bool	false	"true confirma desligar os canais receptivos listados em GET /automation-needs/agent/{id}/publish-impact"
// @Failure		409	{object}	response.ErrorResponse{details=automationneedsx.RefusalDetails}	"automation_publish_switches_off: a nova versão exige variáveis e desligaria canais receptivos; details.impact lista os canais. Confirme com confirmSwitchOff=true"
// @Router			/agents/{id}/publish [post]
func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if r.URL.Query().Get("confirmSwitchOff") == "true" {
		ctx = automationneeds_usecase.WithSwitchOffConfirmed(ctx)
	}
	version, err := h.publishing.Publish(ctx, agentID, httpx.RequesterOf(r))
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, versionResponse(version))
}

// @Summary		Estado de publicação do agente
// @Description	Diz qual versão está publicada, quando e por quem, e se o rascunho tem mudanças ainda não publicadas. Sem versão publicada, `publishedVersion` vem nulo e o agente não atende.
// @Tags			Agentes
// @Produce		json
// @Param			id	path		string	true	"ID do agente"
// @Success		200	{object}	PublicationResponse
// @Security		BearerAuth
// @Router			/agents/{id}/publication [get]
func (h *Handler) Publication(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	state, err := h.publishing.Publication(r.Context(), agentID)
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, PublicationResponse{PublishedVersion: state.PublishedVersion, PublishedAt: state.PublishedAt, PublishedBy: state.PublishedBy, HasDraftChanges: state.HasDraftChanges})
}

// @Summary		Listar as versões publicadas do agente
// @Description	Lista as versões publicadas, da mais nova para a mais antiga, com as revisões de ferramenta que cada uma fixou.
// @Tags			Agentes
// @Produce		json
// @Param			id	path		string	true	"ID do agente"
// @Success		200	{object}	VersionListResponse
// @Security		BearerAuth
// @Router			/agents/{id}/versions [get]
func (h *Handler) Versions(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	list, err := h.publishing.Versions(r.Context(), agentID)
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	out := VersionListResponse{Versions: make([]VersionResponse, 0, len(list))}
	for _, v := range list {
		out.Versions = append(out.Versions, versionResponse(v))
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

// @Summary		Ler uma versão publicada do agente
// @Description	Devolve o prompt, o modelo e as revisões de ferramenta da versão pedida.
// @Tags			Agentes
// @Produce		json
// @Param			id		path		string	true	"ID do agente"
// @Param			version	path		int		true	"Número da versão"
// @Success		200		{object}	VersionResponse
// @Failure		404		{object}	response.ErrorResponse	"agent_version_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/versions/{version} [get]
func (h *Handler) Version(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(mux.Vars(r)["version"])
	if err != nil {
		agenthttp.WriteError(w, agent.ErrAgentVersionMissing)
		return
	}
	v, err := h.publishing.Version(r.Context(), agentID, n)
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, versionResponse(v))
}

// @Summary		Restaurar uma versão como rascunho
// @Description	Copia a versão pedida para o rascunho: campos do agente e ferramentas com a configuração daquela revisão. Nada muda nas conversas até a próxima publicação.
// @Tags			Agentes
// @Produce		json
// @Param			id		path	string	true	"ID do agente"
// @Param			version	path	int		true	"Número da versão"
// @Success		200
// @Failure		404	{object}	response.ErrorResponse	"agent_version_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/versions/{version}/restore [post]
func (h *Handler) Restore(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(mux.Vars(r)["version"])
	if err != nil {
		agenthttp.WriteError(w, agent.ErrAgentVersionMissing)
		return
	}
	restored, err := h.publishing.Restore(r.Context(), agentID, n, httpx.RequesterOf(r))
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	var defs []tools.Definition
	if h.registry != nil {
		defs = h.registry.Definitions()
	}
	out := *restored
	out.InternalTools = agenttool.ConcealBindings(defs, restored.InternalTools)
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (string, bool) {
	return agenthttp.Authorize(w, r, h.agents)
}

func RegisterRoutes(
	protected *mux.Router,
	h *Handler,
	ac func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc,
) {
	ag := workspace_domain.ResourceAgents
	read, update := workspace_domain.ActionReadDetails, workspace_domain.ActionUpdate
	protected.HandleFunc("/agents/{id}/publish", ac(ag, update, h.Publish)).Methods(http.MethodPost)
	protected.HandleFunc("/agents/{id}/publication", ac(ag, read, h.Publication)).Methods(http.MethodGet)
	protected.HandleFunc("/agents/{id}/versions", ac(ag, read, h.Versions)).Methods(http.MethodGet)
	protected.HandleFunc("/agents/{id}/versions/{version}", ac(ag, read, h.Version)).Methods(http.MethodGet)
	protected.HandleFunc("/agents/{id}/versions/{version}/restore", ac(ag, update, h.Restore)).Methods(http.MethodPost)
}
