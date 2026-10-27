package personalinboxhttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/conversation"
	"vozko/domain/personalinbox"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	"vozko/domain/user"
	"vozko/infra/http/middleware"
)

const (
	codeInvalid   = "inbox_preferences_invalid"
	codeForbidden = "inbox_preferences_forbidden"
	codeFailed    = "inbox_preferences_failed"
)

var (
	refusalUnauthorized = response.Refusal{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "Faça login de novo para continuar"}
	refusalInvalid      = response.Refusal{Status: http.StatusBadRequest, Code: codeInvalid, Message: "Informe hideGroups como verdadeiro ou falso"}
	refusalForbidden    = response.Refusal{Status: http.StatusForbidden, Code: codeForbidden, Message: "Você não tem acesso ao atendimento deste workspace"}
	refusalFailed       = response.Refusal{Status: http.StatusInternalServerError, Code: codeFailed, Message: "Não foi possível salvar a sua preferência agora"}
)

var refusals = []response.Refusal{refusalUnauthorized, refusalInvalid, refusalForbidden, refusalFailed}

var saveRefusals = []response.ErrorRefusal{
	{Err: conversation.ErrUnauthorized, Refusal: refusalForbidden},
	{Err: personalinbox.ErrWorkspaceRequired, Refusal: refusalForbidden},
	{Err: personalinbox.ErrUserRequired, Refusal: refusalUnauthorized},
}

func RefusalCodes() []string {
	codes := make([]string, 0, len(refusals))
	for _, r := range refusals {
		codes = append(codes, r.Code)
	}
	return codes
}

type PreferencesService interface {
	InboxPreferences(ctx context.Context, workspaceID, userID string) personalinbox.Preferences
	SaveInboxPreferences(ctx context.Context, by shared.Person, workspaceID string, prefs personalinbox.Preferences) (personalinbox.Preferences, error)
}

type Handler struct {
	preferences PreferencesService
}

func NewHandler(preferences PreferencesService) (*Handler, error) {
	if err := deps.Require("inbox preferences handler", map[string]any{"preferences": preferences}); err != nil {
		return nil, err
	}
	return &Handler{preferences: preferences}, nil
}

func (h *Handler) caller(w http.ResponseWriter, r *http.Request) (shared.Person, string, bool) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return shared.Person{}, "", false
	}
	claims := middleware.GetClaims(r)
	if claims == nil || strings.TrimSpace(claims.UserID) == "" {
		response.WriteRefusal(w, refusalUnauthorized)
		return shared.Person{}, "", false
	}
	return shared.Person{UserID: strings.TrimSpace(claims.UserID), SystemAdmin: claims.Role == string(user.RoleAdmin)}, workspaceID, true
}

// @Summary		Preferências pessoais da caixa de entrada
// @Description	Mostra as preferências de quem está logado na caixa de entrada do workspace do cabeçalho `X-Workspace-ID`. Cada pessoa só vê as próprias.
// @Description
// @Description	Os grupos de WhatsApp ficam sempre na aba Grupos, fora das abas e dos totais das conversas com clientes. Com `hideGroups` verdadeiro, a aba Grupos some para esta pessoa e as mensagens de grupo deixam de tocar som e de mostrar aviso na área de trabalho para ela. Sem preferência salva, ou quando ela não pode ser lida, vale o padrão `hideGroups: false`.
// @Tags			Conversas
// @Produce		json
// @Success		200	{object}	InboxPreferencesResponse
// @Failure		401	{object}	response.ErrorResponse	"unauthorized"
// @Failure		403	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/inbox/preferences [get]
func (h *Handler) Preferences(w http.ResponseWriter, r *http.Request) {
	by, workspaceID, ok := h.caller(w, r)
	if !ok {
		return
	}
	prefs := h.preferences.InboxPreferences(r.Context(), workspaceID, by.UserID)
	response.WriteSuccess(w, http.StatusOK, presentPreferences(prefs))
}

// @Summary		Salvar as preferências pessoais da caixa de entrada
// @Description	Salva as preferências de quem está logado na caixa de entrada deste workspace. `hideGroups` é obrigatório: verdadeiro esconde a aba Grupos e silencia os avisos de mensagens de grupo só para esta pessoa; falso mostra a aba Grupos de novo. A mudança chega em `conversation:inbox_preferences` em todas as conexões da pessoa neste workspace e passa a valer na hora para a lista, a busca e os avisos.
// @Description
// @Description	Códigos: 400 `inbox_preferences_invalid`, 401 `unauthorized`, 403 `inbox_preferences_forbidden` (sem acesso ao atendimento), 500 `inbox_preferences_failed`.
// @Tags			Conversas
// @Accept			json
// @Produce		json
// @Param			body	body		InboxPreferencesRequest	true	"preferências"
// @Success		200		{object}	InboxPreferencesResponse
// @Failure		400		{object}	response.ErrorResponse	"inbox_preferences_invalid"
// @Failure		401		{object}	response.ErrorResponse	"unauthorized"
// @Failure		403		{object}	response.ErrorResponse	"inbox_preferences_forbidden"
// @Failure		500		{object}	response.ErrorResponse	"inbox_preferences_failed"
// @Security		BearerAuth
// @Router			/inbox/preferences [put]
func (h *Handler) SavePreferences(w http.ResponseWriter, r *http.Request) {
	by, workspaceID, ok := h.caller(w, r)
	if !ok {
		return
	}
	var body InboxPreferencesRequest
	if !httpx.DecodeStrictJSON(w, r, &body) {
		return
	}
	if body.HideGroups == nil {
		response.WriteRefusal(w, refusalInvalid)
		return
	}
	saved, err := h.preferences.SaveInboxPreferences(r.Context(), by, workspaceID, personalinbox.Preferences{HideGroups: *body.HideGroups})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		if refusal, known := response.MatchRefusal(err, saveRefusals); known {
			response.WriteRefusal(w, refusal)
			return
		}
		slog.Error("inbox.preferences.save_failed", slog.String("workspace_id", workspaceID), slog.String("user_id", by.UserID), slog.String("error", err.Error()))
		response.WriteRefusal(w, refusalFailed)
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentPreferences(saved))
}
