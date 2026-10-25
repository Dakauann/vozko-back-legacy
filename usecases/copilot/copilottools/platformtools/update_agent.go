package platformtools

import (
	"context"

	"vozko/domain/agent"
	"vozko/domain/copilot"
	"vozko/domain/tools"
	"vozko/domain/workspace"
	"vozko/usecases/copilot/copilottools"
)

type updateAgentTool struct {
	get    agent.GetAgentUseCase
	update agent.UpdateAgentUseCase
	card   AgentDeps
}

func NewUpdateAgentTool(get agent.GetAgentUseCase, update agent.UpdateAgentUseCase, card AgentDeps) copilot.Tool {
	return &updateAgentTool{get: get, update: update, card: card}
}

func (t *updateAgentTool) Meta() copilot.Meta {
	return copilot.Meta{Mutating: true, Resource: workspace.ResourceAgents, Action: workspace.ActionUpdate}
}

func (t *updateAgentTool) Definition() tools.Definition {
	params, _ := scalarParams()
	params["id"] = tools.Parameter{Type: "string", Description: "id do agente a atualizar"}

	params["addTools"] = toolListParam(agentFieldDescriptions["addTools"])
	params["removeTools"] = stringListParam(agentFieldDescriptions["removeTools"])
	params["addKnowledgeBaseIds"] = stringListParam(agentFieldDescriptions["addKnowledgeBaseIds"])
	params["removeKnowledgeBaseIds"] = stringListParam(agentFieldDescriptions["removeKnowledgeBaseIds"])
	params["publish"] = publishParam()

	return tools.Definition{
		Name: "update_agent",
		Description: "Atualiza um agente existente. Apenas os campos informados são alterados; os demais permanecem como estão. " +
			"Ferramentas (servidores MCP inclusive, como ferramentas do tipo mcp_source) e bases de conhecimento são gerenciadas de forma incremental (addTools/removeTools etc.): " +
			"as atuais são preservadas automaticamente, não é preciso reenviá-las.",
		Parameters: params,
		Required:   []string{"id"},
	}
}

func (t *updateAgentTool) Execute(ctx context.Context, cc copilot.Context, args map[string]interface{}) copilot.Result {
	a, err := ownedAgent(t.get, cc, copilottools.ArgString(args, "id"))
	if err != nil {
		return copilot.Result{Status: copilot.StatusDenied, Message: "agente não encontrado neste workspace"}
	}
	id := a.ID

	var fields agentFields
	copilottools.BindArgs(args, &fields)
	in := fields.toUpdateInput()

	in.InternalTools = mergeToolBindings(
		t.card.Catalog,
		a.InternalTools,
		t.card.secrets().reveal(argToolBindings(args, "addTools"), args),
		copilottools.ArgStringList(args, "removeTools"),
	)
	in.KnowledgeBaseIDs = mergeStrings(
		a.KnowledgeBaseIDs,
		copilottools.ArgStringList(args, "addKnowledgeBaseIds"),
		copilottools.ArgStringList(args, "removeKnowledgeBaseIds"),
	)

	in.RequestedBy = copilottools.RequesterOf(cc)
	out, err := t.update.Execute(ctx, id, in)
	if res, refused := copilottools.ConsentGrantRefusal(err); refused {
		return res
	}
	if message, blocked := blockedMessage(err); blocked {
		return copilot.Result{Status: copilot.StatusError, Message: message}
	}
	if err != nil {
		return copilot.Result{Status: copilot.StatusError, Message: err.Error()}
	}
	return t.card.publishAfterSave(ctx, cc, args, out)
}

func (t *updateAgentTool) Validate(ctx context.Context, cc copilot.Context, args map[string]interface{}) error {
	a, err := ownedAgent(t.get, cc, copilottools.ArgString(args, "id"))
	if err != nil {
		return err
	}
	if err := t.card.secrets().validate(argToolBindings(args, "addTools")); err != nil {
		return err
	}
	if err := t.card.requireLinks(ctx, cc.WorkspaceID, copilottools.ArgStringList(args, "addKnowledgeBaseIds")); err != nil {
		return err
	}
	draft := mergeToolBindings(t.card.Catalog, a.InternalTools, argToolBindings(args, "addTools"), copilottools.ArgStringList(args, "removeTools"))
	return t.card.refuseBrokenActiveAgent(ctx, a, draft)
}

func (t *updateAgentTool) Secrets(args map[string]interface{}) []copilot.SecretField {
	return t.card.secrets().fields(argToolBindings(args, "addTools"))
}

func (t *updateAgentTool) Conceal(args map[string]interface{}) map[string]interface{} {
	return t.card.secrets().conceal(args, "addTools")
}

func (t *updateAgentTool) Describe(ctx context.Context, cc copilot.Context, args map[string]interface{}) []copilot.Field {
	var fields agentFields
	copilottools.BindArgs(args, &fields)
	card := []copilot.Field{describeAgent(t.get, cc, copilottools.ArgString(args, "id"))}
	card = append(card, agentFieldCard(fields)...)
	card = append(card, toolBindingField("addTools", argToolBindings(args, "addTools"))...)
	card = append(card, namesField("removeTools", copilottools.ArgStringList(args, "removeTools"))...)
	card = append(card, t.card.knowledgeBaseField(ctx, cc, "addKnowledgeBases", copilottools.ArgStringList(args, "addKnowledgeBaseIds"))...)
	card = append(card, t.card.knowledgeBaseField(ctx, cc, "removeKnowledgeBases", copilottools.ArgStringList(args, "removeKnowledgeBaseIds"))...)
	return append(card, publishField(args)...)
}
