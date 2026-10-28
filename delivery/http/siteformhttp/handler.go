package siteformhttp

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/sitesubmission"
	workspace_domain "vozko/domain/workspace"
	"vozko/usecases/siteforms"
)

const (
	defaultPageSize = 30
	submissionsList = "/sites/projects/{id}/submissions"
	pageFormsList   = "/sites/forms"
)

type SubmissionReader interface {
	Submissions(ctx context.Context, q siteforms.SubmissionQuery) (siteforms.SubmissionPage, error)
}

type PageFormLister interface {
	PageForms(ctx context.Context, workspaceID string) ([]siteforms.PageForm, error)
}

type Reader interface {
	SubmissionReader
	PageFormLister
}

type Handler struct {
	forms Reader
}

func NewHandler(forms Reader) *Handler {
	return &Handler{forms: forms}
}

type AccessControl func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc

func RegisterProtectedRoutes(protected *mux.Router, h *Handler, ac AccessControl) {
	if h == nil {
		return
	}
	protected.HandleFunc(submissionsList, ac(workspace_domain.ResourceSiteSubmissions, workspace_domain.ActionRead, h.List)).Methods(http.MethodGet)
	protected.HandleFunc(pageFormsList, ac(workspace_domain.ResourceSites, workspace_domain.ActionRead, h.PageForms)).Methods(http.MethodGet)
}

type SubmissionResponse struct {
	ID               string                 `json:"id"`
	DeploymentID     string                 `json:"deploymentId"`
	FormID           string                 `json:"formId" example:"contato"`
	Route            string                 `json:"route" example:"/oferta"`
	Variant          string                 `json:"variant,omitempty" example:"b"`
	Name             string                 `json:"name,omitempty" example:"Ana Souza"`
	Contact          string                 `json:"contact,omitempty" example:"+55 11 99999-0000"`
	Source           string                 `json:"source,omitempty" example:"instagram"`
	Fields           map[string]string      `json:"fields"`
	UTM              sitesubmission.UTM     `json:"utm"`
	ReferrerOrigin   string                 `json:"referrerOrigin,omitempty" example:"https://www.google.com"`
	Fbclid           string                 `json:"fbclid,omitempty"`
	Gclid            string                 `json:"gclid,omitempty"`
	Consent          sitesubmission.Consent `json:"consent"`
	UserAgent        string                 `json:"userAgent,omitempty"`
	LeadID           string                 `json:"leadId,omitempty"`
	ConversationID   string                 `json:"conversationId,omitempty"`
	ConversationType string                 `json:"conversationType,omitempty" example:"whatsapp"`
	CreatedAt        time.Time              `json:"createdAt"`
}

type SubmissionListResponse struct {
	Items []SubmissionResponse `json:"items"`
	Total int64                `json:"total"`
}

func present(s sitesubmission.Submission) SubmissionResponse {
	return SubmissionResponse{
		ID: s.ID, DeploymentID: s.DeploymentID, FormID: s.FormID, Route: s.Route, Variant: s.Variant, Name: s.Name(), Contact: s.Contact(), Source: s.Source(), Fields: s.Fields, UTM: s.UTM,
		ReferrerOrigin: s.ReferrerOrigin, Fbclid: s.Fbclid, Gclid: s.Gclid, Consent: s.Consent, UserAgent: s.UserAgent,
		LeadID: s.LeadID, ConversationID: s.ConversationID, ConversationType: s.ConversationType, CreatedAt: s.CreatedAt,
	}
}

func timeParam(r *http.Request, name string) (*time.Time, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, true
	}
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, false
	}
	return &at, true
}

// @Summary		Listar os envios dos formulários de uma página
// @Description	Envios da página, do mais recente para o mais antigo: formulário, rota, variante, nome, contato (o telefone ou, sem ele, o email) e origem (a utm_source ou, sem ela, a origem do referenciador) lidos dos campos do formulário, campos preenchidos, UTM, origem do referenciador, fbclid, gclid, o texto do consentimento aceito, o lead e a conversa de acompanhamento. O IP nunca é guardado. Envios de um lead excluído não aparecem. Exige `site_submissions:read`. Códigos: 400 `submissions_page_invalid`.
// @Tags			Páginas
// @Produce		json
// @Param			id			path		string	true	"página"
// @Param			form		query		string	false	"id do formulário"
// @Param			from		query		string	false	"desde (RFC 3339)"
// @Param			to			query		string	false	"antes de (RFC 3339)"
// @Param			page		query		int		false	"página, a partir de 1"
// @Param			pageSize	query		int		false	"até 100, padrão 30"
// @Success		200			{object}	SubmissionListResponse
// @Failure		400			{object}	response.ErrorResponse
// @Failure		403			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id}/submissions [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	from, okFrom := timeParam(r, "from")
	to, okTo := timeParam(r, "to")
	page, okPage := httpx.QueryInt(r, "page", 1)
	size, okSize := httpx.QueryInt(r, "pageSize", defaultPageSize)
	if !okFrom || !okTo || !okPage || !okSize {
		writeError(w, siteforms.ErrPageInvalid)
		return
	}
	result, err := h.forms.Submissions(r.Context(), siteforms.SubmissionQuery{
		WorkspaceID: workspaceID, ProjectID: mux.Vars(r)["id"], FormID: r.URL.Query().Get("form"), From: from, To: to, Page: page, PageSize: size,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	out := SubmissionListResponse{Items: make([]SubmissionResponse, 0, len(result.Items)), Total: result.Total}
	for _, s := range result.Items {
		out.Items = append(out.Items, present(s))
	}
	response.WriteSuccess(w, http.StatusOK, out)
}
