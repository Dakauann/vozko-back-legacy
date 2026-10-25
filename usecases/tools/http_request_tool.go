package tools_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"vozko/domain/agenttool"
	"vozko/domain/calltrace"
	"vozko/domain/httpcall"
	"vozko/domain/redact"
	"vozko/domain/tools"
	"vozko/domain/workflow"
	"vozko/domain/workspacesecret"
	httpcall_usecase "vozko/usecases/httpcall"
)

const (
	httpRequestToolName = workflow.AgentHTTPRequestTool
	HTTPRequestToolName = httpRequestToolName
)

const (
	maxResponseBytes        = 1 << 20
	ModelResultCharLimit    = 15000
	truncatedMarker         = "… [resposta cortada]"
	maxIdempotentRetries    = 2
	defaultIdempotencyKey   = "Idempotency-Key"
	httpToolOverallDeadline = httpcall.MaxTimeout
)

var idempotencyNamespace = uuid.MustParse("1b8f4a52-7c3e-5d90-9a61-2e4f8c0d3b17")

type httpRequestTool struct {
	caller  *httpcall_usecase.Caller
	secrets workspacesecret.Resolver
	sleep   func(context.Context, time.Duration) bool
}

func NewHTTPRequestToolUseCase(egress httpcall_usecase.Egress, secrets workspacesecret.Resolver) tools.Handler {
	return &httpRequestTool{caller: httpcall_usecase.NewCaller(egress), secrets: secrets, sleep: sleepFor}
}

