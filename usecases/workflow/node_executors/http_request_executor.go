package node_executors

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"time"

	"vozko/domain/httpcall"
	"vozko/domain/workflow"
	"vozko/domain/workspacesecret"
	httpcall_usecase "vozko/usecases/httpcall"
)

const (
	defaultHTTPRequestTimeout = httpcall.DefaultTimeout
	MaxHTTPRequestTimeout     = httpcall.MaxTimeout
	maxHTTPResponseBytes      = 64 * 1024
)

type HTTPEgress = httpcall_usecase.Egress

var normalizeRequestMap = workflow.RequestMap

type httpRequestExecutor struct {
	caller  *httpcall_usecase.Caller
	secrets workspacesecret.Resolver
}

func NewHTTPRequestExecutor(egress HTTPEgress, secrets workspacesecret.Resolver) workflow.NodeExecutor {
	return &httpRequestExecutor{caller: httpcall_usecase.NewCaller(egress), secrets: secrets}
}

func httpRequestTimeout(config map[string]interface{}) time.Duration {
	var seconds float64
	switch v := config["timeout_seconds"].(type) {
	case float64:
		seconds = v
	case int:
		seconds = float64(v)
	}
	return httpcall.Timeout(seconds)
}

func httpFailure(ctx *workflow.NodeContext, label, message string, kind workflow.ErrorKind) *workflow.NodeResult {
	result := routeFailure(&workflow.NodeResult{
		ErrorKind: kind,
		Output: map[string]interface{}{
			"success":   false,
			"error":     message,
			"next_edge": label,
		},
	}, outgoingEdges(ctx), label, timeoutFallback(label)...)
	result.Output["next_node_id"] = result.NextNodeID
	return result
}

