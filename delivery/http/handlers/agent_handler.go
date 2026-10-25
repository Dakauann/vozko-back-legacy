package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"vozko/delivery/http/agenthttp"
	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/ai"
	"vozko/domain/auth"
	"vozko/domain/shared"
	"vozko/domain/tools"
	"vozko/domain/user"
	"vozko/infra/http/middleware"
)

type AgentHandler struct {
	createUseCase           agent.CreateAgentUseCase
	updateUseCase           agent.UpdateAgentUseCase
	assignDepartmentUseCase agent.AssignDepartmentUseCase
	deleteUseCase           agent.DeleteAgentUseCase
	getUseCase              agent.GetAgentUseCase
	listUseCase             agent.ListAgentsUseCase
	simulateUseCase         agent.SimulateTurnUseCase
	toolRegistry            tools.Service
	aiService               ai.Service
}

func NewAgentHandler(
	createUC agent.CreateAgentUseCase,
	updateUC agent.UpdateAgentUseCase,
	assignDepartmentUC agent.AssignDepartmentUseCase,
	deleteUC agent.DeleteAgentUseCase,
	getUC agent.GetAgentUseCase,
	listUC agent.ListAgentsUseCase,
	simulateUC agent.SimulateTurnUseCase,
	toolRegistry tools.Service,
	aiService ai.Service,
) *AgentHandler {
	return &AgentHandler{
		createUseCase:           createUC,
		updateUseCase:           updateUC,
		assignDepartmentUseCase: assignDepartmentUC,
		deleteUseCase:           deleteUC,
		getUseCase:              getUC,
		listUseCase:             listUC,
		simulateUseCase:         simulateUC,
		toolRegistry:            toolRegistry,
		aiService:               aiService,
	}
}

