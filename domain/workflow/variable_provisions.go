package workflow

import (
	"regexp"
	"strings"

	"vozko/domain/variabledecl"
)

const (
	labelSeparator       = " › "
	defaultAIResponseVar = "ai_response"
	defaultLoopItemVar   = "item"
	subWorkflowVarPrefix = "sub_"
)

var (
	codeStateSetCall    = regexp.MustCompile(`\bstate\.set\(`)
	codeStateSetLiteral = regexp.MustCompile(`\bstate\.set\(\s*(?:'([^'\\]+)'|"([^"\\]+)")`)
)

var triggerFieldTypes = map[TriggerInputFieldType]VariableType{
	TriggerFieldText:    VariableTypeText,
	TriggerFieldNumber:  VariableTypeNumber,
	TriggerFieldBoolean: VariableTypeBoolean,
	TriggerFieldObject:  VariableTypeObject,
	TriggerFieldList:    VariableTypeList,
}

func label(parts ...string) string {
	return strings.Join(parts, labelSeparator)
}

func examplePtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func everyEdge(string) bool { return true }

func onlyEdge(label string) func(string) bool {
	return func(l string) bool { return l == label }
}

func exceptEdges(labels ...string) func(string) bool {
	return func(l string) bool {
		for _, excluded := range labels {
			if l == excluded {
				return false
			}
		}
		return true
	}
}

type nodeProvisions struct {
	flow *variableFlow
	node int
	edge func(string) bool
}

func (np nodeProvisions) add(p provision) {
	p.node = np.node
	if np.edge != nil && p.edge == nil {
		p.edge = np.edge
	}
	np.flow.add(p)
}

func (np nodeProvisions) on(edge func(string) bool) nodeProvisions {
	np.edge = edge
	return np
}

func (np nodeProvisions) visible(ref string, match provisionMatch, text string, group VariableGroup, kind VariableKind, typ VariableType) {
	np.add(provision{ref: ref, match: match, label: text, group: group, kind: kind, typ: typ})
}

var keyedExamples = map[string]string{
	"contact_number":       "5511999990000",
	"conversation_message": "Oi, quero saber o valor",
	"conversation_channel": "whatsapp",
	"lead_name":            "Ana Souza",
	"call_caller_number":   "5511999990000",
	"call_called_number":   "551130000000",
	"birthday_date":        "28/10",
	"webhook_method":       "POST",
}

func (np nodeProvisions) keyed(ref string, match provisionMatch, key, text string, group VariableGroup, kind VariableKind, typ VariableType) {
	np.add(provision{ref: ref, match: match, label: text, labelKey: key, group: group, kind: kind, typ: typ, example: examplePtr(keyedExamples[key])})
}

var typeSamples = map[VariableType]string{
	VariableTypeText:    "texto",
	VariableTypeNumber:  "42",
	VariableTypeBoolean: "sim",
	VariableTypeDate:    "28/10/2026",
}

func sampleOf(typ VariableType) *string {
	return examplePtr(typeSamples[typ])
}

func (np nodeProvisions) hidden(ref string, match provisionMatch) {
	np.add(provision{ref: ref, match: match, hidden: true})
}

func (f *variableFlow) collectProvisions(sources VariableSources) {
	for i := range f.graph.Nodes {
		if !f.reached[i] {
			continue
		}
		n := &f.graph.Nodes[i]
		np := nodeProvisions{flow: f, node: i}
		if n.Type.IsTrigger() {
			spec, found := FindTriggerInputSpec(sources.Specs, n.ID)
			if carriesConversationData(n.Type) {
				f.conversationProvisions(np, sources.Declarations)
			}
			f.triggerProvisions(np, n, spec, found)
			continue
		}
		f.nodeProvisions(np, n, sources.Forms)
	}
}

