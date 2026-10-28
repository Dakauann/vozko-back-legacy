package sitehttp

import (
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
)

// @Summary		Listar sistemas de design
// @Description	Os sistemas de design do workspace (cores, fontes, logo, tom de voz e seções salvas); no máximo um é o padrão das páginas novas.
// @Tags			Páginas
// @Produce		json
// @Success		200	{array}	DesignSystemResponse
// @Security		BearerAuth
// @Router			/sites/design-systems [get]
func (h *Handler) DesignSystems(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListDesignSystems(r.Context(), workspaceID)
	if err != nil {
		writeError(w, err, "Failed to list the design systems")
		return
	}
	out := make([]DesignSystemResponse, 0, len(items))
	for _, d := range items {
		out = append(out, presentDesign(d))
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

// @Summary		Criar sistema de design
// @Description	Cria um sistema de design; com isDefault, ele passa a ser o único padrão do workspace.
// @Tags			Páginas
// @Accept			json
// @Produce		json
// @Param			body	body		DesignSystemRequest	true	"sistema de design"
// @Success		201		{object}	DesignSystemResponse
// @Failure		422		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/design-systems [post]
func (h *Handler) CreateDesignSystem(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	var body DesignSystemRequest
	if !h.decodeSource(w, r, &body) {
		return
	}
	d, err := h.svc.CreateDesignSystem(r.Context(), workspaceID, body.System, body.IsDefault)
	if err != nil {
		writeError(w, err, "Failed to create the design system")
		return
	}
	response.WriteSuccess(w, http.StatusCreated, presentDesign(*d))
}

// @Summary		Abrir sistema de design
// @Tags			Páginas
// @Produce		json
// @Param			id	path		string	true	"id do sistema de design"
// @Success		200	{object}	DesignSystemResponse
// @Failure		404	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/design-systems/{id} [get]
func (h *Handler) DesignSystem(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	d, err := h.svc.GetDesignSystem(r.Context(), workspaceID, mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err, "Failed to load the design system")
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentDesign(*d))
}

// @Summary		Salvar sistema de design
// @Description	Substitui o sistema de design a partir da versão do If-Match.
// @Tags			Páginas
// @Accept			json
// @Produce		json
// @Param			id			path		string				true	"id do sistema de design"
// @Param			If-Match	header		string				true	"versão carregada"
// @Param			body		body		DesignSystemRequest	true	"sistema de design"
// @Success		200			{object}	DesignSystemResponse
// @Failure		404			{object}	response.ErrorResponse
// @Failure		409			{object}	httpx.VersionConflict{current=DesignSystemResponse}
// @Failure		422			{object}	response.ErrorResponse
// @Failure		428			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/design-systems/{id} [put]
func (h *Handler) UpdateDesignSystem(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	version, ok := httpx.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var body DesignSystemRequest
	if !h.decodeSource(w, r, &body) {
		return
	}
	id := mux.Vars(r)["id"]
	d, err := h.svc.UpdateDesignSystem(r.Context(), workspaceID, id, version, body.System, body.IsDefault)
	if isConflict(err) {
		current, getErr := h.svc.GetDesignSystem(r.Context(), workspaceID, id)
		if getErr != nil {
			writeError(w, getErr, "Failed to load the design system")
			return
		}
		httpx.WriteVersionConflict(w, "O sistema de design foi salvo em outra aba ou por outra pessoa", presentDesign(*current))
		return
	}
	if err != nil {
		writeError(w, err, "Failed to save the design system")
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentDesign(*d))
}

// @Summary		Excluir sistema de design
// @Description	Exclui o sistema de design; as páginas que o usavam ficam sem sistema de design.
// @Tags			Páginas
// @Param			id	path	string	true	"id do sistema de design"
// @Success		204
// @Failure		404	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/design-systems/{id} [delete]
func (h *Handler) DeleteDesignSystem(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteDesignSystem(r.Context(), workspaceID, mux.Vars(r)["id"]); err != nil {
		writeError(w, err, "Failed to delete the design system")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
