package agenttool

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"vozko/domain/agent"
	"vozko/domain/lint"
	"vozko/domain/tools"
)

const httpToolType = "http_request"

var (
	pathPlaceholderPattern = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)
	sentenceEndPattern     = regexp.MustCompile(`[.!?](\s|$)`)
	wordPattern            = regexp.MustCompile(`[\p{L}\p{N}]+`)
	credentialPrefixes     = regexp.MustCompile(`^(?i:(bearer|basic)\s+\S{8,})$|^(sk|rk|pk)_(live|test)_[A-Za-z0-9]{8,}$|^gh[pousr]_[A-Za-z0-9]{20,}$|^github_pat_[A-Za-z0-9_]{20,}$|^xox[abprs]-[A-Za-z0-9-]{10,}$|^AKIA[0-9A-Z]{16}$|^AIza[0-9A-Za-z_-]{30,}$|^eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)
	opaqueTokenPattern     = regexp.MustCompile(`^[A-Za-z0-9_\-+/=]{32,}$`)
	toolNameMentionPattern = regexp.MustCompile(`\b[a-z][a-z0-9]*(?:_[a-z0-9]+)+\b`)
	letterPattern          = regexp.MustCompile(`[A-Za-z]`)
	digitPattern           = regexp.MustCompile(`[0-9]`)
)

type Subject struct {
	Prompt          string
	Tools           []agent.ToolBinding
	Stored          []agent.ToolBinding
	Definitions     map[string]tools.Definition
	OfferedNames    []string
	DefinitionChars int
}

func Check(s Subject) Report {
	var r Report
	stored := make(map[string]agent.ToolBinding, len(s.Stored))
	for _, b := range s.Stored {
		stored[b.ID] = b
	}
	for _, b := range s.Tools {
		def, ok := s.Definitions[strings.ToLower(b.Name)]
		if !ok {
			r.add(toolIssue(b, CodeTypeUnknown, lint.SeverityBlocking, fmt.Sprintf("A ferramenta %q usa um tipo que não existe mais (%s)", b.Function, b.Name), "Remova a ferramenta ou troque por um tipo do catálogo."))
			continue
		}
		checkName(&r, b)
		checkDescription(&r, def, b)
		if err := def.ValidateConfig(b.Config); err != nil {
			r.add(toolIssue(b, CodeConfigInvalid, lint.SeverityBlocking, fmt.Sprintf("A configuração de %q é inválida: %v", b.Function, err), "Revise os campos obrigatórios da ferramenta."))
		}
		checkCredentials(&r, def, b)
		if strings.EqualFold(def.Name, httpToolType) {
			previous, untouched := stored[b.ID]
			checkHTTP(&r, b, untouched && ValueAt(previous.Config, URLParam, "") == ValueAt(b.Config, URLParam, ""))
		}
	}
	checkUniqueNames(&r, s)
	checkOverlaps(&r, s.Tools)
	checkBudgets(&r, s)
	checkPromptMentions(&r, s)
	return r
}

func toolIssue(b agent.ToolBinding, code string, severity lint.Severity, message, hint string) Issue {
	return Issue{Code: code, Severity: severity, ToolID: b.ID, Message: message, Hint: hint}
}

func checkName(r *Report, b agent.ToolBinding) {
	switch {
	case !tools.ValidToolName(b.Function):
		r.add(toolIssue(b, CodeNameInvalid, lint.SeverityBlocking, fmt.Sprintf("O nome da função %q não é aceito pelos modelos", b.Function), "Use letras minúsculas, dígitos e _, começando por letra, com até 64 caracteres."))
	case !ValidName(b.Function):
		r.add(toolIssue(b, CodeNameUnreadable, lint.SeverityAdvisory, fmt.Sprintf("O nome da função %q é difícil de ler para a IA", b.Function), "Renomeie com palavras que digam o que a ferramenta faz, por exemplo consultar_pedido."))
	}
}

func checkDescription(r *Report, def tools.Definition, b agent.ToolBinding) {
	description := strings.TrimSpace(b.Description)
	if description == "" {
		if def.Instances.Effective() == tools.InstancesMany {
			r.add(toolIssue(b, CodeDescriptionMissing, lint.SeverityAdvisory, fmt.Sprintf("A ferramenta %q não tem descrição própria; a IA lê só a descrição genérica do tipo", b.Function), "Escreva quando usar, quando não usar e o que ela retorna."))
		}
		return
	}
	if len(sentenceEndPattern.FindAllString(description, -1)) < thinDescriptionSentences && len([]rune(description)) < thinDescriptionChars {
		r.add(toolIssue(b, CodeDescriptionThin, lint.SeverityAdvisory, fmt.Sprintf("A descrição de %q é curta para a IA escolher bem", b.Function), "Use três ou quatro frases: quando usar, quando não usar e o que retorna."))
	}
}

func checkCredentials(r *Report, def tools.Definition, b agent.ToolBinding) {
	flagged := map[string]bool{}
	flag := func(field string) {
		if flagged[field] {
			return
		}
		flagged[field] = true
		issue := toolIssue(b, CodeSecretInPlainText, lint.SeverityBlocking, fmt.Sprintf("A ferramenta %q guarda uma credencial em texto em %s", b.Function, field), "Guarde o valor nos segredos do workspace e use {{secret.<nome>}} no lugar.")
		issue.Field = field
		r.add(issue)
	}
	for _, c := range PlainCredentials(def.SensitiveConfig(), b.Config) {
		flag(c.Field())
	}
	for _, param := range def.SensitiveConfig() {
		entries, _ := b.Config[param].(map[string]interface{})
		for _, key := range sortedKeys(entries) {
			if s, ok := entries[key].(string); ok && !IsSealed(s) && LooksLikeCredential(s) {
				flag(key)
			}
		}
	}
	if raw, ok := b.Config[URLParam].(string); ok {
		for _, pair := range queryPairs(raw) {
			if !IsSealed(pair.value) && LooksLikeCredential(pair.value) {
				flag(pair.key)
			}
		}
	}
	for _, f := range fieldMaps(b.Config) {
		name, _ := f["name"].(string)
		value, _ := f["value"].(string)
		if f["source"] != SourceModel && !IsSealed(value) && LooksLikeCredential(value) {
			flag(name)
		}
	}
}

func LooksLikeCredential(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "{{") {
		return false
	}
	if credentialPrefixes.MatchString(value) {
		return true
	}
	return opaqueTokenPattern.MatchString(value) && letterPattern.MatchString(value) && digitPattern.MatchString(value)
}

func checkHTTP(r *Report, b agent.ToolBinding, untouchedURL bool) {
	raw := strings.TrimSpace(ValueAt(b.Config, URLParam, ""))
	parsed, err := url.Parse(raw)
	hostPart := hostOf(raw)
	switch {
	case raw == "" || err != nil || hostPart == "":
		r.add(toolIssue(b, CodeHTTPURLInvalid, lint.SeverityBlocking, fmt.Sprintf("A URL de %q não é válida", b.Function), "Use uma URL completa, por exemplo https://api.exemplo.com/pedidos/{id}."))
		return
	case strings.ContainsAny(hostPart, "{}"):
		r.add(toolIssue(b, CodeHTTPURLInvalid, lint.SeverityBlocking, fmt.Sprintf("O endereço do servidor de %q não pode ter campos variáveis", b.Function), "Deixe o servidor fixo; só o caminho, a consulta e o corpo recebem campos."))
		return
	case parsed.User != nil:
		r.add(toolIssue(b, CodeHTTPURLInvalid, lint.SeverityBlocking, fmt.Sprintf("A URL de %q tem usuário e senha embutidos", b.Function), "Tire o usuário e a senha da URL e use a autenticação com um segredo do workspace."))
		return
	case parsed.Scheme == "http":
		severity := lint.SeverityBlocking
		if untouchedURL {
			severity = lint.SeverityAdvisory
		}
		r.add(toolIssue(b, CodeHTTPURLNotHTTPS, severity, fmt.Sprintf("A URL de %q usa http, sem criptografia", b.Function), "Troque para https. Uma URL http salva antes continua funcionando até ser editada."))
	case parsed.Scheme != "https":
		r.add(toolIssue(b, CodeHTTPURLInvalid, lint.SeverityBlocking, fmt.Sprintf("A URL de %q precisa começar com https://", b.Function), "Use https://."))
		return
	}
	checkHTTPFields(r, b, raw)
}

func hostOf(raw string) string {
	_, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return ""
	}
	end := strings.IndexAny(rest, "/?#")
	if end >= 0 {
		rest = rest[:end]
	}
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		rest = rest[at+1:]
	}
	return rest
}

func checkHTTPFields(r *Report, b agent.ToolBinding, raw string) {
	config, err := ParseHTTPConfig(b.Config)
	if err != nil {
		r.add(toolIssue(b, CodeConfigInvalid, lint.SeverityBlocking, fmt.Sprintf("A configuração de %q não pode ser lida", b.Function), "Abra a ferramenta e salve os campos de novo."))
		return
	}
	add := func(code string, severity lint.Severity, field, message, hint string) {
		issue := toolIssue(b, code, severity, message, hint)
		issue.Field = field
		r.add(issue)
	}
	args := map[string]bool{}
	wire := map[string]bool{}
	pathFields := map[string]bool{}
	for _, f := range config.Fields {
		if problem := fieldShapeProblem(f); problem != "" {
			add(CodeConfigInvalid, lint.SeverityBlocking, f.Name, fmt.Sprintf("O campo %q de %q %s", f.Name, b.Function, problem), "Corrija o campo na lista de campos da ferramenta.")
			continue
		}
		key := f.Location + "\x00" + strings.ToLower(f.Name)
		if wire[key] || (f.Source == SourceModel && args[f.ArgName()]) {
			add(CodeHTTPFieldDuplicate, lint.SeverityBlocking, f.Name, fmt.Sprintf("O campo %q aparece duas vezes em %q", f.Name, b.Function), "Cada campo precisa de um nome único.")
		}
		wire[key] = true
		if f.Source == SourceModel {
			args[f.ArgName()] = true
		}
		if f.Location == LocationPath {
			pathFields[f.Name] = true
		}
		if f.Source == SourceModel && strings.TrimSpace(f.Description) == "" {
			add(CodeHTTPFieldUndescribed, lint.SeverityAdvisory, f.Name, fmt.Sprintf("O campo %q de %q não diz à IA o que preencher", f.Name, b.Function), "Descreva o campo, com um exemplo de valor.")
		}
		if f.Source != SourceModel {
			checkTemplateScopes(add, f.Name, f.Value, false, b.Function)
		}
	}
	base, _, _ := strings.Cut(raw, "?")
	inURL := map[string]bool{}
	for _, m := range pathPlaceholderPattern.FindAllStringSubmatch(templatePattern.ReplaceAllString(base, ""), -1) {
		inURL[m[1]] = true
		if !pathFields[m[1]] {
			add(CodeHTTPFieldUnbound, lint.SeverityBlocking, m[1], fmt.Sprintf("A URL de %q usa {%s}, mas nenhum campo preenche esse valor", b.Function, m[1]), "Adicione o campo ao caminho ou tire o marcador da URL.")
		}
	}
	for _, name := range sortedSet(pathFields) {
		if !inURL[name] {
			add(CodeHTTPFieldUnused, lint.SeverityAdvisory, name, fmt.Sprintf("O campo de caminho %q de %q não aparece na URL", name, b.Function), "Use {"+name+"} na URL ou remova o campo.")
		}
	}
	hasBody := strings.TrimSpace(config.BodyTemplate) != ""
	for _, f := range config.Fields {
		hasBody = hasBody || f.Location == LocationBody
	}
	if strings.TrimSpace(config.BodyTemplate) != "" {
		checkTemplateScopes(add, "body_template", config.BodyTemplate, true, b.Function)
		if !json.Valid([]byte(sampleBody(config))) {
			add(CodeHTTPBodyNotJSON, lint.SeverityBlocking, "body_template", fmt.Sprintf("O modelo do corpo de %q não é um JSON válido", b.Function), "Cada marcador vira um valor JSON: escreva {\"pedido\": {{args.pedido}}} sem aspas em volta do marcador.")
		}
	}
	if hasBody && (config.Method == "GET" || config.Method == "HEAD" || config.Method == "DELETE") {
		r.add(toolIssue(b, CodeHTTPMethodBody, lint.SeverityAdvisory, fmt.Sprintf("%q envia corpo com %s, que muitos servidores ignoram", b.Function, config.Method), "Use POST, PUT ou PATCH para enviar corpo, ou passe os campos na consulta."))
	}
	outputs := map[string]bool{}
	for _, o := range config.Output.Fields {
		if strings.TrimSpace(o.Name) == "" || outputs[o.Name] || !ValidOutputPath(o.Path) {
			add(CodeHTTPOutputPathInvalid, lint.SeverityBlocking, o.Name, fmt.Sprintf("A saída %q de %q tem nome repetido ou caminho inválido (%s)", o.Name, b.Function, o.Path), "Use caminhos como data.status ou itens[0].id.")
		}
		outputs[o.Name] = true
	}
	switch config.Auth.Mode {
	case "", "bearer", "basic", "api_key":
	default:
		add(CodeConfigInvalid, lint.SeverityBlocking, "auth", fmt.Sprintf("A autenticação de %q usa um modo desconhecido", b.Function), "Escolha Bearer, Basic ou chave de API.")
	}
	if config.Auth.Mode != "" && strings.TrimSpace(config.Auth.Token) == "" {
		add(CodeConfigInvalid, lint.SeverityBlocking, "auth", fmt.Sprintf("A autenticação de %q não tem segredo", b.Function), "Escolha o segredo do workspace que guarda o token.")
	}
	checkTimeout(r, b, config.TimeoutSeconds)
}

var (
	fieldLocations = map[string]bool{LocationPath: true, LocationQuery: true, LocationHeader: true, LocationBody: true}
	fieldSources   = map[string]bool{SourceModel: true, SourceFixed: true, SourceContext: true}
	fieldTypes     = map[string]bool{"string": true, "number": true, "integer": true, "boolean": true, "object": true, "array": true}
	templateScopes = map[string]bool{ScopeVar: true, ScopeConversation: true, ScopeLead: true, ScopeSecret: true}
)

func fieldShapeProblem(f HTTPField) string {
	switch {
	case f.Name == "":
		return "não tem nome"
	case !fieldLocations[f.Location]:
		return "não diz onde vai (caminho, consulta, cabeçalho ou corpo)"
	case !fieldSources[f.Source]:
		return "não diz quem preenche (IA, fixo ou conversa)"
	case !fieldTypes[f.Type]:
		return "tem um tipo desconhecido"
	case f.Source != SourceModel && strings.TrimSpace(f.Value) == "":
		return "não tem valor"
	}
	return ""
}

func checkTemplateScopes(add func(string, lint.Severity, string, string, string), field, text string, allowArgs bool, function string) {
	for _, m := range templatePattern.FindAllStringSubmatch(text, -1) {
		scope := m[1]
		if templateScopes[scope] || (allowArgs && scope == ScopeArgs) {
			continue
		}
		add(CodeHTTPFieldUnbound, lint.SeverityBlocking, field, fmt.Sprintf("%q usa %s, que não existe aqui", function, m[0]), "Use {{var.nome}}, {{conversation.entry_id}}, {{lead.id}} ou {{secret.nome}}; {{args.campo}} só no modelo do corpo.")
	}
}

func sampleBody(config HTTPConfig) string {
	samples := map[string]interface{}{}
	for _, f := range config.ModelFields() {
		samples[f.ArgName()] = sampleOf(f.Type)
	}
	r := &renderer{args: samples, lookup: func(string, string) (string, bool) { return "x", true }}
	return r.json(config.BodyTemplate)
}

func sampleOf(fieldType string) interface{} {
	switch fieldType {
	case "number", "integer":
		return 1
	case "boolean":
		return true
	case "object":
		return map[string]interface{}{}
	case "array":
		return []interface{}{}
	}
	return "x"
}

func checkTimeout(r *Report, b agent.ToolBinding, seconds float64) {
	switch {
	case seconds == 0:
	case seconds < 1:
		r.add(toolIssue(b, CodeHTTPTimeoutRange, lint.SeverityBlocking, fmt.Sprintf("O tempo limite de %q precisa ser de 1 a 30 segundos", b.Function), "Use um valor entre 1 e 30."))
	case seconds > 30:
		r.add(toolIssue(b, CodeHTTPTimeoutRange, lint.SeverityAdvisory, fmt.Sprintf("O tempo limite de %q passa de 30 segundos e será limitado a 30", b.Function), "Use um valor entre 1 e 30."))
	}
}

func checkUniqueNames(r *Report, s Subject) {
	seen := map[string]bool{}
	for _, name := range s.OfferedNames {
		if name != "" {
			seen[strings.ToLower(name)] = true
		}
	}
	for _, b := range s.Tools {
		key := strings.ToLower(b.Function)
		if key == "" {
			continue
		}
		if seen[key] {
			r.add(toolIssue(b, CodeNameTaken, lint.SeverityBlocking, fmt.Sprintf("Duas funções do agente se chamam %q", b.Function), "Renomeie uma delas: a IA escolhe pelo nome."))
		}
		seen[key] = true
	}
}

func checkOverlaps(r *Report, list []agent.ToolBinding) {
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			a, b := wordSet(list[i].Description), wordSet(list[j].Description)
			if len(a) < overlapMinWords || len(b) < overlapMinWords || jaccard(a, b) < overlapThreshold {
				continue
			}
			r.add(toolIssue(list[j], CodeDescriptionsOverlap, lint.SeverityAdvisory, fmt.Sprintf("As descrições de %q e %q são quase iguais", list[i].Function, list[j].Function), "Diga em cada uma o que a diferencia, para a IA não confundir."))
		}
	}
}

func checkBudgets(r *Report, s Subject) {
	count := len(s.Tools) + len(s.OfferedNames)
	if count > manyToolsThreshold {
		r.add(Issue{Code: CodeToolsMany, Severity: lint.SeverityAdvisory, Message: fmt.Sprintf("O agente oferece %d funções; acima de %d a IA tende a escolher pior", count, manyToolsThreshold), Hint: "Junte ou remova ferramentas que o agente quase não usa."})
	}
	if tokens := EstimatedTokens(s.DefinitionChars); tokens > tokenBudgetAdvisory {
		r.add(Issue{Code: CodeToolsTokenBudget, Severity: lint.SeverityAdvisory, Message: fmt.Sprintf("As ferramentas somam cerca de %d tokens (estimativa) em toda resposta", tokens), Hint: "Encurte descrições e esquemas ou remova ferramentas pouco usadas."})
	}
}

func checkPromptMentions(r *Report, s Subject) {
	if strings.TrimSpace(s.Prompt) == "" {
		return
	}
	offered := map[string]bool{}
	for _, b := range s.Tools {
		offered[strings.ToLower(b.Function)] = true
	}
	for _, name := range s.OfferedNames {
		offered[strings.ToLower(name)] = true
	}
	known := map[string]bool{}
	for name := range s.Definitions {
		known[name] = true
	}
	var missing []string
	seen := map[string]bool{}
	for _, mention := range toolNameMentionPattern.FindAllString(s.Prompt, -1) {
		if seen[mention] || offered[mention] {
			continue
		}
		seen[mention] = true
		if known[mention] || strings.HasPrefix(mention, "run_workflow_") {
			missing = append(missing, mention)
		}
	}
	if len(missing) == 0 {
		return
	}
	sort.Strings(missing)
	r.add(Issue{Code: CodePromptUnknownTool, Severity: lint.SeverityAdvisory, Message: "O prompt cita ferramentas que o agente não oferece: " + strings.Join(missing, ", "), Hint: "Adicione as ferramentas ao agente ou atualize os nomes no prompt."})
}

func wordSet(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordPattern.FindAllString(strings.ToLower(text), -1) {
		if len([]rune(w)) > 2 {
			out[w] = true
		}
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	inter := 0
	for w := range a {
		if b[w] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