func (f *variableFlow) triggerProvisions(np nodeProvisions, n *Node, spec TriggerInputSpec, found bool) {
	readable := found && spec.Readable()
	switch n.Type {
	case NodeTypeTriggerAgentCall:
		if !readable {
			np.hidden(DataKeyAgentInput+".", matchFamily)
		}
		for _, field := range spec.Fields {
			if field.Key == agentNameInputKey {
				continue
			}
			np.add(provision{
				ref: DataKeyAgentInput + "." + field.Key, match: fieldMatch(field), label: label("Enviado pelo agente", field.Key),
				group: VariableGroupTrigger, kind: VariableKindTrigger, typ: triggerFieldTypes[field.Type], example: examplePtr(field.Example),
			})
		}
		np.add(provision{ref: agentNameInputKey, match: matchExact, label: label("Agente que chamou", "nome"), group: VariableGroupTrigger, kind: VariableKindTrigger, typ: VariableTypeText, example: examplePtr(DefaultTestAgentName)})
		np.keyed(DataKeyAgent+".id", matchExact, "agent_id", label("Agente que chamou", "id"), VariableGroupTrigger, VariableKindTrigger, VariableTypeText)
	case NodeTypeTriggerWebhook:
		np.keyed(webhookBodyRefPrefix[:len(webhookBodyRefPrefix)-1], matchOpen, "webhook_body", "Corpo do webhook", VariableGroupTrigger, VariableKindTrigger, VariableTypeObject)
		for _, path := range spec.ReadKeys {
			np.visible(webhookBodyRefPrefix+path, matchExact, label(append([]string{"Corpo do webhook"}, strings.Split(path, ".")...)...), VariableGroupTrigger, VariableKindTrigger, VariableTypeUnknown)
		}
		np.keyed(DataKeyWebhook+".method", matchExact, "webhook_method", label("Webhook", "método"), VariableGroupTrigger, VariableKindTrigger, VariableTypeText)
		if TriggerNodeStartsOnLead(*n) {
			leadProvisions(np)
		}
	case NodeTypeTriggerFormSubmitted:
		ids, ok := FormTriggerFormIDs(n.Config)
		if !readable || !ok || len(ids) == 0 {
			np.hidden(DataKeyFormAnswerPrefix, matchFamily)
		}
		for _, field := range spec.Fields {
			if answer, isAnswer := strings.CutPrefix(field.Key, DataKeyFormAnswerPrefix); isAnswer {
				np.visible(field.Key, matchOpen, label("Resposta do formulário", firstNonEmpty(field.Description, answer)), VariableGroupTrigger, VariableKindTrigger, VariableTypeText)
			}
		}
		formIdentity(np, VariableGroupTrigger, VariableKindTrigger)
	case NodeTypeTriggerStageAdded:
		np.keyed(DataKeyStageID, matchExact, "stage_id", label("Etapa em que entrou", "id"), VariableGroupTrigger, VariableKindTrigger, VariableTypeText)
		np.hidden(DataKeyCampaignVars, matchOpen)
	case NodeTypeTriggerBirthday:
		np.keyed(DataKeyBirthday, matchExact, "birthday_date", label("Aniversário", "data"), VariableGroupTrigger, VariableKindTrigger, VariableTypeDate)
		leadProvisions(np)
	case NodeTypeTriggerMessageReceived, NodeTypeTriggerFirstMessage:
		messageEventProvisions(np, "conversation_message", label("Conversa", "mensagem recebida"), VariableGroupContact, VariableKindSystem, false)
	case NodeTypeTriggerCallReceived:
		np.keyed("caller_number", matchExact, "call_caller_number", label("Ligação", "número de quem ligou"), VariableGroupContact, VariableKindSystem, VariableTypeText)
		np.keyed("called_number", matchExact, "call_called_number", label("Ligação", "número discado"), VariableGroupContact, VariableKindSystem, VariableTypeText)
		np.hidden("call_id", matchExact)
	default:
		np.hidden("", matchAnyVariable)
	}
}

