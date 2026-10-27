package conversation

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	conversationdomain "vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/user"
	"vozko/infra/http/middleware"
	conversation_usecase "vozko/usecases/conversation"
)

type ResponsibleWorkflowReader interface {
	Execute(viewer shared.Person, workspaceID, entryID, entryType string) (*conversation_usecase.ResponsibleWorkflow, error)
}

type responsibleWorkflowResponse struct {
	Workflow *conversation_usecase.ResponsibleWorkflow `json:"workflow"`
}

var (
	refusalResponsibleUnauthorized       = response.Refusal{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "Faça login de novo para continuar"}
	refusalResponsibleConversationHidden = response.Refusal{Status: http.StatusNotFound, Code: "conversation_not_found", Message: "Conversa não encontrada"}
	refusalResponsibleWorkflowFailed     = response.Refusal{Status: http.StatusInternalServerError, Code: "responsible_workflow_failed", Message: "Não foi possível ler o fluxo responsável por esta conversa"}
)

var responsibleWorkflowRefusals = []response.Refusal{refusalResponsibleUnauthorized, refusalResponsibleConversationHidden, refusalResponsibleWorkflowFailed}

// @Summary		Fluxo responsável pela conversa
// @Description	Informa o fluxo que responde por esta conversa, o mesmo que decide os gatilhos Mensagem recebida, Primeira mensagem e Entrou na etapa: o fluxo para o qual a conversa foi delegada ou, sem delegação, o fluxo do número, conta ou campanha da conversa quando os fluxos estão ligados nele. workflow é null quando nenhum fluxo responde (um agente de IA responde, os fluxos estão desligados ou o fluxo não existe mais). state diz se o fluxo roda: active (publicado e ativo), paused (publicado e pausado) ou draft (nunca publicado). Só aparece para quem pode ver a conversa. Uma falha de leitura responde 500 responsible_workflow_failed, nunca um palpite.
// @Tags			Conversas
// @Produce		json
// @Param			entryType	path		string	true	"Tipo da entrada"
// @Param			entryId		path		string	true	"ID da entrada"
// @Success		200	{object}	responsibleWorkflowResponse
// @Failure		401	{object}	response.ErrorResponse	"unauthorized"
// @Failure		404	{object}	response.ErrorResponse	"conversation_not_found"
// @Failure		500	{object}	response.ErrorResponse	"responsible_workflow_failed"
// @Security		BearerAuth
// @Router			/conversations/{entryType}/{entryId}/responsible-workflow [get]
func (h *ConversationHandler) GetResponsibleWorkflow(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	if claims == nil {
		response.WriteRefusal(w, refusalResponsibleUnauthorized)
		return
	}
	vars := mux.Vars(r)
	viewer := shared.Person{UserID: claims.UserID, SystemAdmin: claims.Role == string(user.RoleAdmin)}
	workspaceID := middleware.GetWorkspaceID(r)
	found, err := h.responsibleWorkflow.Execute(viewer, workspaceID, vars["entryId"], vars["entryType"])
	switch {
	case errors.Is(err, conversationdomain.ErrNotVisible):
		response.WriteRefusal(w, refusalResponsibleConversationHidden)
		return
	case err != nil:
		slog.Error("conversation.responsible_workflow.unreadable",
			slog.String("workspace_id", workspaceID), slog.String("entry_id", vars["entryId"]),
			slog.String("entry_type", vars["entryType"]), slog.String("error", err.Error()))
		response.WriteRefusal(w, refusalResponsibleWorkflowFailed)
		return
	}
	response.WriteSuccess(w, http.StatusOK, responsibleWorkflowResponse{Workflow: found})
}
