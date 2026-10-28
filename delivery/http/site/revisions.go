package sitehttp

import (
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	site_usecase "vozko/usecases/site"
)

// @Summary		Listar versões da página
// @Description	As versões da página, da mais recente para a mais antiga, com o tipo (automática, salva, da Elo, publicação, restauração), o autor e o nome.
// @Tags			Páginas
// @Produce		json
// @Param			id		path		string	true	"id da página"
// @Param			limit	query		int		false	"até 100, padrão 30"
// @Param			offset	query		int		false	"deslocamento"
// @Success		200		{object}	RevisionListResponse
// @Failure		400		{object}	response.ErrorResponse	"page_invalid"
// @Failure		404		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id}/revisions [get]
func (h *Handler) Revisions(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	limit, offset, ok := pageOf(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.ListRevisions(r.Context(), workspaceID, mux.Vars(r)["id"], limit, offset)
	if err != nil {
		writeError(w, err, "Failed to list the versions")
		return
	}
	out := RevisionListResponse{Items: make([]RevisionResponse, 0, len(items)), Total: total}
	for _, rev := range items {
		out.Items = append(out.Items, presentRevision(rev))
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

// @Summary		Salvar versão
// @Description	Guarda o estado atual como uma versão com nome (Salvar versão), a partir da versão do If-Match.
// @Tags			Páginas
// @Accept			json
// @Produce		json
// @Param			id			path		string				true	"id da página"
// @Param			If-Match	header		string				true	"versão carregada"
// @Param			body		body		SaveVersionRequest	true	"nome da versão"
// @Success		201			{object}	SavedResponse
// @Failure		409			{object}	httpx.VersionConflict{current=DetailResponse}
// @Failure		422			{object}	response.ErrorResponse
// @Failure		428			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id}/revisions [post]
func (h *Handler) SaveVersion(w http.ResponseWriter, r *http.Request) {
	workspaceID, person, ok := h.person(w, r)
	if !ok {
		return
	}
	version, ok := httpx.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var body SaveVersionRequest
	if !decode(w, r, envelopeBytes, &body) {
		return
	}
	id := mux.Vars(r)["id"]
	v, err := h.svc.SaveVersion(r.Context(), site_usecase.SaveVersionInput{WorkspaceID: workspaceID, ProjectID: id, ExpectedVersion: version, Author: site_usecase.PersonAuthor(person.UserID), Label: body.Label})
	if err != nil {
		h.fail(w, r, workspaceID, id, err, "Failed to save the version")
		return
	}
	response.WriteSuccess(w, http.StatusCreated, presentSaved(v))
}

// @Summary		Restaurar versão
// @Description	Cria uma nova versão com o conteúdo da versão escolhida, a partir da versão do If-Match; nada do histórico é apagado. Devolve todos os arquivos restaurados.
// @Tags			Páginas
// @Produce		json
// @Param			id			path		string	true	"id da página"
// @Param			rev			path		string	true	"id da versão"
// @Param			If-Match	header		string	true	"versão carregada"
// @Success		200			{object}	FilesResponse
// @Failure		404			{object}	response.ErrorResponse
// @Failure		409			{object}	httpx.VersionConflict{current=DetailResponse}
// @Failure		428			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id}/revisions/{rev}/restore [post]
func (h *Handler) Restore(w http.ResponseWriter, r *http.Request) {
	workspaceID, person, ok := h.person(w, r)
	if !ok {
		return
	}
	version, ok := httpx.IfMatchVersion(w, r)
	if !ok {
		return
	}
	vars := mux.Vars(r)
	v, err := h.svc.Restore(r.Context(), site_usecase.RestoreInput{WorkspaceID: workspaceID, ProjectID: vars["id"], RevisionID: vars["rev"], ExpectedVersion: version, Author: site_usecase.PersonAuthor(person.UserID)})
	if err != nil {
		h.fail(w, r, workspaceID, vars["id"], err, "Failed to restore the version")
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentFiles(v))
}
