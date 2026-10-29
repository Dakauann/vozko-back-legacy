package tutorialhttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/shared/deps"
	"vozko/domain/tutorial"
	"vozko/infra/http/middleware"
	tutorial_usecase "vozko/usecases/tutorial"
)

const (
	codeIDInvalid       = "tutorial_id_invalid"
	codeProgressInvalid = "tutorial_progress_invalid"
	codeProgressFailed  = "tutorial_progress_failed"
	codeChecklistFailed = "tutorial_checklist_failed"
)

var (
	refusalUnauthorized    = response.Refusal{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "Faça login de novo para continuar"}
	refusalIDInvalid       = response.Refusal{Status: http.StatusBadRequest, Code: codeIDInvalid, Message: "O tutorial precisa de um id no formato area.nome"}
	refusalProgressInvalid = response.Refusal{Status: http.StatusBadRequest, Code: codeProgressInvalid, Message: "Informe status in_progress, completed ou dismissed e um passo de 0 a 50"}
	refusalProgressFailed  = response.Refusal{Status: http.StatusInternalServerError, Code: codeProgressFailed, Message: "Não foi possível ler ou salvar o seu progresso agora"}
	refusalChecklistFailed = response.Refusal{Status: http.StatusInternalServerError, Code: codeChecklistFailed, Message: "Não foi possível conferir os primeiros passos agora"}
)

var refusals = []response.Refusal{refusalUnauthorized, refusalIDInvalid, refusalProgressInvalid, refusalProgressFailed, refusalChecklistFailed}

var progressRefusals = []response.ErrorRefusal{
	{Err: tutorial.ErrUserRequired, Refusal: refusalUnauthorized},
	{Err: tutorial.ErrInvalidID, Refusal: refusalIDInvalid},
	{Err: tutorial.ErrInvalidProgress, Refusal: refusalProgressInvalid},
}

func RefusalCodes() []string {
	return response.RefusalCodes(refusals)
}

type Service interface {
	Progress(ctx context.Context, userID string) ([]tutorial.Progress, error)
	SaveProgress(ctx context.Context, userID string, change tutorial_usecase.Change) (tutorial.Progress, error)
	ResetProgress(ctx context.Context, userID, tutorialID string) error
	SitesChecklist(ctx context.Context, workspaceID string) (tutorial.Checklist, error)
}

type Handler struct {
	service Service
}

func NewHandler(service Service) (*Handler, error) {
	if err := deps.Require("tutorial handler", map[string]any{"service": service}); err != nil {
		return nil, err
	}
	return &Handler{service: service}, nil
}

func (h *Handler) person(w http.ResponseWriter, r *http.Request) (string, bool) {
	claims := middleware.GetClaims(r)
	if claims == nil || strings.TrimSpace(claims.UserID) == "" {
		response.WriteRefusal(w, refusalUnauthorized)
		return "", false
	}
	return strings.TrimSpace(claims.UserID), true
}

func (h *Handler) fail(w http.ResponseWriter, err error, fallback response.Refusal, event string, attrs ...slog.Attr) {
	if errors.Is(err, context.Canceled) {
		return
	}
	if refusal, known := response.MatchRefusal(err, progressRefusals); known {
		response.WriteRefusal(w, refusal)
		return
	}
	slog.LogAttrs(context.Background(), slog.LevelError, event, append(attrs, slog.String("error", err.Error()))...)
	response.WriteRefusal(w, fallback)
}

// @Summary		Progresso nos tutoriais
// @Description	Lista o progresso de quem está logado nos tutoriais guiados (tours) do app. O progresso é da pessoa, vale em todos os workspaces dela e só existe para tutoriais que ela já começou, concluiu ou dispensou: um tutorial fora da lista ainda não foi visto.
// @Description
// @Description	`status` é `in_progress`, `completed` ou `dismissed`; `step` é o último passo visto, a partir de 0. Quando o progresso não pode ser lido a resposta é 500 `tutorial_progress_failed`, nunca uma lista vazia, e o app trata os tutoriais como não concluídos.
// @Tags			Tutoriais
// @Produce		json
// @Success		200	{object}	ProgressListResponse
// @Failure		401	{object}	response.ErrorResponse	"unauthorized"
// @Failure		500	{object}	response.ErrorResponse	"tutorial_progress_failed"
// @Security		BearerAuth
// @Router			/tutorials/progress [get]
func (h *Handler) ListProgress(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.person(w, r)
	if !ok {
		return
	}
	list, err := h.service.Progress(r.Context(), userID)
	if err != nil {
		h.fail(w, err, refusalProgressFailed, "tutorial.progress.list_failed", slog.String("user_id", userID))
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentProgressList(list))
}

