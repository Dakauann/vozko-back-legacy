package sitepublishhttp

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/sitepublish"
	"vozko/usecases/siteanalytics"
	"vozko/usecases/sitepublish"
)

const (
	defaultDeploymentLimit = 50
	maxDeploymentLimit     = 200
)

type Publisher interface {
	Publish(ctx context.Context, a sitepublish_usecase.Actor, projectID string) (sitepublish_usecase.PublishResult, error)
	Rollback(ctx context.Context, a sitepublish_usecase.Actor, projectID, deploymentID string) (sitepublish.DeploymentRecord, error)
	Unpublish(ctx context.Context, a sitepublish_usecase.Actor, projectID string) error
	ListDeployments(ctx context.Context, a sitepublish_usecase.Actor, projectID string, limit int) ([]sitepublish.DeploymentRecord, error)
	ListHostnames(ctx context.Context, a sitepublish_usecase.Actor, projectID string) ([]sitepublish.HostnameRecord, error)
	AddHostname(ctx context.Context, a sitepublish_usecase.Actor, projectID string, in sitepublish_usecase.AddHostnameInput) (sitepublish.HostnameRecord, error)
	RemoveHostname(ctx context.Context, a sitepublish_usecase.Actor, projectID, hostnameID string) (sitepublish.HostnameRecord, error)
	VerifyHostname(ctx context.Context, a sitepublish_usecase.Actor, projectID, hostnameID string) (sitepublish.HostnameRecord, error)
	DomainSlots(ctx context.Context, a sitepublish_usecase.Actor) (sitepublish_usecase.DomainSlots, error)
	SettingsChanges(ctx context.Context, a sitepublish_usecase.Actor, projectID string) (sitepublish.SettingsChanges, error)
}

type Analytics interface {
	Read(ctx context.Context, a siteanalytics_usecase.Actor, projectID, from, to string) (siteanalytics_usecase.Report, error)
}

type Handler struct {
	publisher Publisher
	analytics Analytics
	addresses sitepublish.Addresses
}

func NewHandler(publisher Publisher, analytics Analytics, addresses sitepublish.Addresses) *Handler {
	return &Handler{publisher: publisher, analytics: analytics, addresses: addresses}
}

func (h *Handler) actor(w http.ResponseWriter, r *http.Request) (sitepublish_usecase.Actor, bool) {
	viewer, ok := httpx.WorkspaceActor(r)
	if !ok {
		response.WriteRefusal(w, refusalAccessDenied)
		return sitepublish_usecase.Actor{}, false
	}
	return sitepublish_usecase.Actor{UserID: viewer.UserID, WorkspaceID: viewer.WorkspaceID}, true
}

func projectID(r *http.Request) string {
	return mux.Vars(r)["id"]
}

