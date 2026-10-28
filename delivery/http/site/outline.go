package sitehttp

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
)

// @Summary		Pré-visualizar página
// @Description	Quem pode editar (sites:update) compila a versão de trabalho e recebe o endereço assinado e temporário da pré-visualização, que só abre para quem recebeu o link. Quem só lê recebe a pré-visualização já preparada para a versão de trabalho e as configurações atuais, sem compilar nada, ou 409 preview_not_built. As pendências e o mapa da página ficam em GET .../outline.
// @Tags			Páginas
// @Produce		json
// @Param			id	path		string	true	"id da página"
// @Success		200	{object}	PreviewResponse
// @Failure		404	{object}	response.ErrorResponse
// @Failure		409	{object}	response.ErrorResponse	"preview_not_built"
// @Failure		422	{object}	response.ErrorResponse{details=BuildFailedDetails}
// @Failure		503	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id}/preview [post]
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	preview := h.svc.CachedPreview
	if h.canEdit(r, workspaceID) {
		preview = h.svc.Preview
	}
	res, err := preview(r.Context(), workspaceID, mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err, "Failed to prepare the preview")
		return
	}
	response.WriteSuccess(w, http.StatusOK, PreviewResponse{URL: res.URL, RevisionID: res.RevisionID})
}

// @Summary		Mapa da página
// @Description	Devolve o mapa guardado da versão de trabalho com as configurações atuais: formulários, por rota a árvore de elementos, pendências (openGaps) e verificações. O mapa é lido no navegador de quem edita e enviado em POST .../outline; esta rota nunca compila nem renderiza. Sem mapa desta versão, gapsChecked fica false com reasonCode not_inspected. É informativo: a publicação roda as próprias verificações no servidor.
// @Tags			Páginas
// @Produce		json
// @Param			id	path		string	true	"id da página"
// @Success		200	{object}	OutlineResponse
// @Failure		404	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id}/outline [get]
func (h *Handler) Outline(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), workspaceID, mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err, "Failed to outline the page")
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentOutline(h.outlines.Outline(r.Context(), v)))
}

// @Summary		Exportar código da página
// @Description	Um arquivo ZIP com o código-fonte da versão de trabalho, exatamente como aparece na aba Código.
// @Tags			Páginas
// @Produce		application/zip
// @Param			id	path		string	true	"id da página"
// @Success		200	{file}		binary
// @Failure		404	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id}/export [get]
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	name, body, err := h.svc.Export(r.Context(), workspaceID, mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err, "Failed to export the page")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
