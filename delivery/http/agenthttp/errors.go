package agenthttp

import (
	"errors"
	"net/http"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/testinput"
	workspace_department "vozko/domain/workspace/workspace_department"
	agent_usecase "vozko/usecases/agent"
	agenttool_usecase "vozko/usecases/agenttool"
)

func refusal(status int, code, message string) response.Refusal {
	return response.Refusal{Status: status, Code: code, Message: message}
}

var (
	RefusalInternal            = refusal(http.StatusInternalServerError, "agent_internal_error", "Erro inesperado. Tente de novo.")
	RefusalInvalidBody         = refusal(http.StatusBadRequest, "agent_invalid_body", "Corpo da requisição inválido")
	RefusalForbidden           = refusal(http.StatusForbidden, "agent_forbidden", "Você não tem acesso a este agente")
	RefusalNotActivatable      = refusal(http.StatusUnprocessableEntity, agenttool.NotActivatableCode, "O agente tem problemas que impedem salvar com ele ativo")
	RefusalNotPublishable      = refusal(http.StatusUnprocessableEntity, agenttool.NotPublishableCode, "O rascunho tem problemas que impedem publicar")
	RefusalToolsMoved          = refusal(http.StatusBadRequest, "agent_tools_moved", "As ferramentas do agente são salvas em /agents/{id}/tools; tire internalTools do corpo").Expecting(map[string]string{"internalTools": "use /agents/{id}/tools"})
	RefusalMCPCollectionsMoved = refusal(http.StatusBadRequest, "agent_mcp_collections_moved", "Servidores MCP agora são ferramentas do agente (Servidor MCP); tire mcpCollectionIds do corpo").Expecting(map[string]string{"mcpCollectionIds": "use an mcp_source tool"})
)

var RequestRefusals = []response.Refusal{RefusalInternal, RefusalInvalidBody, RefusalForbidden, RefusalNotActivatable, RefusalNotPublishable, RefusalToolsMoved, RefusalMCPCollectionsMoved}

var (
	departmentInvalid = refusal(http.StatusBadRequest, "agent_department_invalid", "Departamento inválido ou sem acesso").Expecting(map[string]string{"departmentId": "invalid"})
	providerInvalid   = refusal(http.StatusBadRequest, "agent_provider_invalid", "Provedor não suportado").Expecting(map[string]string{"provider": "unsupported"})
)

