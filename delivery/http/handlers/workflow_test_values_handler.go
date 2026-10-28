package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/automationneedsx"
	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/workflow"
	workflow_usecase "vozko/usecases/workflow"
)

type TestValuesEditor interface {
	Form(wf *workflow.Workflow, userID string) (*workflow_usecase.TestValuesForm, error)
	Save(wf *workflow.Workflow, userID string, values map[string]string) (*workflow_usecase.TestValuesForm, error)
	Clear(wf *workflow.Workflow, userID string) error
}

type saveTestValuesRequest struct {
	Values map[string]string `json:"values"`
}

type testValuesResponse struct {
	Variables []automationneedsx.NeedResponse `json:"variables"`
	Values    map[string]string               `json:"values"`
}

func testValuesOf(form *workflow_usecase.TestValuesForm) testValuesResponse {
	values := form.Values
	if values == nil {
		values = map[string]string{}
	}
	return testValuesResponse{Variables: automationneedsx.Needs(form.Needs), Values: values}
}

var refusalTestValuesUnwired = workflowRefusal(http.StatusNotImplemented, "test_values_unwired", "Os dados do teste não estão disponíveis")

func (h *WorkflowHandler) testedWorkflow(w http.ResponseWriter, r *http.Request) (*workflow.Workflow, string, bool) {
	viewer, ok := httpx.RequirePerson(w, r)
	if !ok {
		return nil, "", false
	}
	if h.testValues == nil {
		response.WriteRefusal(w, refusalTestValuesUnwired)
		return nil, "", false
	}
	wf, err := h.workflows.Get(workflowScope(r), mux.Vars(r)["id"])
	if err != nil {
		h.handleDomainError(w, err)
		return nil, "", false
	}
	return wf, viewer.UserID, true
}

// @Summary Dados do teste de um fluxo
// @Description Lista as variáveis que o fluxo usa e que um teste precisa receber, pela mesma regra das campanhas e das passagens de conversa: as campvars que os passos leem, mais as variáveis dos agentes e dos subfluxos que o fluxo chama (o rascunho, com agentes e subfluxos publicados). Cada variável vem com a descrição e o caminho via (o último item é quem a usa). values são os valores que quem pede salvou para este fluxo; cada pessoa tem os seus, e eles só valem nos testes (Testar e teste de nó), nunca em execuções de produção nem nas variáveis das conversas. Valores de variáveis que o fluxo não usa mais ficam de fora.
// @Tags workflows
// @Produce json
// @Param id path string true "ID do workflow"
// @Success 200 {object} testValuesResponse
// @Failure 403 {object} response.ErrorResponse "Workflow de outro departamento"
// @Failure 404 {object} response.ErrorResponse "Workflow não encontrado"
// @Failure 422 {object} response.ErrorResponse "automation_needs_not_found: um agente ou subfluxo que o fluxo chama não existe mais"
// @Failure 503 {object} response.ErrorResponse "test_values_unavailable ou automation_needs_unreadable: não foi possível ler agora"
// @Router /workflows/{id}/test-values [get]
func (h *WorkflowHandler) TestValues(w http.ResponseWriter, r *http.Request) {
	wf, userID, ok := h.testedWorkflow(w, r)
	if !ok {
		return
	}
	form, err := h.testValues.Form(wf, userID)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, testValuesOf(form))
}

// @Summary Salvar os dados do teste de um fluxo
// @Description Substitui os valores que quem pede guardou para os testes deste fluxo. values vai por nome de variável; um valor vazio apaga o valor salvo. Só aceita variáveis que o fluxo usa (test_value_not_needed, inclusive lead_name, lead_number e campaign_name, que o teste preenche sozinho) e valores de até 2000 caracteres (test_value_invalid). O Testar começa só quando todas as variáveis têm valor salvo; o teste de nó usa estes valores como ponto de partida.
// @Tags workflows
// @Accept json
// @Produce json
// @Param id path string true "ID do workflow"
// @Param body body saveTestValuesRequest true "Valores por nome de variável"
// @Success 200 {object} testValuesResponse
// @Failure 400 {object} response.ErrorResponse "Corpo inválido (invalid_body) ou valor longo demais (test_value_invalid)"
// @Failure 403 {object} response.ErrorResponse "Workflow de outro departamento"
// @Failure 404 {object} response.ErrorResponse "Workflow não encontrado"
// @Failure 422 {object} response.ErrorResponse "test_value_not_needed: o fluxo não usa essa variável"
// @Failure 503 {object} response.ErrorResponse "test_values_unavailable ou automation_needs_unreadable: não foi possível gravar agora"
// @Router /workflows/{id}/test-values [put]
func (h *WorkflowHandler) SaveTestValues(w http.ResponseWriter, r *http.Request) {
	wf, userID, ok := h.testedWorkflow(w, r)
	if !ok {
		return
	}
	var req saveTestValuesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteRefusal(w, refusalInvalidBody.Expecting(map[string]string{"values": "object of string by variable name"}))
		return
	}
	form, err := h.testValues.Save(wf, userID, req.Values)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, testValuesOf(form))
}

// @Summary Limpar os dados do teste de um fluxo
// @Description Apaga todos os valores que quem pede guardou para os testes deste fluxo. Os de outras pessoas ficam.
// @Tags workflows
// @Param id path string true "ID do workflow"
// @Success 204 "Valores apagados"
// @Failure 403 {object} response.ErrorResponse "Workflow de outro departamento"
// @Failure 404 {object} response.ErrorResponse "Workflow não encontrado"
// @Failure 503 {object} response.ErrorResponse "test_values_unavailable: não foi possível apagar agora"
// @Router /workflows/{id}/test-values [delete]
func (h *WorkflowHandler) ClearTestValues(w http.ResponseWriter, r *http.Request) {
	wf, userID, ok := h.testedWorkflow(w, r)
	if !ok {
		return
	}
	if err := h.testValues.Clear(wf, userID); err != nil {
		h.handleDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
