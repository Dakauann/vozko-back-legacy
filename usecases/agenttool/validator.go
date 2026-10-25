package agenttool_usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"vozko/brand"
	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/ai"
	"vozko/domain/lint"
	"vozko/domain/tools"
	"vozko/domain/workspacesecret"
	tools_usecase "vozko/usecases/tools"
)

const (
	replyTokenReserve   = 2048
	historyTokenReserve = 4096
)

type SecretNames interface {
	List(ctx context.Context, workspaceID string) ([]workspacesecret.Summary, error)
}

type HostClassifier interface {
	ClassifyHost(ctx context.Context, host string) (agenttool.HostVerdict, error)
}

type ValidatorDeps struct {
	Registry tools.Service
	Secrets  SecretNames
	Hosts    HostClassifier
	Models   ai.ModelCatalog
}

type Validator struct {
	deps ValidatorDeps
}

func NewValidator(deps ValidatorDeps) *Validator {
	return &Validator{deps: deps}
}

func (v *Validator) Validate(ctx context.Context, a *agent.Agent, stored []agent.ToolBinding) agenttool.Report {
	if v == nil || v.deps.Registry == nil {
		return agenttool.Unavailable("o catálogo de ferramentas não está disponível")
	}
	if a == nil {
		return agenttool.Unavailable("o agente não foi carregado")
	}
	defs := map[string]tools.Definition{}
	for _, def := range v.deps.Registry.Definitions() {
		defs[strings.ToLower(def.Name)] = def
	}
	resolved := tools_usecase.ResolveToolsWithOptions(v.deps.Registry, a.InternalTools, agent.ToolVisibilityMessaging, tools_usecase.ToolResolverOptions{Agent: a})
	offered, chars := extraNamesAndSize(resolved.Definitions, a.InternalTools)
	report := agenttool.Check(agenttool.Subject{
		Prompt:          a.MessagingPrompt,
		Tools:           a.InternalTools,
		Stored:          stored,
		Definitions:     defs,
		OfferedNames:    offered,
		DefinitionChars: chars,
	})
	for _, check := range []func(context.Context, *agent.Agent, map[string]tools.Definition, int) (agenttool.Report, error){
		v.checkSecrets, v.checkHosts, v.checkBindings, v.checkModel,
	} {
		found, err := check(ctx, a, defs, chars)
		if err != nil {
			return report.Merge(agenttool.Unavailable(err.Error()))
		}
		report = report.Merge(found)
	}
	return report
}

func extraNamesAndSize(defs []tools.Definition, instances []agent.ToolBinding) ([]string, int) {
	own := map[string]bool{}
	for _, b := range instances {
		own[strings.ToLower(b.Function)] = true
	}
	var extra []string
	chars := 0
	for _, def := range defs {
		raw, _ := json.Marshal(struct {
			Name        string
			Description string
			Parameters  map[string]tools.Parameter
		}{def.Name, def.Description, def.Parameters})
		chars += len(raw)
		if def.Instance == nil && !own[strings.ToLower(def.Name)] {
			extra = append(extra, def.Name)
		}
	}
	return extra, chars
}

func (v *Validator) checkSecrets(ctx context.Context, a *agent.Agent, _ map[string]tools.Definition, _ int) (agenttool.Report, error) {
	var report agenttool.Report
	referenced := map[string][]string{}
	for _, b := range a.InternalTools {
		referenced[b.ID] = agenttool.SecretNames(stringsOf(b.Config)...)
	}
	if !anyNames(referenced) {
		return report, nil
	}
	if v.deps.Secrets == nil {
		return report, fmt.Errorf("os segredos do workspace não estão disponíveis")
	}
	list, err := v.deps.Secrets.List(ctx, a.WorkspaceID)
	if err != nil {
		return report, fmt.Errorf("os segredos do workspace não responderam")
	}
	existing := map[string]bool{}
	for _, s := range list {
		existing[s.Name] = true
	}
	for _, b := range a.InternalTools {
		var missing []string
		for _, name := range referenced[b.ID] {
			if !existing[name] {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			report.Issues = append(report.Issues, agenttool.Issue{
				Code: agenttool.CodeSecretMissing, Severity: lint.SeverityBlocking, ToolID: b.ID,
				Message: fmt.Sprintf("A ferramenta %q usa segredos que não existem neste workspace: %s", b.Function, strings.Join(missing, ", ")),
				Hint:    "Crie o segredo com esse nome ou escolha outro na lista de segredos.",
			})
		}
	}
	return report, nil
}

func (v *Validator) checkHosts(ctx context.Context, a *agent.Agent, defs map[string]tools.Definition, _ int) (agenttool.Report, error) {
	var report agenttool.Report
	for _, b := range a.InternalTools {
		def, ok := defs[strings.ToLower(b.Name)]
		if !ok || !strings.EqualFold(def.Name, tools_usecase.HTTPRequestToolName) {
			continue
		}
		host := literalHost(agenttool.ValueAt(b.Config, agenttool.URLParam, ""))
		if host == "" {
			continue
		}
		if v.deps.Hosts == nil {
			return report, fmt.Errorf("a conferência de endereços não está disponível")
		}
		verdict, err := v.deps.Hosts.ClassifyHost(ctx, host)
		if err != nil {
			return report, fmt.Errorf("não foi possível resolver %s agora", host)
		}
		switch verdict {
		case agenttool.HostBlocked:
			report.Issues = append(report.Issues, agenttool.Issue{Code: agenttool.CodeHTTPHostBlocked, Severity: lint.SeverityBlocking, ToolID: b.ID,
				Message: fmt.Sprintf("O servidor %s de %q aponta para um endereço interno, que %s nunca chama", host, b.Function, brand.Active().Portuguese().TheBrand),
				Hint:    "Use o endereço público da API."})
		case agenttool.HostNotFound:
			report.Issues = append(report.Issues, agenttool.Issue{Code: agenttool.CodeHTTPHostUnresolved, Severity: lint.SeverityBlocking, ToolID: b.ID,
				Message: fmt.Sprintf("O servidor %s de %q não existe", host, b.Function),
				Hint:    "Confira se o endereço está escrito certo."})
		}
	}
	return report, nil
}

func literalHost(raw string) string {
	_, rest, ok := strings.Cut(strings.TrimSpace(raw), "://")
	if !ok {
		return ""
	}
	if end := strings.IndexAny(rest, "/?#"); end >= 0 {
		rest = rest[:end]
	}
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		rest = rest[at+1:]
	}
	if strings.ContainsAny(rest, "{}") {
		return ""
	}
	if host, _, found := strings.Cut(rest, ":"); found && !strings.HasPrefix(rest, "[") {
		return host
	}
	return rest
}

