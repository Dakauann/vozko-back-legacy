package workflow

type TriggerScope string

const (
	TriggerScopeConversation TriggerScope = "conversation"
	TriggerScopeGlobal       TriggerScope = "global"
)

var triggerScopes = map[TriggerType]TriggerScope{
	TriggerMessageReceived: TriggerScopeConversation,
	TriggerFirstMessage:    TriggerScopeConversation,
	TriggerStageAdded:      TriggerScopeConversation,
	TriggerWebhook:         TriggerScopeGlobal,
	TriggerFormSubmitted:   TriggerScopeGlobal,
	TriggerAgentCall:       TriggerScopeGlobal,
	TriggerBirthday:        TriggerScopeGlobal,
	TriggerCallReceived:    TriggerScopeGlobal,
}

func (t TriggerType) Scope() (TriggerScope, bool) {
	scope, known := triggerScopes[t]
	return scope, known
}

func (t TriggerType) FollowsResponsibleWorkflow() bool {
	scope, known := t.Scope()
	return known && scope == TriggerScopeConversation
}

func definitionTriggerScope(def NodeDefinition) TriggerScope {
	if !def.Type.IsTrigger() {
		return ""
	}
	scope, _ := TriggerType(def.Type).Scope()
	return scope
}

type ConversationResponsibility interface {
	ResponsibleWorkflow(workspaceID, entryID, entryType string) (string, bool, error)
}

func (s TriggerScope) Guide() string {
	switch s {
	case TriggerScopeConversation:
		return "desta conversa: dispara só nas conversas pelas quais este fluxo responde (números, contas e campanhas que usam este fluxo, ou conversas delegadas a ele)"
	case TriggerScopeGlobal:
		return "global: dispara para toda a área de trabalho conforme a configuração do próprio gatilho, sem depender de qual fluxo responde pela conversa"
	}
	return ""
}