func (e *httpRequestExecutor) Definition() workflow.NodeDefinition {
	return workflow.NodeDefinition{
		Type:         workflow.NodeTypeActionHTTPRequest,
		TestBehavior: workflow.NodeTestExternalCall,
		Category:     workflow.NodeCategoryAction,
		Scopes:       []workflow.NodeScope{workflow.NodeScopeShared, workflow.NodeScopeVoice},
		Label:        "Requisição HTTP",
		Description:  "Faz uma requisição HTTP para um serviço externo.",
		Icon:         "Globe",
		Guidance: workflow.NodeGuidance{
			When: "Para chamar uma API externa (ex.: ViaCEP, CRM, etc.).",
			Behavior: "Monte url, headers e corpo com variáveis (ex.: url com {{node.<id>.<arg>}}). " +
				"Se a resposta for JSON, CADA campo do corpo torna-se uma saída de dados deste nó, acessível adiante via {{node.<id>.<campo>}} (ex.: {{node.n4.logradouro}}). " +
				"Credenciais (senha, token, chave de API) ficam nos segredos do workspace: use {{secret.<nome>}} na url, nos headers, no corpo ou no token; o valor só é lido ao montar a requisição e nunca vai para o estado nem para os logs.",
			Examples: []string{
				"Após um agente em route com tool 'buscar_cep' (arg cep): url=\"https://viacep.com.br/ws/{{node.n2.cep}}/json/\"; depois envie {{node.n4.logradouro}}, {{node.n4.bairro}}.",
			},
		},
		DefaultConfig: map[string]interface{}{
			"method":  "GET",
			"url":     "",
			"headers": map[string]interface{}{},
		},
		Outputs: []workflow.HandleDefinition{
			{ID: "sucesso", Label: "Sucesso"},
			{ID: "falha", Label: "Falha", Optional: true},
			{ID: "timeout", Label: "Tempo esgotado", Optional: true},
		},
		OutputKeys: []workflow.OutputKeyDefinition{
			{Key: "status_code", Description: "Código HTTP (ex: 200, 404)", Label: "Código HTTP", Type: workflow.VariableTypeNumber},
			{Key: "body", Description: "Corpo bruto da resposta (string)", Label: "Corpo da resposta", Type: workflow.VariableTypeText},
			{Key: "success", Description: "true se status < 400", Label: "Deu certo", Type: workflow.VariableTypeBoolean},
			{Key: "*", Description: "Se a resposta for JSON, cada campo do objeto é extraído automaticamente. Ex: se o body for {\"bairro\":\"Sé\"}, use {{last.bairro}}"},
		},
		ConfigSchema: []workflow.ConfigField{
			{
				Key:   "method",
				Label: "Método",
				Type:  "select",
				Options: []workflow.ConfigFieldOption{
					{Value: "GET", Label: "GET"},
					{Value: "POST", Label: "POST"},
					{Value: "PUT", Label: "PUT"},
					{Value: "PATCH", Label: "PATCH"},
					{Value: "DELETE", Label: "DELETE"},
					{Value: "HEAD", Label: "HEAD"},
					{Value: "OPTIONS", Label: "OPTIONS"},
				},
				Required: true,
			},
			{Key: "url", Label: "URL", Type: "text", Placeholder: "https://api.example.com/endpoint", Required: true},
			{Key: "headers", Label: "Headers", Type: "keyvalue", Placeholder: "Content-Type: application/json", Description: "Cabeçalhos HTTP em formato chave-valor"},
			{Key: "query_params", Label: "Parâmetros de Query", Type: "keyvalue", Placeholder: "key=value", Description: "Parâmetros adicionados à URL (?key=value)"},
			{Key: "body", Label: "Corpo da Requisição", Type: "textarea", Placeholder: "{}", Description: "Corpo JSON da requisição (suporta variáveis)"},
			{Key: "auth_type", Label: "Autenticação", Type: "select", Options: []workflow.ConfigFieldOption{
				{Value: "", Label: "Nenhuma"},
				{Value: "bearer", Label: "Bearer Token"},
				{Value: "basic", Label: "Basic Auth"},
				{Value: "api_key", Label: "API Key (Header)"},
			}},
			{Key: "auth_token", Label: "Token / Senha", Type: "text", Placeholder: "Token ou senha (suporta variáveis)", Description: "Bearer token, senha do Basic Auth, ou valor da API Key"},
			{Key: "auth_user", Label: "Usuário (Basic Auth)", Type: "text", Placeholder: "username", Description: "Apenas para Basic Auth"},
			{Key: "auth_key_name", Label: "Nome do Header (API Key)", Type: "text", Placeholder: "X-API-Key", Description: "Nome do header para API Key"},
			{Key: "timeout_seconds", Label: "Timeout (segundos)", Type: "number", Placeholder: "15", Description: "Tempo máximo de espera pela resposta, até 30 segundos"},
			{Key: "capture_variable", Label: "Salvar resposta em variável", Type: "text", Placeholder: "nome_variavel", Description: "Salva o corpo da resposta em uma variável para uso em nós posteriores"},
		},
	}
}