func sleepFor(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (t *httpRequestTool) Definition() tools.Definition {
	return tools.Definition{
		Name:               httpRequestToolName,
		DisplayName:        "Requisição HTTP",
		DisplayDescription: "Chama uma API externa com campos que a IA preenche, valores fixos e dados da conversa.",
		Description:        "Chama uma API externa configurada para este agente.",
		Parameters:         map[string]tools.Parameter{},
		Visibility:         []tools.ToolVisibility{tools.VisibilityMessaging},
		Category:           tools.CategoryAgentAction,
		ConfigSchema: map[string]tools.ConfigParameter{
			"url":                {Type: "string", DisplayName: "URL do endpoint", DisplayDescription: "Endereço https da API. Use {campo} no caminho para valores que a IA ou a conversa preenchem.", Required: true},
			"method":             {Type: "string", DisplayName: "Método HTTP", DisplayDescription: "Operação usada em todas as chamadas desta ferramenta.", Default: "GET", Required: true, Options: []tools.ConfigParameterOption{{Value: "GET", Label: "GET"}, {Value: "POST", Label: "POST"}, {Value: "PUT", Label: "PUT"}, {Value: "PATCH", Label: "PATCH"}, {Value: "DELETE", Label: "DELETE"}}},
			"fields":             {Type: "array", DisplayName: "Campos", DisplayDescription: "Cada campo do caminho, da consulta, do cabeçalho ou do corpo, e quem o preenche: a IA, um valor fixo ou a conversa."},
			"body_template":      {Type: "string", DisplayName: "Modelo do corpo", DisplayDescription: "JSON com {{args.campo}}, {{var.nome}}, {{conversation.entry_id}}, {{lead.id}} e {{secret.nome}}; cada marcador vira um valor JSON."},
			"auth":               {Type: "object", DisplayName: "Autenticação", DisplayDescription: "Bearer, Basic ou chave de API, sempre com um segredo do workspace."},
			"output":             {Type: "object", DisplayName: "Resposta", DisplayDescription: "Caminhos da resposta que a IA recebe. Sem caminhos, a IA recebe o JSON da resposta."},
			"timeout_seconds":    {Type: "number", DisplayName: "Tempo limite", DisplayDescription: "De 1 a 30 segundos; o padrão é 15.", Default: 15},
			"idempotency_header": {Type: "string", DisplayName: "Cabeçalho de idempotência", DisplayDescription: "Nome do cabeçalho com a chave que POST e PATCH enviam. O padrão é Idempotency-Key."},
			"headers":            {Type: "object", DisplayName: "Cabeçalhos fixos", DisplayDescription: "Forma antiga, convertida em campos ao salvar.", Sensitive: true},
			"path_params":        {Type: "array", DisplayName: "Parâmetros de caminho", DisplayDescription: "Forma antiga, convertida em campos ao salvar."},
			"query_schema":       {Type: "array", DisplayName: "Parâmetros de consulta", DisplayDescription: "Forma antiga, convertida em campos ao salvar."},
			"body_schema":        {Type: "array", DisplayName: "Campos do corpo", DisplayDescription: "Forma antiga, convertida em campos ao salvar."},
		},
		RequiredConfig: []string{"url", "method"},
		Instances:      tools.Many(),
		RequiresConfig: true,
	}
}

func (t *httpRequestTool) DefinitionWithContext(ctx tools.ToolContext) tools.Definition {
	def := t.Definition()
	config, err := agenttool.ParseHTTPConfig(ctx.Config)
	if err != nil {
		return def
	}
	def.Parameters, def.Required = config.Parameters()
	return def
}

func (t *httpRequestTool) Execute(ctx context.Context, params map[string]interface{}) (tools.ExecutionResult, error) {
	return t.ExecuteWithConfig(ctx, nil, params)
}

func (t *httpRequestTool) ExecuteWithConfig(ctx context.Context, config map[string]interface{}, params map[string]interface{}) (tools.ExecutionResult, error) {
	call, err := agenttool.ParseHTTPConfig(config)
	if err != nil || call.URL == "" {
		return failed(calltrace.ErrorKindConfig, "a ferramenta não está configurada", nil, nil), nil
	}
	secrets, err := httpcall_usecase.ResolveSecrets(ctx, t.secrets, configString(config, "__workspace_id"), secretNamesOf(call))
	if err != nil {
		return failed(calltrace.ErrorKindSecret, err.Error(), err, nil), nil
	}
	rendered, err := call.Render(params, contextLookup(config, secrets.Values))
	if err != nil {
		var callErr *agenttool.CallError
		kind := calltrace.ErrorKindRender
		if errors.As(err, &callErr) {
			kind = callErr.Kind
		}
		return failed(kind, secrets.Scrub.String(err.Error()), err, nil), nil
	}
	scrub := redact.SecretsOnly(append(rendered.Secrets, secretValues(secrets.Values)...)...)
	headers := rendered.Headers
	if !httpcall.IdempotentMethod(rendered.Method) {
		name := call.IdempotencyHeader
		if name == "" {
			name = defaultIdempotencyKey
		}
		headers[name] = idempotencyKey(ctx)
	}

	budget, cancel := context.WithTimeout(ctx, httpToolOverallDeadline)
	defer cancel()
	prepared := httpcall_usecase.Prepared{
		Method: rendered.Method, URL: rendered.URL, Body: rendered.Body, Headers: headers,
		Auth:    httpcall.Auth{Mode: rendered.Auth.Mode, User: rendered.Auth.User, Token: rendered.Auth.Token, KeyName: rendered.Auth.KeyName},
		Timeout: httpcall.Timeout(call.TimeoutSeconds), ResponseLimit: maxResponseBytes,
	}
	var exchanges []calltrace.HTTPExchange
	var outcome httpcall_usecase.Outcome
	for attempt := 0; ; attempt++ {
		outcome = t.caller.Do(budget, prepared)
		outcome.Exchange.Attempt = attempt + 1
		exchanges = append(exchanges, outcome.Exchange.Redact(scrub).Redact(redact.New()))
		wait, retry := retryAfter(rendered.Method, outcome, attempt)
		if !retry {
			break
		}
		if deadline, ok := budget.Deadline(); ok && time.Until(deadline) <= wait {
			break
		}
		if !t.sleep(budget, wait) {
			break
		}
	}
	trace := &calltrace.ToolTrace{Exchanges: calltrace.FitHTTPExchanges(exchanges)}

	if outcome.Err != nil {
		message := fmt.Sprintf("a chamada à API falhou (%s): %s", outcome.Kind, scrub.String(httpcall_usecase.TransportCause(outcome.Err)))
		return failed(outcome.Kind, message, outcome.Err, trace), nil
	}
	body := scrub.String(string(outcome.Body))
	contextUpdate := fmt.Sprintf("a API respondeu %d", outcome.Status)
	if outcome.Status < 200 || outcome.Status >= 300 {
		trace.ErrorKind = calltrace.ErrorKindHTTPStatus
		trace.ErrorChain = []string{fmt.Sprintf("HTTP %d", outcome.Status)}
		return tools.ExecutionResult{
			Result:            map[string]interface{}{"status": outcome.Status, "error": capForModel(body)},
			IsError:           true,
			ContextUpdateText: contextUpdate,
			Trace:             trace,
		}, nil
	}
	mapped, err := call.MapOutput(outcome.Status, []byte(body))
	if err != nil {
		return failed(calltrace.ErrorKindMapping, err.Error(), err, trace), nil
	}
	trace.MappedOutput = mapped
	return tools.ExecutionResult{Result: modelValue(mapped), ContextUpdateText: contextUpdate, Trace: trace}, nil
}

func failed(kind calltrace.ErrorKind, message string, cause error, trace *calltrace.ToolTrace) tools.ExecutionResult {
	if trace == nil {
		trace = &calltrace.ToolTrace{}
	}
	trace.ErrorKind = kind
	if cause != nil {
		trace.ErrorChain = redact.New().Strings(calltrace.ErrorChain(cause))
	} else {
		trace.ErrorChain = []string{message}
	}
	return tools.ExecutionResult{Result: message, IsError: true, ContextUpdateText: message, Trace: trace}
}

func retryAfter(method string, outcome httpcall_usecase.Outcome, attempt int) (time.Duration, bool) {
	if attempt >= maxIdempotentRetries || !httpcall.IdempotentMethod(method) {
		return 0, false
	}
	switch {
	case outcome.Err != nil && (outcome.Kind == calltrace.ErrorKindTransport || outcome.Kind == calltrace.ErrorKindTimeout) && outcome.Exchange.EgressRefusal == "":
		return backoff(attempt), true
	case outcome.Status == http.StatusTooManyRequests || outcome.Status == http.StatusServiceUnavailable:
		if seconds, err := strconv.Atoi(strings.TrimSpace(outcome.Header.Get("Retry-After"))); err == nil && seconds >= 0 {
			return time.Duration(seconds) * time.Second, true
		}
		return backoff(attempt), true
	}
	return 0, false
}

func backoff(attempt int) time.Duration {
	return time.Duration(250*(attempt+1)) * time.Millisecond
}

func idempotencyKey(ctx context.Context) string {
	seed := tools.RunIDFrom(ctx) + "\x00" + tools.CallIDFrom(ctx)
	if ref, ok := tools.InstanceFrom(ctx); ok {
		seed += "\x00" + ref.ID
	}
	if strings.Trim(seed, "\x00") == "" {
		return uuid.NewString()
	}
	return uuid.NewSHA1(idempotencyNamespace, []byte(seed)).String()
}

func secretNamesOf(c agenttool.HTTPConfig) []string {
	texts := []string{c.URL, c.BodyTemplate, c.Auth.Token, c.Auth.User}
	for _, f := range c.Fields {
		if f.Source != agenttool.SourceModel {
			texts = append(texts, f.Value)
		}
	}
	return agenttool.SecretNames(texts...)
}

func contextLookup(config map[string]interface{}, secrets map[string]string) agenttool.Lookup {
	return func(scope, key string) (string, bool) {
		switch scope {
		case agenttool.ScopeSecret:
			value, ok := secrets[key]
			return value, ok
		case agenttool.ScopeConversation:
			value := configString(config, "__"+key)
			return value, value != "" && (key == "entry_id" || key == "entry_type")
		case agenttool.ScopeLead:
			value := configString(config, "__lead_id")
			return value, key == "id" && value != ""
		case agenttool.ScopeVar:
			switch vars := config["__vars"].(type) {
			case map[string]string:
				value, ok := vars[key]
				return value, ok
			case map[string]interface{}:
				value, ok := vars[key].(string)
				return value, ok
			}
		}
		return "", false
	}
}

func secretValues(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, v)
	}
	return out
}

func modelValue(value interface{}) interface{} {
	if s, ok := value.(string); ok {
		return capForModel(s)
	}
	raw, err := json.Marshal(value)
	if err != nil || len([]rune(string(raw))) <= ModelResultCharLimit {
		return value
	}
	return capForModel(string(raw))
}

func capForModel(text string) string {
	runes := []rune(text)
	if len(runes) <= ModelResultCharLimit {
		return text
	}
	return string(runes[:ModelResultCharLimit]) + truncatedMarker
}

var (
	_ tools.Handler           = (*httpRequestTool)(nil)
	_ tools.ContextualHandler = (*httpRequestTool)(nil)
)

func (t *httpRequestTool) NormalizeConfig(config map[string]interface{}) (map[string]interface{}, error) {
	if len(config) == 0 {
		return config, nil
	}
	return agenttool.CanonicalHTTPConfig(config)
}

var _ tools.ConfigNormalizer = (*httpRequestTool)(nil)
