package agenttool_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/ai"
	"vozko/domain/lint"
	"vozko/domain/tools"
	"vozko/domain/workspacesecret"
)

type fakeHandler struct {
	def      tools.Definition
	validate func(config map[string]interface{}) error
}

func (h fakeHandler) Definition() tools.Definition { return h.def }
func (h fakeHandler) Execute(context.Context, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, nil
}
func (h fakeHandler) ExecuteWithConfig(context.Context, map[string]interface{}, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, nil
}

type bindingHandler struct{ fakeHandler }

func (h bindingHandler) ValidateBinding(_ context.Context, _ string, config map[string]interface{}) error {
	return h.validate(config)
}

type fakeRegistry struct {
	tools.Service
	handlers map[string]tools.Handler
}

func (r fakeRegistry) Definitions() []tools.Definition {
	var out []tools.Definition
	for _, h := range r.handlers {
		out = append(out, h.Definition())
	}
	return out
}

func (r fakeRegistry) Handler(name string) (tools.Handler, bool) {
	h, ok := r.handlers[name]
	return h, ok
}

type secretList struct {
	names []string
	err   error
}

func (s secretList) List(context.Context, string) ([]workspacesecret.Summary, error) {
	out := make([]workspacesecret.Summary, len(s.names))
	for i, n := range s.names {
		out[i] = workspacesecret.Summary{Name: n}
	}
	return out, s.err
}

type hostVerdicts struct {
	verdict agenttool.HostVerdict
	err     error
}

func (h hostVerdicts) ClassifyHost(context.Context, string) (agenttool.HostVerdict, error) { return h.verdict, h.err }

type catalog struct {
	models []ai.ModelInfo
	err    error
}

func (c catalog) GetModelsWithPricing(context.Context) ([]ai.ModelInfo, error) { return c.models, c.err }

var toolModel = ai.ModelInfo{ID: "openai/gpt-4o", ContextLength: 128000, Capabilities: []ai.ModelCapability{ai.CapabilityTools}}

func validatorDeps() ValidatorDeps {
	httpDef := tools.Definition{Name: "http_request", Instances: tools.Many(), Description: "HTTP", RequiredConfig: []string{"url", "method"}, ConfigSchema: map[string]tools.ConfigParameter{
		"url": {Type: "string"}, "method": {Type: "string"}, "headers": {Type: "object", Sensitive: true},
	}}
	flowDef := tools.Definition{Name: "run_workflow", Instances: tools.PerTarget("workflow_id"), Description: "Fluxo", ConfigSchema: map[string]tools.ConfigParameter{"workflow_id": {Type: "string"}}}
	return ValidatorDeps{
		Registry: fakeRegistry{handlers: map[string]tools.Handler{
			"http_request": fakeHandler{def: httpDef},
			"run_workflow": bindingHandler{fakeHandler{def: flowDef, validate: func(config map[string]interface{}) error {
				if config["workflow_id"] == "wf-draft" {
					return tools.Invalid(agenttool.CodeWorkflowNotCallable, "o fluxo não está publicado")
				}
				if config["workflow_id"] == "wf-offline" {
					return errors.New("db down")
				}
				return nil
			}}},
		}},
		Secrets: secretList{names: []string{"loja_token"}},
		Hosts:   hostVerdicts{verdict: agenttool.HostOK},
		Models:  catalog{models: []ai.ModelInfo{toolModel}},
	}
}

func agentWith(bindings ...agent.ToolBinding) *agent.Agent {
	return &agent.Agent{ID: "ag-1", WorkspaceID: "ws-1", MessagingModel: "openai/gpt-4o", MessagingPrompt: "Atenda o cliente.", InternalTools: bindings}
}

const description = "Consulta o status de um pedido pelo número informado. Use quando o cliente perguntar pela compra. Não use para trocas. Retorna o status."

func httpTool(url string, headers map[string]interface{}) agent.ToolBinding {
	return agent.ToolBinding{ID: "t1", Name: "http_request", Function: "consultar_pedido", Description: description, Config: map[string]interface{}{"url": url, "method": "GET", "headers": headers}}
}

func codeOf(r agenttool.Report, code string) (agenttool.Issue, bool) {
	for _, i := range r.Issues {
		if i.Code == code {
			return i, true
		}
	}
	return agenttool.Issue{}, false
}

func TestAHealthyAgentValidatesGreen(t *testing.T) {
	r := NewValidator(validatorDeps()).Validate(context.Background(), agentWith(httpTool("https://api.loja.test/orders", map[string]interface{}{"Authorization": "{{secret.loja_token}}"})), nil)
	if !r.IsGreen() || len(r.Issues) != 0 {
		t.Fatalf("issues %+v", r.Issues)
	}
}