func (e *httpRequestExecutor) Execute(ctx *workflow.NodeContext) (*workflow.NodeResult, error) {
	method, _ := ctx.Node.Config["method"].(string)
	rawURL, _ := ctx.Node.Config["url"].(string)

	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)

	if rawURL == "" {
		slog.Warn("workflow.http_request.empty_url", nodeLogAttrs(ctx)...)
		return httpFailure(ctx, "falha", "URL is empty", workflow.ErrorKindConfig), nil
	}

	secretNames := workflow.SecretRefs(ctx.Node.Config)
	secrets, err := resolveNodeSecrets(ctx, e.secrets, secretNames)
	if err == nil {
		err = requireAllSecrets(secretNames, secrets)
	}
	if err != nil {
		return secretFailure(err)
	}
	render := func(text string) string {
		return workflow.InterpolateWithSecrets(text, ctx.State, nil, secrets.values)
	}

	rawURL = render(rawURL)

	if qp := normalizeRequestMap(ctx.Node.Config["query_params"]); len(qp) > 0 {
		parsed, parseErr := url.Parse(rawURL)
		if parseErr == nil {
			q := parsed.Query()
			for k, v := range qp {
				q.Set(k, render(workflow.TemplateText(v)))
			}
			parsed.RawQuery = q.Encode()
			rawURL = parsed.String()
		}
	}

	bodyStr := ""
	if bs, ok := ctx.Node.Config["body"].(string); ok && bs != "" {
		bodyStr = render(bs)
	}

	headers := map[string]string{}
	for k, v := range normalizeRequestMap(ctx.Node.Config["headers"]) {
		headers[k] = render(workflow.TemplateText(v))
	}
	auth := httpcall.Auth{}
	auth.Mode, _ = ctx.Node.Config["auth_type"].(string)
	if token, _ := ctx.Node.Config["auth_token"].(string); token != "" {
		auth.Token = render(token)
	}
	if user, _ := ctx.Node.Config["auth_user"].(string); user != "" {
		auth.User = render(user)
	}
	auth.KeyName, _ = ctx.Node.Config["auth_key_name"].(string)

	callCtx, releaseCall := ctx.CallContext()
	defer releaseCall()
	outcome := e.caller.Do(callCtx, httpcall_usecase.Prepared{
		Method: method, URL: rawURL, Body: bodyStr, Headers: headers, Auth: auth,
		Timeout: httpRequestTimeout(ctx.Node.Config), ResponseLimit: maxHTTPResponseBytes,
	})
	target := requestTarget(ctx, outcome.Exchange.URL, method, secrets)
	if outcome.Kind == workflow.ErrorKindConfig {
		slog.Warn("workflow.http_request.refused", append(nodeLogAttrs(ctx), slog.String("method", method), slog.String("error", secrets.scrub.String(httpcall_usecase.TransportCause(outcome.Err))))...)
		return httpFailure(ctx, "falha", secrets.scrub.String(outcome.Err.Error()), workflow.ErrorKindConfig), nil
	}
	if outcome.Exchange.TLSFallback {
		slog.Info("workflow.http_request.tls_fallback", target...)
	}
	ctx.Trace.RecordHTTP(outcome.Exchange)
	if outcome.Err != nil {
		label := outputFailure
		if outcome.Kind == workflow.ErrorKindTimeout {
			label = outputTimeout
		}
		slog.Warn("workflow.http_request.transport_error", append(target, slog.String("label", label), slog.String("error", secrets.scrub.String(httpcall_usecase.TransportCause(outcome.Err))))...)
		return httpFailure(ctx, label, secrets.scrub.String(fmt.Sprintf("transport error: %v", outcome.Err)), outcome.Kind), nil
	}

	respBody := secrets.scrub.String(string(outcome.Body))
	output := map[string]interface{}{
		"status_code": outcome.Status,
		"body":        respBody,
		"success":     outcome.Status < 400,
	}

	var jsonResp interface{}
	if err := json.Unmarshal([]byte(respBody), &jsonResp); err == nil {
		output["json"] = jsonResp

		if obj, isObj := jsonResp.(map[string]interface{}); isObj {
			reserved := map[string]bool{"status_code": true, "body": true, "success": true, "json": true, "error": true}
			for k, v := range obj {
				if !reserved[k] {
					output[k] = v
				}
			}
		}
	}

	if captureVar, ok := ctx.Node.Config["capture_variable"].(string); ok && captureVar != "" {
		var captured interface{} = respBody
		if jsonResp != nil {
			captured = jsonResp
		}
		workflow.RecordHTTPCapture(ctx.State, captureVar, captured, outcome.Status)
	}

	edges := ctx.Graph.OutgoingEdges(ctx.Node.ID)
	var label string
	var kind workflow.ErrorKind
	if outcome.Status < 400 {
		label = "sucesso"
	} else {
		label, kind = outputFailure, workflow.ErrorKindHTTPStatus
		output["error"] = fmt.Sprintf("HTTP %d: %s", outcome.Status, respBody)
	}

	output["next_edge"] = label
	result := routeOutput(&workflow.NodeResult{Output: output, ErrorKind: kind}, edges, label)
	output["next_node_id"] = result.NextNodeID
	slog.Info("workflow.http_request", append(target, slog.Int("status", outcome.Status), slog.String("label", label))...)
	return result, nil
}

func requestTarget(ctx *workflow.NodeContext, rawURL, method string, secrets nodeSecrets) []any {
	host, path := "", ""
	if parsed, err := url.Parse(rawURL); err == nil {
		host, path = parsed.Host, parsed.Path
	}
	return append(nodeLogAttrs(ctx),
		slog.String("method", method),
		slog.String("host", secrets.scrub.String(host)),
		slog.String("path", secrets.scrub.String(path)),
	)
}