// @Summary		Salvar o progresso em um tutorial
// @Description	Salva o progresso de quem está logado em um tutorial. O id tem o formato `area.nome` (por exemplo `sites.editor`), com letras e números. Um tutorial concluído continua concluído quando chega `in_progress` ou `dismissed` depois: só o `DELETE` recomeça.
// @Description
// @Description	Códigos: 400 `tutorial_id_invalid`, 400 `tutorial_progress_invalid` (status desconhecido ou passo fora de 0 a 50), 400 `invalid_body`, 401 `unauthorized`, 500 `tutorial_progress_failed`.
// @Tags			Tutoriais
// @Accept			json
// @Produce		json
// @Param			tutorialId	path		string			true	"id do tutorial"
// @Param			body		body		ProgressRequest	true	"progresso"
// @Success		200			{object}	ProgressResponse
// @Failure		400			{object}	response.ErrorResponse	"tutorial_id_invalid, tutorial_progress_invalid ou invalid_body"
// @Failure		401			{object}	response.ErrorResponse	"unauthorized"
// @Failure		500			{object}	response.ErrorResponse	"tutorial_progress_failed"
// @Security		BearerAuth
// @Router			/tutorials/progress/{tutorialId} [put]
func (h *Handler) SaveProgress(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.person(w, r)
	if !ok {
		return
	}
	var body ProgressRequest
	if !httpx.DecodeStrictJSON(w, r, &body) {
		return
	}
	if body.Status == nil {
		response.WriteRefusal(w, refusalProgressInvalid)
		return
	}
	tutorialID := mux.Vars(r)["tutorialId"]
	saved, err := h.service.SaveProgress(r.Context(), userID, tutorial_usecase.Change{TutorialID: tutorialID, Status: tutorial.Status(*body.Status), Step: body.Step})
	if err != nil {
		h.fail(w, err, refusalProgressFailed, "tutorial.progress.save_failed", slog.String("user_id", userID), slog.String("tutorial_id", tutorialID))
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentProgress(saved))
}

// @Summary		Recomeçar um tutorial
// @Description	Apaga o progresso de quem está logado em um tutorial, para que ele apareça de novo como não visto. Apagar um tutorial sem progresso também responde 204.
// @Description
// @Description	Códigos: 400 `tutorial_id_invalid`, 401 `unauthorized`, 500 `tutorial_progress_failed`.
// @Tags			Tutoriais
// @Param			tutorialId	path	string	true	"id do tutorial"
// @Success		204
// @Failure		400	{object}	response.ErrorResponse	"tutorial_id_invalid"
// @Failure		401	{object}	response.ErrorResponse	"unauthorized"
// @Failure		500	{object}	response.ErrorResponse	"tutorial_progress_failed"
// @Security		BearerAuth
// @Router			/tutorials/progress/{tutorialId} [delete]
func (h *Handler) ResetProgress(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.person(w, r)
	if !ok {
		return
	}
	tutorialID := mux.Vars(r)["tutorialId"]
	if err := h.service.ResetProgress(r.Context(), userID, tutorialID); err != nil {
		h.fail(w, err, refusalProgressFailed, "tutorial.progress.reset_failed", slog.String("user_id", userID), slog.String("tutorial_id", tutorialID))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary		Primeiros passos dos Sites
// @Description	Mostra os primeiros passos dos Sites no workspace do cabeçalho `X-Workspace-ID`, conferidos nos dados reais do workspace (de qualquer pessoa da equipe). Exige `sites:read`.
// @Description
// @Description	| Item | Conta como feito quando |
// @Description	|---|---|
// @Description	| `create_page` | existe uma página |
// @Description	| `edit_by_hand` | uma pessoa editou uma página à mão (salvamento automático de pessoa) |
// @Description	| `form_to_funnel` | um formulário de página envia leads para um funil |
// @Description	| `site_chat` | uma página tem o chat do site ligado |
// @Description	| `booking` | uma página tem um bloco de agendamento configurado |
// @Description	| `custom_domain` | um domínio próprio está ativo |
// @Description	| `publish` | uma página já foi ao ar |
// @Description
// @Description	A ordem dos itens é a ordem sugerida. Quando os dados não podem ser lidos a resposta é 500 `tutorial_checklist_failed`, nunca itens marcados como feitos.
// @Tags			Tutoriais
// @Produce		json
// @Success		200	{object}	ChecklistResponse
// @Failure		401	{object}	response.ErrorResponse	"unauthorized"
// @Failure		403	{object}	response.ErrorResponse
// @Failure		500	{object}	response.ErrorResponse	"tutorial_checklist_failed"
// @Security		BearerAuth
// @Router			/tutorials/checklists/sites [get]
func (h *Handler) SitesChecklist(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.person(w, r); !ok {
		return
	}
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	list, err := h.service.SitesChecklist(r.Context(), workspaceID)
	if err != nil {
		h.fail(w, err, refusalChecklistFailed, "tutorial.checklist.sites_failed", slog.String("workspace_id", workspaceID))
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentChecklist(list))
}