func (v *Validator) checkBindings(ctx context.Context, a *agent.Agent, _ map[string]tools.Definition, _ int) (agenttool.Report, error) {
	var report agenttool.Report
	for _, b := range a.InternalTools {
		handler, ok := v.deps.Registry.Handler(strings.ToLower(b.Name))
		if !ok {
			continue
		}
		validator, ok := handler.(tools.BindingValidator)
		if !ok {
			continue
		}
		err := validator.ValidateBinding(ctx, a.WorkspaceID, b.Config)
		if err == nil {
			continue
		}
		problem, isProblem := tools.ProblemOf(err)
		if !isProblem {
			return report, fmt.Errorf("a conferência de %q não respondeu", b.Function)
		}
		report.Issues = append(report.Issues, agenttool.Issue{Code: problem.Code, Severity: lint.SeverityBlocking, ToolID: b.ID,
			Message: fmt.Sprintf("%q: %s", b.Function, problem.Message), Hint: "Escolha de novo o destino desta ferramenta."})
	}
	return report, nil
}

func (v *Validator) checkModel(ctx context.Context, a *agent.Agent, _ map[string]tools.Definition, chars int) (agenttool.Report, error) {
	var report agenttool.Report
	if len(a.InternalTools) == 0 && !a.HasMCPCollections() {
		return report, nil
	}
	if v.deps.Models == nil {
		return report, fmt.Errorf("o catálogo de modelos não está disponível")
	}
	models, err := v.deps.Models.GetModelsWithPricing(ctx)
	if err != nil {
		return report, fmt.Errorf("o catálogo de modelos não respondeu")
	}
	model, found := ai.FindModel(models, a.MessagingModel)
	if !found || !model.Has(ai.CapabilityTools) {
		report.Issues = append(report.Issues, agenttool.Issue{Code: agenttool.CodeModelWithoutTools, Severity: lint.SeverityBlocking,
			Message: fmt.Sprintf("O modelo %s não usa ferramentas", a.MessagingModel),
			Hint:    "Escolha um modelo com a capacidade de ferramentas ou remova as ferramentas."})
		return report, nil
	}
	if model.ContextLength <= 0 {
		return report, nil
	}
	needed := agenttool.EstimatedTokens(chars+len(a.MessagingPrompt)) + replyTokenReserve + historyTokenReserve
	if int64(needed) > model.ContextLength {
		report.Issues = append(report.Issues, agenttool.Issue{Code: agenttool.CodeToolsExceedContext, Severity: lint.SeverityBlocking,
			Message: fmt.Sprintf("Prompt e ferramentas somam cerca de %d tokens (estimativa), mais do que a janela de %d do modelo", needed, model.ContextLength),
			Hint:    "Escolha um modelo com janela maior ou reduza as ferramentas."})
	}
	return report, nil
}

func stringsOf(value interface{}) []string {
	var out []string
	switch v := value.(type) {
	case string:
		out = append(out, v)
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, stringsOf(v[k])...)
		}
	case []interface{}:
		for _, item := range v {
			out = append(out, stringsOf(item)...)
		}
	}
	return out
}

func anyNames(m map[string][]string) bool {
	for _, names := range m {
		if len(names) > 0 {
			return true
		}
	}
	return false
}
