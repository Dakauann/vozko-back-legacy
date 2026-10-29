package siteformhttp

import (
	"net/http"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
)

type PageFormResponse struct {
	ProjectID   string `json:"projectId" format:"uuid"`
	ProjectName string `json:"projectName" example:"Clínica Sorriso"`
	FormID      string `json:"formId" example:"contato"`
	FormName    string `json:"formName" example:"Fale conosco"`
	FormKey     string `json:"formKey" example:"f_8Jd2kq0Lr7"`
	Live        bool   `json:"live"`
}

type PageFormListResponse struct {
	Items []PageFormResponse `json:"items"`
}

// @Summary		Listar os formulários das páginas
// @Description	Os formulários das páginas do workspace que já receberam uma chave pública (a chave nasce na primeira publicação da página com o formulário), com o nome da página, o id do formulário no código, o nome amigável dado nas configurações (formName, vazio quando não tem) e se ele está no ar agora. A chave é o valor que o gatilho "formulário enviado" dos fluxos usa para os formulários de páginas. Exige `sites:read`.
// @Tags			Páginas
// @Produce		json
// @Success		200	{object}	PageFormListResponse
// @Failure		403	{object}	response.ErrorResponse
// @Failure		500	{object}	response.ErrorResponse	"internal_error"
// @Security		BearerAuth
// @Router			/sites/forms [get]
func (h *Handler) PageForms(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	forms, err := h.forms.PageForms(r.Context(), workspaceID)
	if err != nil {
		writeError(w, err)
		return
	}
	out := PageFormListResponse{Items: make([]PageFormResponse, 0, len(forms))}
	for _, f := range forms {
		out.Items = append(out.Items, PageFormResponse{ProjectID: f.ProjectID, ProjectName: f.ProjectName, FormID: f.FormID, FormName: f.FormName, FormKey: f.FormKey, Live: f.Live})
	}
	response.WriteSuccess(w, http.StatusOK, out)
}