func carriesConversationData(t NodeType) bool {
	switch t {
	case NodeTypeTriggerMessageReceived, NodeTypeTriggerFirstMessage, NodeTypeTriggerStageAdded:
		return true
	}
	return false
}

var declaredVariableTypes = map[variabledecl.Kind]VariableType{
	variabledecl.KindText:   VariableTypeText,
	variabledecl.KindNumber: VariableTypeNumber,
	variabledecl.KindDate:   VariableTypeDate,
	variabledecl.KindChoice: VariableTypeText,
}

func declaredVariableType(t variabledecl.Type) VariableType {
	if typ, ok := declaredVariableTypes[t.Kind]; ok {
		return typ
	}
	return VariableTypeText
}

func (f *variableFlow) conversationProvisions(np nodeProvisions, declarations []variabledecl.Declaration) {
	catalog := variabledecl.NewCatalog(declarations)
	conversationVar := func(p provision) {
		p.match, p.group, p.kind = matchExact, VariableGroupConversation, VariableKindConversation
		np.add(p)
	}
	for _, name := range variabledecl.SystemKeys() {
		label, _ := variabledecl.SystemLabel(name)
		np.add(provision{match: matchExact, group: VariableGroupContact, kind: VariableKindSystem, ref: ConversationRef(name), label: label, labelKey: variabledecl.SystemLabelKey(name), typ: VariableTypeText, example: examplePtr(variabledecl.SystemExample(name))})
	}
	for _, d := range catalog.All() {
		conversationVar(provision{ref: ConversationRef(d.Name), label: d.Label, typ: declaredVariableType(d.Type), example: examplePtr(d.Example)})
	}
	for _, name := range ExtractCampaignVars(*f.graph) {
		if _, declared := catalog.Find(name); declared || variabledecl.IsSystemKey(name) {
			continue
		}
		conversationVar(provision{ref: ConversationRef(name), label: catalog.LabelOf(name), typ: VariableTypeText, undeclared: true})
	}
}

