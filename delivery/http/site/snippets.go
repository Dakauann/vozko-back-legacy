package sitehttp

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/site"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
)

const starterPreviewCache = "public, max-age=86400"

// @Summary		Salvar como seção
// @Description	Guarda o código exato de um elemento da página (node e rev da pré-visualização) como uma seção salva do workspace, no sistema de design da página (ou no padrão do workspace, criado quando não existe nenhum). A seção volta com a chave para insert_section (library_key) e entra em GET /sites/sections como kind snippet. Uma seção de página html cabe nas duas tecnologias; uma de página react, só em react.
// @Tags			Páginas
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"id da página"
// @Param			body	body		SaveSnippetRequest	true	"elemento e nome"
// @Success		201		{object}	SnippetResponse
// @Failure		404		{object}	response.ErrorResponse	"project_not_found"
// @Failure		422		{object}	response.ErrorResponse	"name_invalid, edit_refused (node_stale, node_invalid, field_required), snippets_full"
// @Failure		503		{object}	response.ErrorResponse	"build_unavailable"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/snippets [post]
func (h *Handler) SaveSnippet(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	var body SaveSnippetRequest
	if !decode(w, r, envelopeBytes, &body) {
		return
	}
	saved, err := h.svc.SaveSnippet(r.Context(), site_usecase.SaveSnippetInput{
		WorkspaceID: workspaceID, ProjectID: mux.Vars(r)["id"], Node: body.Node, Rev: body.Rev, Name: body.Name,
	})
	if err != nil {
		writeError(w, err, "Failed to save the section")
		return
	}
	response.WriteSuccess(w, http.StatusCreated, SnippetResponse{
		SectionResponse:     presentSection(sitebuild.SnippetSection(saved.Snippet)),
		DesignSystemID:      saved.DesignSystem.ID,
		DesignSystemVersion: saved.DesignSystem.Version,
	})
}

func (h *Handler) starterPreviewURL(s site_usecase.Starter) string {
	return h.apiOrigin + "/public/sites/starters/" + string(s.Stack) + "/" + s.Key + "/" + site_usecase.StarterPreviewFile
}

// @Summary		Imagem de um modelo de página
// @Description	A captura (JPEG 1440x900) de um modelo de página, pública para servir em img; a mesma de previewUrl em GET /sites/starters.
// @Tags			Páginas
// @Produce		image/jpeg
// @Param			stack	path	string	true	"tecnologia"	Enums(react, html)
// @Param			key		path	string	true	"modelo"
// @Success		200		{file}	binary
// @Failure		404		{string}	string
// @Router			/public/sites/starters/{stack}/{key}/preview.jpg [get]
func (h *Handler) StarterPreview(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	starter, err := site_usecase.StarterFor(site.Stack(vars["stack"]), vars["key"])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", starterPreviewCache)
	w.Header().Set("Content-Length", strconv.Itoa(len(starter.Preview)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(starter.Preview)
}

func RegisterPublicRoutes(public *mux.Router, h *Handler) {
	if h == nil {
		return
	}
	public.HandleFunc("/public/sites/starters/{stack}/{key}/"+site_usecase.StarterPreviewFile, h.StarterPreview).Methods(http.MethodGet)
}