func TestAReferenceToAMissingSecretBlocks(t *testing.T) {
	r := NewValidator(validatorDeps()).Validate(context.Background(), agentWith(httpTool("https://api.loja.test/orders?k={{secret.absent}}", map[string]interface{}{"Authorization": "{{secret.loja_token}}"})), nil)
	i, ok := codeOf(r, agenttool.CodeSecretMissing)
	if !ok || i.Severity != lint.SeverityBlocking || i.ToolID != "t1" {
		t.Fatalf("issues %+v", r.Issues)
	}
}

func TestBlockedAndUnresolvedHostsBlock(t *testing.T) {
	for verdict, code := range map[agenttool.HostVerdict]string{agenttool.HostBlocked: agenttool.CodeHTTPHostBlocked, agenttool.HostNotFound: agenttool.CodeHTTPHostUnresolved} {
		deps := validatorDeps()
		deps.Hosts = hostVerdicts{verdict: verdict}
		r := NewValidator(deps).Validate(context.Background(), agentWith(httpTool("https://intranet.test/x", nil)), nil)
		if i, ok := codeOf(r, code); !ok || i.Severity != lint.SeverityBlocking {
			t.Fatalf("%s: issues %+v", code, r.Issues)
		}
	}
}

func TestATargetThatCannotBeCalledBlocks(t *testing.T) {
	flow := agent.ToolBinding{ID: "t2", Name: "run_workflow", Function: "run_workflow_wf-draft", Config: map[string]interface{}{"workflow_id": "wf-draft"}}
	r := NewValidator(validatorDeps()).Validate(context.Background(), agentWith(flow), nil)
	if i, ok := codeOf(r, agenttool.CodeWorkflowNotCallable); !ok || i.ToolID != "t2" || i.Severity != lint.SeverityBlocking {
		t.Fatalf("issues %+v", r.Issues)
	}
}

func TestAModelWithoutToolsBlocks(t *testing.T) {
	deps := validatorDeps()
	deps.Models = catalog{models: []ai.ModelInfo{{ID: "openai/gpt-4o"}}}
	r := NewValidator(deps).Validate(context.Background(), agentWith(httpTool("https://api.loja.test/x", nil)), nil)
	if _, ok := codeOf(r, agenttool.CodeModelWithoutTools); !ok {
		t.Fatalf("issues %+v", r.Issues)
	}
}

func TestToolsThatDoNotFitTheContextWindowBlock(t *testing.T) {
	deps := validatorDeps()
	deps.Models = catalog{models: []ai.ModelInfo{{ID: "openai/gpt-4o", ContextLength: 1000, Capabilities: []ai.ModelCapability{ai.CapabilityTools}}}}
	r := NewValidator(deps).Validate(context.Background(), agentWith(httpTool("https://api.loja.test/x", nil)), nil)
	if i, ok := codeOf(r, agenttool.CodeToolsExceedContext); !ok || i.Severity != lint.SeverityBlocking {
		t.Fatalf("issues %+v", r.Issues)
	}
}

func TestEveryUnavailableCheckFailsClosed(t *testing.T) {
	cases := map[string]func(*ValidatorDeps){
		"secrets":  func(d *ValidatorDeps) { d.Secrets = secretList{err: errors.New("vault down")} },
		"no store": func(d *ValidatorDeps) { d.Secrets = nil },
		"hosts":    func(d *ValidatorDeps) { d.Hosts = hostVerdicts{err: errors.New("dns down")} },
		"no hosts": func(d *ValidatorDeps) { d.Hosts = nil },
		"models":   func(d *ValidatorDeps) { d.Models = catalog{err: errors.New("catalog down")} },
		"binding":  func(d *ValidatorDeps) {},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			deps := validatorDeps()
			mutate(&deps)
			bindings := []agent.ToolBinding{httpTool("https://api.loja.test/x?k={{secret.loja_token}}", nil)}
			if name == "binding" {
				bindings = append(bindings, agent.ToolBinding{ID: "t2", Name: "run_workflow", Function: "run_workflow_wf-offline", Config: map[string]interface{}{"workflow_id": "wf-offline"}})
			}
			r := NewValidator(deps).Validate(context.Background(), agentWith(bindings...), nil)
			if i, ok := codeOf(r, agenttool.CodeValidationOffline); !ok || i.Severity != lint.SeverityBlocking || r.IsGreen() {
				t.Fatalf("issues %+v", r.Issues)
			}
		})
	}
}

func TestAnAgentWithoutToolsSkipsTheModelCheck(t *testing.T) {
	deps := validatorDeps()
	deps.Models = catalog{err: errors.New("catalog down")}
	if r := NewValidator(deps).Validate(context.Background(), agentWith(), nil); !r.IsGreen() {
		t.Fatalf("issues %+v", r.Issues)
	}
}