type toolBindingRequest struct {
	Name        string                 `json:"name" example:"http_request"`
	Function    string                 `json:"function,omitempty" example:"consultar_pedido"`
	Description string                 `json:"description,omitempty" example:"Consulta o status de um pedido pelo número que o cliente informou."`
	Visibility  []string               `json:"visibility,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

type ragConfigRequest struct {
	MaxCharacters   *int     `json:"maxCharacters,omitempty"`
	MaxChunks       *int     `json:"maxChunks,omitempty"`
	MinScore        *float64 `json:"minScore,omitempty"`
	NumCandidates   *int     `json:"numCandidates,omitempty"`
	ContextWindow   *int     `json:"contextWindow,omitempty"`
	MaxChunksPerDoc *int     `json:"maxChunksPerDoc,omitempty"`
}

type agentVariableRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	DefaultValue string `json:"defaultValue,omitempty"`
}

type createAgentRequest struct {
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	MessagingPrompt  string                 `json:"messagingPrompt"`
	MessagingModel   string                 `json:"messagingModel"`
	AvatarURL        string                 `json:"avatarUrl"`
	Tags             []string               `json:"tags"`
	Metadata         map[string]string      `json:"metadata"`
	IsActive         *bool                  `json:"isActive"`
	Provider         string                 `json:"provider"`
	InternalTools    []toolBindingRequest   `json:"internalTools"`
	MediaIDs         []string               `json:"mediaIds"`
	KnowledgeBaseIDs []string               `json:"knowledgeBaseIds,omitempty"`
	MCPCollectionIDs json.RawMessage        `json:"mcpCollectionIds,omitempty" swaggerignore:"true"`
	DepartmentID     string                 `json:"departmentId,omitempty"`
	RAGEnabled       *bool                  `json:"ragEnabled,omitempty"`
	RAGConfig        *ragConfigRequest      `json:"ragConfig,omitempty"`
	Variables        []agentVariableRequest `json:"variables,omitempty"`
}

type updateAgentRequest struct {
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	MessagingPrompt  string                 `json:"messagingPrompt"`
	MessagingModel   string                 `json:"messagingModel"`
	AvatarURL        string                 `json:"avatarUrl"`
	Tags             []string               `json:"tags"`
	Metadata         map[string]string      `json:"metadata"`
	IsActive         *bool                  `json:"isActive"`
	Provider         string                 `json:"provider"`
	InternalTools    json.RawMessage        `json:"internalTools,omitempty" swaggerignore:"true"`
	MediaIDs         []string               `json:"mediaIds"`
	KnowledgeBaseIDs []string               `json:"knowledgeBaseIds,omitempty"`
	MCPCollectionIDs json.RawMessage        `json:"mcpCollectionIds,omitempty" swaggerignore:"true"`
	RAGEnabled       *bool                  `json:"ragEnabled,omitempty"`
	RAGConfig        *ragConfigRequest      `json:"ragConfig,omitempty"`
	Variables        []agentVariableRequest `json:"variables,omitempty"`
}

func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeInvalidBodyError(w)
		return
	}

	claims := middleware.GetClaims(r)
	if claims == nil {
		response.WriteError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	if req.Tags == nil {
		req.Tags = make([]string, 0)
	}

	provider, ok := parseProvider(req.Provider)
	if !ok {
		response.WriteValidationError(w, map[string]string{"provider": "invalid provider"})
		return
	}

	if strings.TrimSpace(req.MessagingModel) == "" {
		response.WriteValidationError(w, map[string]string{"messagingModel": "required"})
		return
	}
	if !isSupportedMessagingModel(provider, req.MessagingModel) {
		response.WriteValidationError(w, map[string]string{"messagingModel": "unsupported model for provider"})
		return
	}

	if req.InternalTools == nil {
		response.WriteValidationError(w, map[string]string{"internalTools": "required"})
		return
	}
	if carriesTools(req.MCPCollectionIDs) {
		response.WriteRefusal(w, agenthttp.RefusalMCPCollectionsMoved)
		return
	}

	r = withDepartmentCreationScope(r, req.DepartmentID)

	created, err := h.createUseCase.Execute(r.Context(), agent.CreateAgentInput{
		WorkspaceID:      middleware.GetWorkspaceID(r),
		Name:             req.Name,
		Description:      req.Description,
		MessagingPrompt:  req.MessagingPrompt,
		MessagingModel:   req.MessagingModel,
		AvatarURL:        req.AvatarURL,
		Tags:             req.Tags,
		Metadata:         req.Metadata,
		Provider:         provider,
		IsActive:         req.IsActive,
		RAGEnabled:       req.RAGEnabled,
		RAGConfig:        convertRAGConfig(req.RAGConfig),
		InternalTools:    convertToolBindings(req.InternalTools),
		MediaIDs:         req.MediaIDs,
		KnowledgeBaseIDs: req.KnowledgeBaseIDs,
		Variables:        convertVariables(req.Variables),
		RequestedBy:      httpx.RequesterOf(r),
	})
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	response.WriteSuccess(w, http.StatusCreated, h.concealed(created))
}

// @Summary		Atualizar um agente
// @Description	Salva os campos do rascunho do agente: nome, descrição, prompt, modelo, avatar, etiquetas, metadados, mídias, bases de conhecimento, RAG, variáveis e o estado ativo. Conversas seguem usando a versão publicada até a próxima publicação.
// @Description
// @Description	As ferramentas não vão neste corpo: cada uma é salva em /agents/{id}/tools. Um corpo com `internalTools` responde 400 `agent_tools_moved` sem salvar nada. Servidores MCP também são ferramentas (tipo `mcp_source`, com as ferramentas permitidas): um corpo com `mcpCollectionIds` responde 400 `agent_mcp_collections_moved`.
// @Tags			Agentes
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"ID do agente"
// @Param			request	body		updateAgentRequest	true	"Campos do agente"
// @Success		200		{object}	agent.Agent
// @Failure		400		{object}	response.ErrorResponse	"agent_tools_moved, agent_mcp_collections_moved, agent_invalid_body, agent_name_required"
// @Failure		403		{object}	response.ErrorResponse	"agent_forbidden"
// @Failure		404		{object}	response.ErrorResponse	"agent_not_found"
// @Failure		422		{object}	response.ErrorResponse	"agent_not_activatable"
// @Security		BearerAuth
// @Router			/agents/{id} [put]
func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var req updateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeInvalidBodyError(w)
		return
	}
	if carriesTools(req.InternalTools) {
		response.WriteRefusal(w, agenthttp.RefusalToolsMoved)
		return
	}
	if carriesTools(req.MCPCollectionIDs) {
		response.WriteRefusal(w, agenthttp.RefusalMCPCollectionsMoved)
		return
	}

	claims := middleware.GetClaims(r)
	if claims == nil {
		response.WriteError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	if !h.verifyOwnership(w, id, claims, r) {
		return
	}

	if req.IsActive == nil {
		response.WriteValidationError(w, map[string]string{"isActive": "required"})
		return
	}

	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	if req.Tags == nil {
		req.Tags = make([]string, 0)
	}

	provider, ok := parseProvider(req.Provider)
	if !ok {
		response.WriteValidationError(w, map[string]string{"provider": "invalid provider"})
		return
	}

	if strings.TrimSpace(req.MessagingModel) == "" {
		response.WriteValidationError(w, map[string]string{"messagingModel": "required"})
		return
	}
	if !isSupportedMessagingModel(provider, req.MessagingModel) {
		response.WriteValidationError(w, map[string]string{"messagingModel": "unsupported model for provider"})
		return
	}

	updated, err := h.updateUseCase.Execute(r.Context(), id, agent.UpdateAgentInput{
		Name:             &req.Name,
		Description:      &req.Description,
		MessagingPrompt:  &req.MessagingPrompt,
		MessagingModel:   &req.MessagingModel,
		Provider:         &provider,
		AvatarURL:        &req.AvatarURL,
		Tags:             req.Tags,
		Metadata:         req.Metadata,
		IsActive:         req.IsActive,
		RAGEnabled:       ptrBool(boolPtrOrDefault(req.RAGEnabled, false)),
		MediaIDs:         req.MediaIDs,
		KnowledgeBaseIDs: req.KnowledgeBaseIDs,
		Variables:        convertVariables(req.Variables),
		ReplaceRAGConfig: true,
		RAGConfig:        convertRAGConfig(req.RAGConfig),
		RequestedBy:      httpx.RequesterOf(r),
	})
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	response.WriteSuccess(w, http.StatusOK, h.concealed(updated))
}

func (h *AgentHandler) AssignDepartment(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := middleware.GetClaims(r)
	if claims == nil {
		response.WriteError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	if !requirePrivilegedDepartmentAssignment(w, r) {
		return
	}

	departmentID, ok := decodeDepartmentAssignmentRequest(w, r)
	if !ok {
		return
	}

	if !h.verifyOwnership(w, id, claims, r) {
		return
	}

	updated, err := h.assignDepartmentUseCase.Execute(withDepartmentCreationScope(r, departmentID).Context(), id)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	response.WriteSuccess(w, http.StatusOK, h.concealed(updated))
}

func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	claims := middleware.GetClaims(r)
	if claims == nil {
		response.WriteError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	if !h.verifyOwnership(w, id, claims, r) {
		return
	}

	if err := h.deleteUseCase.Execute(id); err != nil {
		h.handleDomainError(w, err)
		return
	}

	response.WriteSuccess(w, http.StatusNoContent, nil)
}

func (h *AgentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	claims := middleware.GetClaims(r)
	if claims == nil {
		response.WriteError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	agentItem, err := h.getUseCase.Execute(id)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	if agentItem.WorkspaceID != middleware.GetWorkspaceID(r) {
		response.WriteRefusal(w, agenthttp.RefusalForbidden)
		return
	}

	if !canAccessDepartment(r, agentItem.DepartmentID) {
		response.WriteRefusal(w, agenthttp.RefusalForbidden)
		return
	}

	response.WriteSuccess(w, http.StatusOK, h.concealed(agentItem))
}

func (h *AgentHandler) RequiredVariables(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	claims := middleware.GetClaims(r)
	if claims == nil {
		response.WriteError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	agentItem, err := h.getUseCase.Execute(id)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	if agentItem.WorkspaceID != middleware.GetWorkspaceID(r) {
		response.WriteRefusal(w, agenthttp.RefusalForbidden)
		return
	}

	if !canAccessDepartment(r, agentItem.DepartmentID) {
		response.WriteRefusal(w, agenthttp.RefusalForbidden)
		return
	}

	infos := agentItem.RequiredVariableInfos()
	response.WriteSuccess(w, http.StatusOK, infos)
}

func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	if claims == nil {
		response.WriteError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	values := r.URL.Query()

	options := shared.QueryOptions{
		Pagination: parsePagination(values),
		Sorts:      parseSort(values, map[string]string{"name": "name", "createdat": "createdAt", "updatedat": "updatedAt"}),
	}

	search := strings.TrimSpace(values.Get("search"))

	tagValues := append([]string{}, values["tag"]...)
	tagValues = append(tagValues, values["tags"]...)
	tags := splitListValues(tagValues)

	var isActive *bool
	if raw := strings.TrimSpace(values.Get("isActive")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			response.WriteValidationError(w, map[string]string{"isActive": "must be true or false"})
			return
		}
		isActive = &parsed
	}

	var createdAfter *time.Time
	if raw := strings.TrimSpace(values.Get("createdAfter")); raw != "" {
		parsed, err := parseTime(raw)
		if err != nil {
			response.WriteValidationError(w, map[string]string{"createdAfter": "must be a valid RFC3339 timestamp or YYYY-MM-DD"})
			return
		}
		createdAfter = &parsed
	}

	var createdBefore *time.Time
	if raw := strings.TrimSpace(values.Get("createdBefore")); raw != "" {
		parsed, err := parseTime(raw)
		if err != nil {
			response.WriteValidationError(w, map[string]string{"createdBefore": "must be a valid RFC3339 timestamp or YYYY-MM-DD"})
			return
		}
		createdBefore = &parsed
	}

	notArchived := false
	if shouldReturnEmptyDepartmentList(r) {
		response.WritePaginated(w, http.StatusOK, []*agent.AgentListItem{}, response.PaginationMeta{
			Page:       options.Pagination.Page,
			PageSize:   options.Pagination.PageSize,
			TotalPages: 0,
			TotalItems: 0,
		})
		return
	}
	result, err := h.listUseCase.Execute(agent.ListAgentsInput{
		WorkspaceID:   middleware.GetWorkspaceID(r),
		DepartmentIDs: departmentFilterIDs(r),
		Search:        search,
		Tags:          tags,
		IsActive:      isActive,
		Archived:      &notArchived,
		CreatedAfter:  createdAfter,
		CreatedBefore: createdBefore,
		Options:       options,
	})
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "Failed to list agents", nil)
		return
	}

	response.WritePaginated(w, http.StatusOK, result.Items, response.PaginationMeta{
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
		TotalItems: result.TotalItems,
	})
}

func (h *AgentHandler) ListArchived(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	if claims == nil {
		response.WriteError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	values := r.URL.Query()

	options := shared.QueryOptions{
		Pagination: parsePagination(values),
		Sorts:      parseSort(values, map[string]string{"name": "name", "createdat": "createdAt", "updatedat": "updatedAt"}),
	}

	isArchived := true
	if shouldReturnEmptyDepartmentList(r) {
		response.WritePaginated(w, http.StatusOK, []*agent.AgentListItem{}, response.PaginationMeta{
			Page:       options.Pagination.Page,
			PageSize:   options.Pagination.PageSize,
			TotalPages: 0,
			TotalItems: 0,
		})
		return
	}
	result, err := h.listUseCase.Execute(agent.ListAgentsInput{
		WorkspaceID:   middleware.GetWorkspaceID(r),
		DepartmentIDs: departmentFilterIDs(r),
		Search:        strings.TrimSpace(values.Get("search")),
		Archived:      &isArchived,
		Options:       options,
	})
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "Failed to list archived agents", nil)
		return
	}

	response.WritePaginated(w, http.StatusOK, result.Items, response.PaginationMeta{
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
		TotalItems: result.TotalItems,
	})
}

func (h *AgentHandler) Archive(w http.ResponseWriter, r *http.Request) {
	agentID := mux.Vars(r)["id"]
	if agentID == "" {
		response.WriteError(w, http.StatusBadRequest, "Agent ID is required", nil)
		return
	}

	existing, err := h.getUseCase.Execute(agentID)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	if existing.WorkspaceID != middleware.GetWorkspaceID(r) {
		response.WriteRefusal(w, agenthttp.RefusalForbidden)
		return
	}

	archived := true
	if _, err := h.updateUseCase.Execute(r.Context(), agentID, agent.UpdateAgentInput{Archived: &archived}); err != nil {
		response.WriteRefusal(w, agenthttp.RefusalInternal)
		return
	}

	response.WriteSuccess(w, http.StatusOK, map[string]interface{}{"archived": true})
}

func (h *AgentHandler) Unarchive(w http.ResponseWriter, r *http.Request) {
	agentID := mux.Vars(r)["id"]
	if agentID == "" {
		response.WriteError(w, http.StatusBadRequest, "Agent ID is required", nil)
		return
	}

	existing, err := h.getUseCase.Execute(agentID)
	if err != nil {
		h.handleDomainError(w, err)
		return
	}

	if existing.WorkspaceID != middleware.GetWorkspaceID(r) {
		response.WriteRefusal(w, agenthttp.RefusalForbidden)
		return
	}

	unarchived := false
	if _, err := h.updateUseCase.Execute(r.Context(), agentID, agent.UpdateAgentInput{Archived: &unarchived}); err != nil {
		response.WriteRefusal(w, agenthttp.RefusalInternal)
		return
	}

	response.WriteSuccess(w, http.StatusOK, map[string]interface{}{"archived": false})
}

func (h *AgentHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	if h.toolRegistry == nil {
		response.WriteSuccess(w, http.StatusOK, []tools.Definition{})
		return
	}
	definitions := h.toolRegistry.Definitions()

	claims := middleware.GetClaims(r)
	isAdmin := claims != nil && claims.Role == string(user.RoleAdmin)
	if !isAdmin {
		filtered := make([]tools.Definition, 0, len(definitions))
		for _, def := range definitions {
			if !def.AdminOnly {
				filtered = append(filtered, def)
			}
		}
		definitions = filtered
	}

	sort.SliceStable(definitions, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(definitions[i].Name))
		right := strings.ToLower(strings.TrimSpace(definitions[j].Name))
		return left < right
	})

	type camelParam struct {
		Type               string `json:"type"`
		Description        string `json:"description"`
		DisplayName        string `json:"displayName"`
		DisplayDescription string `json:"displayDescription"`
	}

	type camelConfigParam struct {
		Type               string `json:"type"`
		Description        string `json:"description"`
		DisplayName        string `json:"displayName"`
		DisplayDescription string `json:"displayDescription"`
		DefaultValue       any    `json:"defaultValue,omitempty"`
		Options            []struct {
			Value string `json:"value"`
			Label string `json:"label"`
		} `json:"options,omitempty"`
		OptionsSource string `json:"optionsSource,omitempty"`
		Required      bool   `json:"required"`
	}

	type camelTool struct {
		Name               string                      `json:"name"`
		DisplayName        string                      `json:"displayName"`
		Description        string                      `json:"description"`
		DisplayDescription string                      `json:"displayDescription"`
		Parameters         map[string]camelParam       `json:"parameters"`
		Required           []string                    `json:"required"`
		Visibility         []string                    `json:"visibility"`
		Category           string                      `json:"category"`
		ConfigSchema       map[string]camelConfigParam `json:"configSchema,omitempty"`
		RequiredConfig     []string                    `json:"requiredConfig,omitempty"`
		RequiresConfig     bool                        `json:"requiresConfig"`
		BindingKey         string                      `json:"bindingKey,omitempty"`
		Instances          string                      `json:"instances"`
		TestMode           agenttool.TestMode          `json:"testMode"`
	}

	camelDefs := make([]camelTool, len(definitions))
	for i, def := range definitions {
		params := make(map[string]camelParam, len(def.Parameters))
		for k, v := range def.Parameters {
			paramDisplayName := v.DisplayName
			if paramDisplayName == "" {
				paramDisplayName = k
			}
			paramDisplayDesc := v.DisplayDescription
			if paramDisplayDesc == "" {
				paramDisplayDesc = v.Description
			}
			params[toCamelCase(k)] = camelParam{
				Type:               v.Type,
				Description:        paramDisplayDesc,
				DisplayName:        paramDisplayName,
				DisplayDescription: paramDisplayDesc,
			}
		}
		configSchema := map[string]camelConfigParam{}
		if len(def.ConfigSchema) > 0 {
			for k, v := range def.ConfigSchema {
				displayName := v.DisplayName
				if displayName == "" {
					displayName = k
				}
				displayDescription := v.DisplayDescription
				if displayDescription == "" {
					displayDescription = v.Description
				}

				options := make([]struct {
					Value string `json:"value"`
					Label string `json:"label"`
				}, 0, len(v.Options))
				for _, option := range v.Options {
					options = append(options, struct {
						Value string `json:"value"`
						Label string `json:"label"`
					}{Value: option.Value, Label: option.Label})
				}

				configSchema[k] = camelConfigParam{
					Type:               v.Type,
					Description:        displayDescription,
					DisplayName:        displayName,
					DisplayDescription: displayDescription,
					DefaultValue:       v.Default,
					Options:            options,
					OptionsSource:      v.OptionsSource,
					Required:           v.Required,
				}
			}
		}
		visibility := make([]string, len(def.Visibility))
		for j, v := range def.Visibility {
			visibility[j] = string(v)
		}

		displayName := def.DisplayName
		if displayName == "" {
			displayName = def.Name
		}
		displayDesc := def.DisplayDescription
		if displayDesc == "" {
			displayDesc = def.Description
		}

		camelDefs[i] = camelTool{
			Name:               def.Name,
			DisplayName:        displayName,
			Description:        displayDesc,
			DisplayDescription: displayDesc,
			Parameters:         params,
			Required:           def.Required,
			Visibility:         visibility,
			Category:           string(def.Category),
			ConfigSchema:       configSchema,
			RequiredConfig:     def.RequiredConfig,
			RequiresConfig:     def.RequiresConfig,
			BindingKey:         def.Instances.Key,
			Instances:          string(def.Instances.Effective()),
			TestMode:           agenttool.TestModeIn(h.toolRegistry, def.Name),
		}
	}

	response.WriteSuccess(w, http.StatusOK, camelDefs)
}

func toCamelCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

type agentOptionsDefaults struct {
	MessagingModel string `json:"messagingModel"`
}

type agentOptionsResponse struct {
	Providers         []agentProviderOption          `json:"providers"`
	Models            map[string]agentProviderModels `json:"models"`
	Messaging         []string                       `json:"messaging"`
	ModelPricing      []ai.ModelInfo                 `json:"modelPricing"`
	ModelCapabilities []ai.ModelCapability           `json:"modelCapabilities"`
	Defaults          agentOptionsDefaults           `json:"defaults"`
}

// @Summary		Opções de modelos de IA
// @Description	Lista os modelos de conversa disponíveis, do mais popular para o menos popular, com preço por milhão de tokens, janela de contexto, capacidades e níveis de raciocínio. É a mesma lista dos seletores de modelo da Elo, do construtor de fluxos, dos nós de IA e dos agentes.
// @Description
// @Description	`modelCapabilities` é o vocabulário de capacidades, na ordem de exibição. Cada item de `modelPricing` traz em `capabilities` só o que o catálogo do provedor confirma; o que o catálogo não informa fica de fora, nunca é presumido.
// @Description
// @Description	| Capacidade | Quando o modelo tem |
// @Description	|---|---|
// @Description	| vision | aceita imagens na entrada |
// @Description	| tools | aceita ferramentas (a Elo e o construtor de fluxos precisam) |
// @Description	| reasoning | aceita o parâmetro de raciocínio |
// @Description	| long_context | janela de contexto de 1.000.000 de tokens ou mais |
// @Description	| image_output | gera imagens na saída |
// @Description	| audio_input | aceita áudio na entrada |
// @Description	| low_price | preço conhecido de até US$ 0,30 por milhão de tokens de entrada e US$ 1,50 de saída |
// @Description	| free | preço conhecido e zero na entrada e na saída |
// @Description
// @Description	`reasoning` aparece só quando o catálogo lista os níveis do modelo e o padrão dele: `levels` em ordem crescente (`none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`, só os que o modelo aceita; `none` some quando o raciocínio é obrigatório) e `default`. Sem `reasoning`, o modelo não oferece escolha de nível, e um nível enviado para ele é recusado.
// @Tags			Agentes
// @Produce		json
// @Success		200	{object}	agentOptionsResponse
// @Failure		401	{object}	response.ErrorResponse
// @Failure		403	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/agents/options [get]
func (h *AgentHandler) ListOptions(w http.ResponseWriter, r *http.Request) {
	modelsByProvider := map[agent.AgentProvider]agentProviderModels{}
	var messagingModels []string
	var modelsWithPricing []ai.ModelInfo

	if h.aiService != nil {
		if priced, err := h.aiService.GetModelsWithPricing(r.Context()); err == nil && len(priced) > 0 {
			modelsWithPricing = priced
			ids := make([]string, len(priced))
			for i, m := range priced {
				ids[i] = m.ID
			}
			modelsByProvider[agent.AgentProviderAI] = agentProviderModels{LLM: ids}
			messagingModels = ids
		}
	}

	payload := agentOptionsResponse{
		Providers:         getSupportedAgentProviders(),
		Models:            make(map[string]agentProviderModels, len(modelsByProvider)),
		Messaging:         messagingModels,
		ModelPricing:      modelsWithPricing,
		ModelCapabilities: ai.ModelCapabilities(),
		Defaults:          agentOptionsDefaults{MessagingModel: defaultMessagingModel},
	}

	for provider, models := range modelsByProvider {
		payload.Models[string(provider)] = models
	}

	response.WriteSuccess(w, http.StatusOK, payload)
}

func (h *AgentHandler) handleDomainError(w http.ResponseWriter, err error) {
	agenthttp.WriteError(w, err)
}

func (h *AgentHandler) writeInvalidBodyError(w http.ResponseWriter) {
	response.WriteInvalidBodyError(w, map[string]string{
		"name":            "string (required)",
		"description":     "string (optional)",
		"messagingPrompt": "string (required)",
		"language":        "string (required)",
		"messagingModel":  "string (required)",
		"avatarUrl":       "string (optional)",
		"tags":            "string array (optional)",
		"metadata":        "object map[string]string (optional)",
		"provider":        "string (required, supported: \"platform\")",
		"isActive":        "boolean (optional)",
		"internalTools":   "array of {name: string, visibility?: string[], config?: object} (required; use [] for none)",
		"departmentId":    "string (optional; required for members who belong to multiple departments)",
		"mediaIds":        "string array (optional; IDs of media linked to agent for send_whatsapp_media)",
	})
}

func parseTime(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02", value)
}

func boolPtrOrDefault(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func ptrBool(v bool) *bool {
	return &v
}

func parseProvider(value string) (agent.AgentProvider, bool) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return "", false
	}

	provider := agent.AgentProvider(trimmed)
	if !provider.IsValid() {
		return provider, false
	}

	return provider, true
}

func convertToolBindings(reqTools []toolBindingRequest) []agent.ToolBinding {
	if reqTools == nil {
		return nil
	}
	result := make([]agent.ToolBinding, 0, len(reqTools))
	for _, t := range reqTools {
		tb := agent.ToolBinding{Name: t.Name, Function: t.Function, Description: t.Description, Config: t.Config}
		if len(t.Visibility) > 0 {
			tb.Visibility = make([]agent.ToolVisibility, 0, len(t.Visibility))
			for _, v := range t.Visibility {
				tb.Visibility = append(tb.Visibility, agent.ToolVisibility(v))
			}
		}
		result = append(result, tb)
	}
	return result
}

func convertVariables(reqVars []agentVariableRequest) []agent.AgentVariable {
	if reqVars == nil {
		return nil
	}
	result := make([]agent.AgentVariable, 0, len(reqVars))
	for _, v := range reqVars {
		result = append(result, agent.AgentVariable{
			Name:         v.Name,
			Description:  v.Description,
			DefaultValue: v.DefaultValue,
		})
	}
	return result
}

func convertRAGConfig(req *ragConfigRequest) *agent.RAGConfig {
	if req == nil {
		return nil
	}
	cfg := &agent.RAGConfig{}
	if req.MaxCharacters != nil {
		cfg.MaxCharacters = *req.MaxCharacters
	}
	if req.MaxChunks != nil {
		cfg.MaxChunks = *req.MaxChunks
	}
	if req.MinScore != nil {
		cfg.MinScore = *req.MinScore
	}
	if req.NumCandidates != nil {
		cfg.NumCandidates = *req.NumCandidates
	}
	if req.ContextWindow != nil {
		cfg.ContextWindow = *req.ContextWindow
	}
	if req.MaxChunksPerDoc != nil {
		cfg.MaxChunksPerDoc = *req.MaxChunksPerDoc
	}
	return cfg
}

func (h *AgentHandler) verifyOwnership(w http.ResponseWriter, agentID string, claims *auth.Claims, r *http.Request) bool {
	agentItem, err := h.getUseCase.Execute(agentID)
	if err != nil {
		h.handleDomainError(w, err)
		return false
	}

	if agentItem.WorkspaceID != middleware.GetWorkspaceID(r) {
		response.WriteRefusal(w, agenthttp.RefusalForbidden)
		return false
	}

	return true
}

func carriesTools(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}
