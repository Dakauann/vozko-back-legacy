package sitehttp

import (
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/site"
	site_usecase "vozko/usecases/site"
)

// @Summary		Ler os arquivos da página
// @Description	Os arquivos da versão de trabalho com o rev de cada um, a versão de trabalho e a versão da página.
// @Tags			Páginas
// @Produce		json
// @Param			id	path		string	true	"id da página"
// @Success		200	{object}	FilesResponse
// @Failure		404	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id}/files [get]
func (h *Handler) Files(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), workspaceID, mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err, "Failed to load the files")
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentFiles(v))
}

// @Summary		Salvar arquivos editados no código
// @Description	Grava o conteúdo inteiro dos arquivos alterados e exclui os listados em deleted, a partir da versão do If-Match. Cada arquivo alterado ganha um novo rev. Recusa com 422 e o código da primeira regra quebrada (caminho, tamanho, diretivas de CSS, site.json) quando o envio cria um problema novo; problemas antigos em arquivos não tocados não bloqueiam. Edições seguidas da mesma pessoa em até 3 minutos atualizam a mesma versão automática. A resposta traz em changes cada arquivo alterado com o conteúdo e o rev depois de salvar (files), os excluídos (deleted) e os renomeados (renamed), e em facts as rotas, os tokens do tema, os knobs com o valor atual e os breakpoints lidos do resultado.
// @Tags			Páginas
// @Accept			json
// @Produce		json
// @Param			id			path		string				true	"id da página"
// @Param			If-Match	header		string				true	"versão carregada"
// @Param			body		body		SaveFilesRequest	true	"arquivos"
// @Success		200			{object}	SavedResponse
// @Failure		409			{object}	httpx.VersionConflict{current=DetailResponse}
// @Failure		413			{object}	response.ErrorResponse
// @Failure		422			{object}	response.ErrorResponse{details=IssuesDetails}
// @Failure		428			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id}/files [put]
func (h *Handler) SaveFiles(w http.ResponseWriter, r *http.Request) {
	workspaceID, person, ok := h.person(w, r)
	if !ok {
		return
	}
	version, ok := httpx.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var body SaveFilesRequest
	if !h.decodeSource(w, r, &body) {
		return
	}
	id := mux.Vars(r)["id"]
	v, err := h.svc.SaveFiles(r.Context(), site_usecase.SaveFilesInput{
		WorkspaceID: workspaceID, ProjectID: id, ExpectedVersion: version, Author: site_usecase.PersonAuthor(person.UserID), Files: body.Files, Deleted: body.Deleted,
	})
	if err != nil {
		h.fail(w, r, workspaceID, id, err, "Failed to save the files")
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentSaved(v))
}

// @Summary		Aplicar operações na página
// @Description	Executa um lote de operações visuais, de código e de tema (o mesmo contrato da Elo) a partir da versão do If-Match. O lote é atômico: qualquer operação recusada recusa tudo com 422 edit_refused e a lista de falhas, das dependentes puladas e dos problemas novos. write_file de arquivo existente, delete_file e rename_file levam o rev lido do arquivo (file_stale quando ele mudou depois); nodes velhos são mapeados pelo histórico de cada arquivo. set_text aceita text (marcação inline) ou html (o subconjunto do editor: b, strong, i, em, u, a, br, parágrafos). Atalhos expandidos no servidor: define_knob(name, variable, kind, min, max, step, unit), rename_knob(name, to) e remove_knob(name) reescrevem knobs.json; set_whatsapp_link(node, rev, number_ref, text) e clear_whatsapp_link(node, rev) mexem nos atributos data-vz-whatsapp, data-vz-text e href, com o número validado contra o workspace (whatsapp_number_unknown) e o texto até 500 caracteres. insert_element(at, rev, element, key, ref) insere um bloco da plataforma escrito pelo servidor, nunca HTML livre: element webchat com a chave pública ou o id de um chat do site deste workspace (webchat_widget_unknown) ou booking com o id de um bloco de agendamento salvo nas configurações (booking_unknown); outro element responde element_unknown e uma chave malformada element_key_invalid. set_form_field(node, rev, field, input, required) liga um campo de formulário a um campo do lead escrevendo name, type e required (form_field_unknown, form_field_type_invalid); insert_form_field(at, rev, field, input, text, required, ref) insere um campo novo escrito pelo servidor, com rótulo de até 120 caracteres (form_label_too_long); GET /sites/forms/targets lista os campos e tipos aceitos. Os índices de falhas e passos apontam para a operação enviada. Devolve os passos (para a reaplicação da Elo), o novo rev de cada arquivo, changes (arquivos alterados com conteúdo e rev, excluídos e renomeados, para o editor substituir sem reaplicar passos) e facts.
// @Tags			Páginas
// @Accept			json
// @Produce		json
// @Param			id			path		string		true	"id da página"
// @Param			If-Match	header		string		true	"versão carregada"
// @Param			body		body		EditRequest	true	"operações"
// @Success		200			{object}	EditResponse
// @Failure		400			{object}	response.ErrorResponse
// @Failure		409			{object}	httpx.VersionConflict{current=DetailResponse}
// @Failure		413			{object}	response.ErrorResponse
// @Failure		422			{object}	response.ErrorResponse{details=EditRefusalDetails}
// @Failure		428			{object}	response.ErrorResponse
// @Failure		502			{object}	response.ErrorResponse
// @Failure		503			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id}/edits [post]
func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	workspaceID, person, ok := h.person(w, r)
	if !ok {
		return
	}
	version, ok := httpx.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var body EditRequest
	if !h.decodeSource(w, r, &body) {
		return
	}
	id := mux.Vars(r)["id"]
	res, err := h.svc.Edit(r.Context(), site_usecase.EditInput{
		WorkspaceID: workspaceID, ProjectID: id, ExpectedVersion: version, Author: site_usecase.PersonAuthor(person.UserID), Batch: body.Operations,
	})
	if err != nil {
		h.fail(w, r, workspaceID, id, err, "Failed to edit the page")
		return
	}
	steps := res.Steps
	if steps == nil {
		steps = []site.StepPatch{}
	}
	response.WriteSuccess(w, http.StatusOK, EditResponse{
		Project: presentProject(res.View.Project), Revision: presentRevision(res.View.Revision), Revs: revs(res.View.Files), Steps: steps, Ignored: res.Ignored,
		Changes: changesOf(&res.View), Facts: presentFacts(&res.View),
	})
}