func fieldMatch(field TriggerInputField) provisionMatch {
	if field.Type == TriggerFieldObject || field.Type == TriggerFieldList {
		return matchOpen
	}
	return matchExact
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func leadProvisions(np nodeProvisions) {
	np.keyed(DataKeyLeadID, matchExact, "lead_id", label("Lead", "id"), VariableGroupContact, VariableKindSystem, VariableTypeText)
	np.keyed(DataKeyLeadName, matchExact, "lead_name", label("Lead", "nome"), VariableGroupContact, VariableKindSystem, VariableTypeText)
}

func formIdentity(np nodeProvisions, group VariableGroup, kind VariableKind) {
	np.keyed(DataKeyFormID, matchExact, "form_id", label("Formulário", "id"), group, kind, VariableTypeText)
	np.keyed(DataKeyFormResponseID, matchExact, "form_response_id", label("Formulário", "id da resposta"), group, kind, VariableTypeText)
}

func messageEventProvisions(np nodeProvisions, messageKey, messageLabel string, group VariableGroup, kind VariableKind, showChoice bool) {
	np.keyed(DataKeyMessage, matchExact, messageKey, messageLabel, group, kind, VariableTypeText)
	np.keyed(DataKeyContactNumber, matchExact, "contact_number", label("Contato", "número"), VariableGroupContact, VariableKindSystem, VariableTypeText)
	np.keyed(VarChannel, matchExact, "conversation_channel", label("Conversa", "canal"), VariableGroupContact, VariableKindSystem, VariableTypeText)
	if showChoice {
		np.visible(DataKeySelectedOptionID, matchExact, label("Opção escolhida", "id"), group, kind, VariableTypeText)
		np.visible(DataKeySelectedOptionTitle, matchExact, label("Opção escolhida", "título"), group, kind, VariableTypeText)
	} else {
		np.hidden(DataKeySelectedOptionID, matchExact)
		np.hidden(DataKeySelectedOptionTitle, matchExact)
	}
	for _, key := range []string{DataKeyMessageType, DataKeyMediaType, DataKeySelectedOptionKind, DataKeyTriggerMessageID, DataKeyWorkspaceID, "campaign_id", "campaign_workflow_id", "account_workflow_id"} {
		np.hidden(key, matchExact)
	}
	np.hidden(DataKeyCampaignVars, matchOpen)
}

func (f *variableFlow) nodeProvisions(np nodeProvisions, n *Node, forms map[string]TriggerForm) {
	def, known := f.defs[n.Type]
	if !known {
		np.hidden("", matchAnyVariable)
		np.hidden("last.", matchFamily)
		np.hidden("node."+n.ID+".", matchFamily)
	}
	name := f.nodeName(np.node)
	toolParamProvisions(np, n, name)
	for _, out := range declaredOutputKeysOf(n, def) {
		if out.Key == "" {
			np.hidden("node."+n.ID+".", matchFamily)
			np.hidden("last.", matchFamily)
			continue
		}
		np.hidden("last."+out.Key, matchOpen)
		if out.RepeatsConfig {
			np.hidden("node."+n.ID+"."+out.Key, matchOpen)
			continue
		}
		typ := offeredOutputType(out)
		example := examplePtr(out.Example)
		if example == nil {
			example = sampleOf(typ)
		}
		np.add(provision{ref: "node." + n.ID + "." + out.Key, match: matchOpen, label: label(name, firstNonEmpty(out.Label, out.Key)), labelKey: OutputLabelKey(out), labelPrefix: name, group: VariableGroupNodes, kind: VariableKindNode, typ: typ, example: example})
	}
	stateVar := func(ref, text string, typ VariableType) {
		if ref = canonicalRef(ref); ref != "" && !strings.Contains(ref, "{{") {
			np.visible(ref, matchOpen, text, VariableGroupNodes, VariableKindNode, typ)
		}
	}
	switch n.Type {
	case NodeTypeActionSetVariable:
		variable := configText(n, "variable")
		stateVar(variable, firstNonEmpty(configText(n, "variable_label"), label("Variável", variable)), VariableTypeUnknown)
	case NodeTypeActionHTTPRequest:
		capture := configText(n, "capture_variable")
		stateVar(capture, firstNonEmpty(configText(n, "capture_label"), label("Resposta HTTP", capture)), VariableTypeObject)
	case NodeTypeActionCode:
		codeProvisions(np, n, stateVar)
	case NodeTypeActionAIAgent:
		response := firstNonEmpty(configText(n, "response_variable"), defaultAIResponseVar)
		stateVar(response, label("Resposta da IA", response), VariableTypeObject)
		np.hidden("ai.", matchFamily)
	case NodeTypeActionGetCurrentTime:
		if variable := configText(n, "variable"); variable != "" {
			stateVar(variable, label("Data e hora", variable), VariableTypeObject)
			for _, suffix := range []string{"_datetime", "_hour", "_weekday"} {
				stateVar(variable+suffix, label("Data e hora", variable+suffix), VariableTypeText)
			}
		}
	case NodeTypeActionLoop:
		item := firstNonEmpty(configText(n, "item_variable"), defaultLoopItemVar)
		stateVar(item, label("Item da repetição", item), VariableTypeUnknown)
		stateVar(item+"_index", label("Item da repetição", item+"_index"), VariableTypeNumber)
	case NodeTypeActionRunWorkflow:
		np.hidden(subWorkflowVarPrefix, matchFamily)
	case NodeTypeWaitForReply:
		messageEventProvisions(np.on(onlyEdge("replied")), "", label(name, "resposta"), VariableGroupNodes, VariableKindNode, false)
	case NodeTypeActionSendInteractive:
		messageEventProvisions(np.on(exceptEdges(InteractiveHandleNoReply, InteractiveHandleSendFailed)), "", label(name, "resposta"), VariableGroupNodes, VariableKindNode, true)
	case NodeTypeActionSendForm:
		formAnswerProvisions(np.on(onlyEdge(FormHandleCompleted)), n, name, forms)
	case NodeTypeActionExportMessagesSheet:
		sheetExportProvisions(np, n)
	}
}

func formAnswerProvisions(np nodeProvisions, n *Node, name string, forms map[string]TriggerForm) {
	formIdentity(np, VariableGroupNodes, VariableKindNode)
	form, found := forms[strings.TrimSpace(configText(n, FormNodeFormIDKey))]
	if !found {
		np.hidden(DataKeyFormAnswerPrefix, matchFamily)
		return
	}
	for _, q := range form.Questions {
		np.visible(FormAnswerDataKey(q.Key), matchOpen, label("Resposta do formulário", firstNonEmpty(q.Prompt, q.Key)), VariableGroupNodes, VariableKindNode, VariableTypeText)
	}
}

func codeProvisions(np nodeProvisions, n *Node, stateVar func(ref, text string, typ VariableType)) {
	if output := configText(n, "output_variable"); output != "" {
		stateVar(output, label("Retorno do código", output), VariableTypeUnknown)
	}
	code := configText(n, "code")
	literals := codeStateSetLiteral.FindAllStringSubmatch(code, -1)
	for _, m := range literals {
		key := m[1] + m[2]
		stateVar(key, label("Variável", key), VariableTypeUnknown)
	}
	if len(codeStateSetCall.FindAllStringIndex(code, -1)) > len(literals) {
		np.hidden("", matchAnyVariable)
	}
}

func agentToolsOf(n *Node) []CustomTool {
	if n.Type != NodeTypeActionAIAgent {
		return nil
	}
	return CustomToolsOf(n.Config)
}

func toolParamProvisions(np nodeProvisions, n *Node, name string) {
	for _, tool := range agentToolsOf(n) {
		onTool := np.on(onlyEdge(tool.Name))
		for _, param := range tool.Parameters {
			typ := param.VariableType()
			example := sampleOf(typ)
			if len(param.Enum) > 0 {
				example = examplePtr(param.Enum[0])
			}
			onTool.add(provision{ref: "node." + n.ID + "." + param.Name, match: matchOpen, label: label(name, "Ferramenta "+tool.Name, param.Name), labelPrefix: name, group: VariableGroupNodes, kind: VariableKindNode, typ: typ, example: example})
		}
	}
}

func outputKeysOf(n *Node, def NodeDefinition) []OutputKeyDefinition {
	var keys []OutputKeyDefinition
	for _, tool := range agentToolsOf(n) {
		for _, param := range tool.Parameters {
			keys = append(keys, OutputKeyDefinition{Key: param.Name, Label: param.Name, Type: param.VariableType()})
		}
	}
	return append(keys, declaredOutputKeysOf(n, def)...)
}

func declaredOutputKeysOf(n *Node, def NodeDefinition) []OutputKeyDefinition {
	var keys []OutputKeyDefinition
	for _, ok := range def.OutputKeys {
		switch {
		case n.Type == NodeTypeActionReplyToAgent && strings.Contains(ok.Key, "<"):
			for _, field := range ReplyToAgentFieldsOf(n.Config) {
				keys = append(keys, OutputKeyDefinition{Key: field.Key, Label: field.Key, Type: VariableTypeText})
			}
		case IsDynamicOutputKey(ok.Key):
			keys = append(keys, OutputKeyDefinition{})
		default:
			keys = append(keys, ok)
		}
	}
	return keys
}

func IsDynamicOutputKey(key string) bool {
	return strings.ContainsAny(key, "*<")
}

func offeredOutputType(out OutputKeyDefinition) VariableType {
	if out.Type == "" {
		return VariableTypeUnknown
	}
	return out.Type
}