var Refusals = []response.ErrorRefusal{
	{Err: agent.ErrAgentNotFound, Refusal: refusal(http.StatusNotFound, "agent_not_found", "Agente não encontrado")},
	{Err: agent.ErrAgentNameRequired, Refusal: refusal(http.StatusBadRequest, "agent_name_required", "Informe o nome do agente").Expecting(map[string]string{"name": "required"})},
	{Err: agent.ErrAgentNameTooLong, Refusal: refusal(http.StatusBadRequest, "agent_name_too_long", "O nome do agente tem no máximo 120 caracteres").Expecting(map[string]string{"name": "max 120"})},
	{Err: agent.ErrAgentMessagingPromptRequired, Refusal: refusal(http.StatusBadRequest, "agent_messaging_prompt_required", "Informe o prompt de mensagens").Expecting(map[string]string{"messagingPrompt": "required"})},
	{Err: agent.ErrAgentProviderRequired, Refusal: refusal(http.StatusBadRequest, "agent_provider_required", "Informe o provedor").Expecting(map[string]string{"provider": "required"})},
	{Err: agent.ErrAgentProviderInvalid, Refusal: providerInvalid},
	{Err: agent.ErrAgentProviderUnsupported, Refusal: providerInvalid},
	{Err: agent.ErrAgentProviderUnavailable, Refusal: refusal(http.StatusFailedDependency, "agent_provider_unavailable", "O provedor está indisponível agora")},
	{Err: agent.ErrAgentMessagingModelRequired, Refusal: refusal(http.StatusBadRequest, "agent_messaging_model_required", "Informe o modelo de mensagens").Expecting(map[string]string{"messagingModel": "required"})},
	{Err: agent.ErrAgentMessagingModelTooLong, Refusal: refusal(http.StatusBadRequest, "agent_messaging_model_too_long", "O nome do modelo tem no máximo 64 caracteres").Expecting(map[string]string{"messagingModel": "max 64"})},
	{Err: agent.ErrAgentMetadataInvalid, Refusal: refusal(http.StatusBadRequest, "agent_metadata_invalid", "Os metadados precisam ser um objeto JSON de textos").Expecting(map[string]string{"metadata": "object of strings"})},
	{Err: agent.ErrAgentInternalToolInvalid, Refusal: refusal(http.StatusBadRequest, "agent_tool_invalid", "Uma ferramenta escolhida é inválida ou está indisponível").Expecting(map[string]string{"internalTools": "invalid"})},
	{Err: agent.ErrAgentInternalToolsRequired, Refusal: refusal(http.StatusBadRequest, "agent_tools_required", "Envie a lista de ferramentas, mesmo vazia").Expecting(map[string]string{"internalTools": "required"})},
	{Err: agent.ErrAgentVariableNameInvalid, Refusal: refusal(http.StatusBadRequest, "agent_variable_name_invalid", "Nomes de variáveis usam só letras, dígitos e _").Expecting(map[string]string{"variables": "letters, digits and underscore"})},
	{Err: agent.ErrAgentVariableDuplicate, Refusal: refusal(http.StatusBadRequest, "agent_variable_duplicate", "Os nomes das variáveis precisam ser únicos").Expecting(map[string]string{"variables": "duplicate"})},
	{Err: agent.ErrAgentVariableTooMany, Refusal: refusal(http.StatusBadRequest, "agent_variable_too_many", "O agente tem no máximo 50 variáveis").Expecting(map[string]string{"variables": "max 50"})},
	{Err: agent.ErrAgentKnowledgeBaseNoAccess, Refusal: refusal(http.StatusBadRequest, "agent_knowledge_base_no_access", "Base de conhecimento não encontrada neste workspace").Expecting(map[string]string{"knowledgeBaseIds": "not found"})},
	{Err: workspace_department.ErrDepartmentRequired, Refusal: refusal(http.StatusBadRequest, "agent_department_required", "Escolha o departamento do agente").Expecting(map[string]string{"departmentId": "required"})},
	{Err: workspace_department.ErrDepartmentAccessDenied, Refusal: departmentInvalid},
	{Err: workspace_department.ErrDepartmentNotFound, Refusal: departmentInvalid},
	{Err: agent.ErrAgentNotPublished, Refusal: refusal(http.StatusConflict, "agent_not_published", "O agente ainda não tem versão publicada")},
	{Err: agent.ErrAgentVersionMissing, Refusal: refusal(http.StatusNotFound, "agent_version_not_found", "Versão do agente não encontrada")},
	{Err: agent_usecase.ErrRealCallNotConfirmed, Refusal: refusal(http.StatusUnprocessableEntity, "agent_tool_test_real_call_not_confirmed", "Este método altera dados no sistema externo. Confirme a chamada real para testar")},
	{Err: agent_usecase.ErrTestInputTooLarge, Refusal: refusal(http.StatusRequestEntityTooLarge, "agent_tool_test_input_too_large", "Os argumentos do teste passam de 64 KB")},
	{Err: agent_usecase.ErrTestInputMissing, Refusal: refusal(http.StatusNotFound, "agent_tool_test_input_not_found", "Exemplo salvo não encontrado")},
	{Err: agent_usecase.ErrToolTestUnsupported, Refusal: refusal(http.StatusUnprocessableEntity, "agent_tool_test_unsupported", "Este tipo de ferramenta é testado num cenário do simulador")},
	{Err: testinput.ErrTooLarge, Refusal: refusal(http.StatusRequestEntityTooLarge, "agent_tool_test_input_too_large", "Os argumentos do teste passam de 64 KB")},
	{Err: testinput.ErrInvalid, Refusal: refusal(http.StatusBadRequest, "agent_tool_test_input_invalid", "Cada exemplo salvo precisa de nome único e JSON válido")},
	{Err: agenttool.ErrToolNotFound, Refusal: refusal(http.StatusNotFound, "agent_tool_not_found", "Ferramenta não encontrada neste agente")},
	{Err: agenttool.ErrTypeUnknown, Refusal: refusal(http.StatusBadRequest, "agent_tool_type_unknown", "Tipo de ferramenta desconhecido")},
	{Err: agenttool.ErrNameInvalid, Refusal: refusal(http.StatusBadRequest, "agent_tool_name_invalid", "O nome da função usa letras minúsculas, dígitos e _, começa com letra e tem até 64 caracteres").Expecting(map[string]string{"name": "lowercase snake case, 1 to 64"})},
	{Err: agenttool.ErrNameTaken, Refusal: refusal(http.StatusConflict, "agent_tool_name_taken", "Outra ferramenta deste agente já usa esse nome").Expecting(map[string]string{"name": "taken"})},
	{Err: agenttool.ErrTargetTaken, Refusal: refusal(http.StatusConflict, "agent_tool_target_taken", "Este agente já tem uma ferramenta deste tipo para o mesmo destino")},
	{Err: agenttool.ErrSingleTypeTaken, Refusal: refusal(http.StatusConflict, "agent_tool_single_type_taken", "Este tipo de ferramenta vale uma vez por agente")},
	{Err: agenttool.ErrRevisionConflict, Refusal: refusal(http.StatusConflict, "agent_tool_revision_conflict", "A ferramenta mudou desde que você abriu. Recarregue e tente de novo")},
	{Err: agenttool.ErrDescriptionRequired, Refusal: refusal(http.StatusBadRequest, "agent_tool_description_required", "Descreva quando a IA deve usar esta ferramenta").Expecting(map[string]string{"description": "required"})},
	{Err: agenttool.ErrOrderMismatch, Refusal: refusal(http.StatusBadRequest, "agent_tool_order_mismatch", "A ordem precisa listar cada ferramenta do agente uma vez").Expecting(map[string]string{"ids": "every tool once"})},
	{Err: agenttool.ErrRevisionNotFound, Refusal: refusal(http.StatusNotFound, "agent_tool_revision_not_found", "Versão da ferramenta não encontrada")},
	{Err: agenttool.ErrValidationOffline, Refusal: refusal(http.StatusServiceUnavailable, "agent_tool_validation_unavailable", "Não foi possível conferir as ferramentas agora. Tente de novo em instantes")},
	{Err: agenttool_usecase.ErrSecretStoreUnavailable, Refusal: refusal(http.StatusServiceUnavailable, "agent_tool_secret_store_unavailable", "Os segredos do workspace estão indisponíveis; a credencial não foi salva")},
	{Err: agenttool_usecase.ErrSecretNamesExhausted, Refusal: refusal(http.StatusConflict, "agent_tool_secret_name_exhausted", "Não há nome livre para guardar esta credencial; remova segredos antigos do agente")},
}

func WriteError(w http.ResponseWriter, err error) {
	if httpx.WriteConsentGrantRefusal(w, err) || httpx.WriteSheetBindingRefusal(w, err) {
		return
	}
	var blocked *agenttool.BlockedError
	if errors.As(err, &blocked) {
		refusal := RefusalNotActivatable
		if errors.Is(err, agenttool.ErrNotPublishable) {
			refusal = RefusalNotPublishable
		}
		response.WriteErrorWithDetails(w, refusal.Status, refusal.Code, refusal.Message, blocked.Report)
		return
	}
	if errors.Is(err, agent.ErrAgentProviderUnavailable) {
		response.WriteErrorWithCode(w, http.StatusFailedDependency, "agent_provider_unavailable", err.Error(), nil)
		return
	}
	if !response.WriteErrorRefusal(w, err, Refusals) {
		response.WriteRefusal(w, RefusalInternal)
	}
}