// @Summary		Publicações da página
// @Description	Lista as publicações (deployments) da página, da mais nova para a mais antiga, com status, verificações informativas e pendências que bloquearam a publicação. address é o endereço imutável da publicação (no ar ou anterior), servido só para leitura. Exige `sites:read`.
// @Tags			sites
// @Produce		json
// @Param			id		path		string	true	"ID da página"
// @Param			limit		query		int		false	"Quantidade (padrão 50, máximo 200)"
// @Success		200			{array}		DeploymentDTO
// @Failure		403			{object}	response.ErrorResponse	"site_access_denied"
// @Failure		503			{object}	response.ErrorResponse	"publish_unavailable"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/deployments [get]
func (h *Handler) ListDeployments(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	limit := defaultDeploymentLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			response.WriteRefusal(w, refusalInvalidBody)
			return
		}
		limit = min(n, maxDeploymentLimit)
	}
	list, err := h.publisher.ListDeployments(r.Context(), a, projectID(r), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]DeploymentDTO, 0, len(list))
	for _, rec := range list {
		out = append(out, deploymentDTO(rec, h.addresses))
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

// @Summary		Publicar a página
// @Description	Compila a versão atual em modo publicação, pré-renderiza as rotas React, valida o HTML final (regras e pendências), verifica links e o subdomínio no Safe Browsing, copia as imagens da biblioteca para a publicação, envia os arquivos e troca o endereço para a nova publicação. Fica no ar em até 1 minuto em todos os domínios ativos; o endereço imutável vale na hora. Páginas que imitam bancos, Pix, gov.br, Receita ou Correios com campos de formulário ficam em análise (held true) em vez de entrar no ar. Exige `sites:publish` e assinatura ativa.
// @Tags			sites
// @Produce		json
// @Param			id		path		string	true	"ID da página"
// @Success		201			{object}	PublishResponse
// @Failure		402			{object}	response.ErrorResponse	"subscription_inactive"
// @Failure		403			{object}	response.ErrorResponse	"site_access_denied"
// @Failure		404			{object}	response.ErrorResponse	"site_not_found"
// @Failure		409			{object}	response.ErrorResponse	"publish_under_review ou site_suspended"
// @Failure		422			{object}	response.ErrorResponse	"build_failed, publish_blocked, gaps_open (details lista as pendências), form_key_missing, form_channel_required, form_success_channel_required, whatsapp_number_unavailable ou webchat_widget_unknown"
// @Failure		502			{object}	response.ErrorResponse	"prerender_failed"
// @Failure		503			{object}	response.ErrorResponse	"media_unavailable, forms_unavailable, hosting_unavailable ou publish_unavailable"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/deployments [post]
func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	res, err := h.publisher.Publish(r.Context(), a, projectID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	hosts := res.Hostnames
	if hosts == nil {
		hosts = []string{}
	}
	response.WriteSuccess(w, http.StatusCreated, PublishResponse{Deployment: deploymentDTO(res.Deployment, h.addresses), Held: res.Held, Hostnames: hosts, ProjectVersion: res.ProjectVersion, RevisionID: res.RevisionID, HasUnpublishedChanges: res.HasUnpublishedChanges})
}

// @Summary		Tirar a página do ar
// @Description	Apaga o endereço de todos os domínios ativos da página, desativa os formulários dela e volta a página para rascunho. Os domínios continuam reservados para a próxima publicação e as publicações anteriores ficam no histórico. Repetir com a página fora do ar não faz nada. Exige `sites:publish`.
// @Tags			sites
// @Param			id	path	string	true	"ID da página"
// @Success		204	"sem conteúdo"
// @Failure		403	{object}	response.ErrorResponse	"site_access_denied"
// @Failure		404	{object}	response.ErrorResponse	"site_not_found"
// @Failure		503	{object}	response.ErrorResponse	"forms_unavailable, hosting_unavailable ou publish_unavailable"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/unpublish [post]
func (h *Handler) Unpublish(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	if err := h.publisher.Unpublish(r.Context(), a, projectID(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary		Voltar uma publicação ao ar
// @Description	Coloca no ar uma publicação anterior (rollback): reativa os formulários dela e troca o endereço de todos os domínios ativos. Exige `sites:publish` e assinatura ativa.
// @Tags			sites
// @Produce		json
// @Param			id			path		string	true	"ID da página"
// @Param			deploymentId	path		string	true	"ID da publicação"
// @Success		200				{object}	DeploymentDTO
// @Failure		402				{object}	response.ErrorResponse	"subscription_inactive"
// @Failure		403				{object}	response.ErrorResponse	"site_access_denied"
// @Failure		404				{object}	response.ErrorResponse	"deployment_not_found"
// @Failure		409				{object}	response.ErrorResponse	"deployment_not_activatable, publish_under_review ou site_suspended"
// @Failure		503				{object}	response.ErrorResponse	"forms_unavailable ou hosting_unavailable"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/deployments/{deploymentId}/activate [post]
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	rec, err := h.publisher.Rollback(r.Context(), a, projectID(r), mux.Vars(r)["deploymentId"])
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, deploymentDTO(rec, h.addresses))
}

// @Summary		Domínios da página
// @Description	Lista o subdomínio da plataforma e os domínios próprios da página com o estado (pending_dns, pending_ssl, active, removing, failed) e os registros DNS a criar. Exige `site_domains:read`.
// @Tags			sites
// @Produce		json
// @Param			id		path		string	true	"ID da página"
// @Success		200			{array}		HostnameDTO
// @Failure		403			{object}	response.ErrorResponse	"site_access_denied"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/hostnames [get]
func (h *Handler) ListHostnames(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	list, err := h.publisher.ListHostnames(r.Context(), a, projectID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]HostnameDTO, 0, len(list))
	for _, rec := range list {
		out = append(out, hostnameDTO(rec))
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

// @Summary		Adicionar domínio
// @Description	kind platform cria o subdomínio da plataforma (slug opcional; sem slug, usa o nome da página e do workspace). kind custom registra um domínio próprio: responde pending_dns com o CNAME e os registros TXT de validação; o domínio só entra no ar depois que o certificado é emitido. Domínios próprios contam contra a inclusão do plano mais os adicionais contratados. Exige `site_domains:update` e assinatura ativa.
// @Tags			sites
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"ID da página"
// @Param			body		body		AddHostnameRequest	true	"Domínio"
// @Success		201			{object}	HostnameDTO
// @Failure		400			{object}	response.ErrorResponse	"invalid_body ou hostname_kind_invalid"
// @Failure		402			{object}	response.ErrorResponse	"subscription_inactive"
// @Failure		403			{object}	response.ErrorResponse	"site_access_denied"
// @Failure		409			{object}	response.ErrorResponse	"hostname_taken, platform_hostname_exists ou site_domains_exhausted"
// @Failure		422			{object}	response.ErrorResponse	"hostname_invalid, hostname_forbidden, slug_invalid ou slug_reserved"
// @Failure		503			{object}	response.ErrorResponse	"domain_provider_unavailable ou hosting_unavailable"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/hostnames [post]
func (h *Handler) AddHostname(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	var body AddHostnameRequest
	if !httpx.DecodeStrictJSON(w, r, &body) {
		return
	}
	rec, err := h.publisher.AddHostname(r.Context(), a, projectID(r), sitepublish_usecase.AddHostnameInput{
		Kind: sitepublish.HostnameKind(body.Kind), Slug: body.Slug, Hostname: body.Hostname,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusCreated, hostnameDTO(rec))
}

// @Summary		Remover domínio
// @Description	Tira o domínio do ar e o remove do provedor. Se o provedor falhar, o domínio fica em removing e a varredura conclui a remoção. Exige `site_domains:update`.
// @Tags			sites
// @Produce		json
// @Param			id		path		string	true	"ID da página"
// @Param			hostnameId	path		string	true	"ID do domínio"
// @Success		200			{object}	HostnameDTO
// @Failure		403			{object}	response.ErrorResponse	"site_access_denied"
// @Failure		404			{object}	response.ErrorResponse	"hostname_not_found"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/hostnames/{hostnameId} [delete]
func (h *Handler) RemoveHostname(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	rec, err := h.publisher.RemoveHostname(r.Context(), a, projectID(r), mux.Vars(r)["hostnameId"])
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, hostnameDTO(rec))
}

// @Summary		Números da página
// @Description	Visualizações, visitantes únicos (por dia, sem cookies), cliques em CTA, formulários vistos, iniciados e enviados e cliques no WhatsApp, por dia, rota e variante, com totais e taxa de conversão (envios mais WhatsApp sobre visitantes, em pontos-base). Os números são contados pelo servidor das páginas e consolidados de hora em hora; robôs não contam. sources traz as visitas e os envios por utm_source e pela origem de quem indicou (sem a própria página), até 25 de cada. leads conta as pessoas que enviaram um formulário da página ou chegaram por ela a uma conversa no período; wonDeals e wonRevenue somam as oportunidades dessas pessoas ganhas no período, dentro do escopo de oportunidades de quem consulta (dealsVisible falso quando a pessoa não vê oportunidades). wonRevenue está em unidades da moeda wonRevenueCurrency (a de maior valor); wonRevenueByCurrency traz cada moeda em centavos. Sem from e to, os últimos 30 dias. Exige `sites:read`.
// @Tags			sites
// @Produce		json
// @Param			id		path		string	true	"ID da página"
// @Param			from		query		string	false	"Primeiro dia (AAAA-MM-DD, UTC)"
// @Param			to			query		string	false	"Último dia (AAAA-MM-DD, UTC), no máximo 400 dias após from"
// @Success		200			{object}	siteanalytics_usecase.Report
// @Failure		400			{object}	response.ErrorResponse	"analytics_period_invalid"
// @Failure		403			{object}	response.ErrorResponse	"site_access_denied"
// @Failure		404			{object}	response.ErrorResponse	"site_not_found"
// @Failure		503			{object}	response.ErrorResponse	"analytics_unavailable"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/analytics [get]
func (h *Handler) Analytics(w http.ResponseWriter, r *http.Request) {
	viewer, ok := httpx.WorkspaceActor(r)
	if !ok {
		response.WriteRefusal(w, refusalAccessDenied)
		return
	}
	q := r.URL.Query()
	report, err := h.analytics.Read(r.Context(), siteanalytics_usecase.Actor{UserID: viewer.UserID, WorkspaceID: viewer.WorkspaceID, IsAdmin: viewer.IsAdmin}, projectID(r), q.Get("from"), q.Get("to"))
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, report)
}

// @Summary		Verificar domínio próprio
// @Description	Confere agora o registro TXT de propriedade do domínio (verification.ownership). Com o TXT encontrado e um domínio contratado livre, o domínio é registrado no provedor e segue para o certificado (pending_ssl); sem domínio livre, fica em suspended até haver vaga. Um domínio em failed é registrado de novo. Domínios sem verificação expiram em 72 horas (verification.pendingUntil). Exige `site_domains:update`.
// @Tags			sites
// @Produce		json
// @Param			id		path		string	true	"ID da página"
// @Param			hostnameId	path		string	true	"ID do domínio"
// @Success		200			{object}	HostnameDTO
// @Failure		403			{object}	response.ErrorResponse	"site_access_denied"
// @Failure		404			{object}	response.ErrorResponse	"hostname_not_found"
// @Failure		409			{object}	response.ErrorResponse	"domain_check_in_progress"
// @Failure		503			{object}	response.ErrorResponse	"domain_provider_unavailable"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/hostnames/{hostnameId}/verify [post]
func (h *Handler) VerifyHostname(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	rec, err := h.publisher.VerifyHostname(r.Context(), a, projectID(r), mux.Vars(r)["hostnameId"])
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, hostnameDTO(rec))
}

// @Summary		Domínios próprios do workspace
// @Description	Quantos domínios próprios o workspace usa (used: todos os que não estão sendo removidos, em qualquer página) e quantos pode manter no ar agora (allowed: só os domínios contratados como adicional; nenhum plano inclui domínios). offline billing indica assinatura pendente ou pagamento contestado: os domínios ficam fora do ar até o pagamento ser confirmado. Exige `site_domains:read`.
// @Tags			sites
// @Produce		json
// @Success		200			{object}	DomainSlotsDTO
// @Failure		403			{object}	response.ErrorResponse	"site_access_denied"
// @Failure		503			{object}	response.ErrorResponse	"publish_unavailable"
// @Security		BearerAuth
// @Router			/sites/domain-slots [get]
func (h *Handler) DomainSlots(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	slots, err := h.publisher.DomainSlots(r.Context(), a)
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, DomainSlotsDTO{Used: slots.Used, Allowed: slots.Allowed, Offline: string(slots.Offline)})
}
