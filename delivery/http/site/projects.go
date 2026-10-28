package sitehttp

import (
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/site"
	site_usecase "vozko/usecases/site"
)

// @Summary		Listar páginas
// @Description	Páginas do workspace, da editada mais recentemente para a mais antiga, sem os arquivos. Sem status, as arquivadas ficam de fora; q busca pelo nome.
// @Tags			Páginas
// @Produce		json
// @Param			status	query		string	false	"situação"	Enums(draft, published, suspended, archived)
// @Param			q		query		string	false	"busca pelo nome"
// @Param			limit	query		int		false	"até 100, padrão 30"
// @Param			offset	query		int		false	"deslocamento"
// @Success		200		{object}	ListResponse
// @Failure		400		{object}	response.ErrorResponse	"page_invalid, status_invalid"
// @Security		BearerAuth
// @Router			/sites/projects [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	limit, offset, ok := pageOf(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	items, total, err := h.svc.List(r.Context(), site_usecase.ListQuery{WorkspaceID: workspaceID, Status: site_usecase.Status(query.Get("status")), Search: query.Get("q"), Limit: limit, Offset: offset})
	if err != nil {
		writeError(w, err, "Failed to list the pages")
		return
	}
	out := ListResponse{Items: make([]ListItemResponse, 0, len(items)), Total: total}
	for _, p := range items {
		out.Items = append(out.Items, presentListItem(p))
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

// @Summary		Listar modelos de página
// @Description	Modelos prontos por tecnologia (react ou html) para criar uma página sem custo de IA. previewUrl é uma imagem pública (JPEG 1440x900) do modelo.
// @Tags			Páginas
// @Produce		json
// @Success		200	{array}	StarterResponse
// @Security		BearerAuth
// @Router			/sites/starters [get]
func (h *Handler) Starters(w http.ResponseWriter, r *http.Request) {
	all, err := site_usecase.Starters()
	if err != nil {
		writeError(w, err, "Failed to list the starters")
		return
	}
	out := make([]StarterResponse, 0, len(all))
	for _, s := range all {
		out = append(out, StarterResponse{Stack: string(s.Stack), Key: s.Key, Title: s.Title, Description: s.Description, PreviewURL: h.starterPreviewURL(s)})
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

// @Summary		Criar página
// @Description	Cria uma página a partir de um modelo (stack react ou html). Sem name, o nome sai das primeiras palavras de brief (o pedido feito à Elo). O endereço (slug) sai do nome e fica único no workspace. Sem designSystemId, a página herda o sistema de design padrão do workspace. Com origin (a conversa da Elo e o id do comando de tela), repetir o pedido devolve a mesma página.
// @Tags			Páginas
// @Accept			json
// @Produce		json
// @Param			body	body		CreateRequest	true	"página"
// @Success		201		{object}	DetailResponse
// @Failure		404		{object}	response.ErrorResponse
// @Failure		409		{object}	response.ErrorResponse
// @Failure		422		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	workspaceID, person, ok := h.person(w, r)
	if !ok {
		return
	}
	var body CreateRequest
	if !decode(w, r, envelopeBytes, &body) {
		return
	}
	in := site_usecase.CreateInput{WorkspaceID: workspaceID, UserID: person.UserID, Name: body.Name, Brief: body.Brief, Stack: site.Stack(body.Stack), Starter: body.Starter, DesignSystemID: body.DesignSystemID}
	if body.Origin != nil {
		in.Origin = site_usecase.Origin{ThreadID: body.Origin.ThreadID, CommandID: body.Origin.CommandID}
	}
	v, err := h.svc.Create(r.Context(), in)
	if err != nil {
		writeError(w, err, "Failed to create the page")
		return
	}
	response.WriteSuccess(w, http.StatusCreated, presentDetail(v))
}

// @Summary		Duplicar página
// @Description	Cria uma página nova com os arquivos da versão de trabalho, as configurações e o sistema de design da página de origem. Nunca copia domínios nem publicações; a cópia nasce como rascunho e ganha um endereço (slug) novo. Sem nome, usa o nome da origem com " (cópia)".
// @Tags			Páginas
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"id da página de origem"
// @Param			body	body		DuplicateRequest	false	"nome da cópia"
// @Success		201		{object}	DetailResponse
// @Failure		404		{object}	response.ErrorResponse
// @Failure		422		{object}	response.ErrorResponse	"name_invalid"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/duplicate [post]
func (h *Handler) Duplicate(w http.ResponseWriter, r *http.Request) {
	workspaceID, person, ok := h.person(w, r)
	if !ok {
		return
	}
	var body DuplicateRequest
	if r.ContentLength != 0 && !decode(w, r, envelopeBytes, &body) {
		return
	}
	v, err := h.svc.Duplicate(r.Context(), site_usecase.DuplicateInput{WorkspaceID: workspaceID, UserID: person.UserID, ProjectID: mux.Vars(r)["id"], Name: body.Name})
	if err != nil {
		writeError(w, err, "Failed to duplicate the page")
		return
	}
	response.WriteSuccess(w, http.StatusCreated, presentDetail(v))
}

// @Summary		Abrir página
// @Description	A página com todos os arquivos da versão de trabalho, as configurações e a versão atual, que vai no If-Match ao salvar.
// @Tags			Páginas
// @Produce		json
// @Param			id	path		string	true	"id da página"
// @Success		200	{object}	DetailResponse
// @Failure		404	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id} [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), workspaceID, mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err, "Failed to load the page")
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentDetail(v))
}

// @Summary		Alterar página
// @Description	Renomeia, troca o endereço (slug), troca o sistema de design ou arquiva e desarquiva, a partir da versão do If-Match. Uma página no ar não troca de endereço nem é arquivada aqui (409 project_published).
// @Tags			Páginas
// @Accept			json
// @Produce		json
// @Param			id			path		string			true	"id da página"
// @Param			If-Match	header		string			true	"versão carregada"
// @Param			body		body		UpdateRequest	true	"mudanças"
// @Success		200			{object}	ProjectResponse
// @Failure		409			{object}	httpx.VersionConflict{current=DetailResponse}
// @Failure		422			{object}	response.ErrorResponse
// @Failure		428			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id} [patch]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	version, ok := httpx.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var body UpdateRequest
	if !decode(w, r, envelopeBytes, &body) {
		return
	}
	id := mux.Vars(r)["id"]
	p, err := h.svc.Update(r.Context(), workspaceID, id, version, site_usecase.Change{Name: body.Name, Slug: body.Slug, DesignSystemID: body.DesignSystemID, Archived: body.Archived})
	if err != nil {
		h.fail(w, r, workspaceID, id, err, "Failed to update the page")
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentProject(*p))
}

// @Summary		Excluir página
// @Description	Exclui a página, as versões e as configurações. Uma página no ar ou suspensa não é excluída (409 project_published).
// @Tags			Páginas
// @Param			id	path	string	true	"id da página"
// @Success		204
// @Failure		404	{object}	response.ErrorResponse
// @Failure		409	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), workspaceID, mux.Vars(r)["id"]); err != nil {
		writeError(w, err, "Failed to delete the page")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
