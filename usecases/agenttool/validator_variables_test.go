package agenttool_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/automationneeds"
	"vozko/domain/lint"
	"vozko/domain/variabledecl"
)

type declaredTypes struct {
	catalog variabledecl.Catalog
	err     error
}

func (d declaredTypes) Rejected(_ context.Context, _ string, values map[string]string) ([]string, error) {
	if d.err != nil {
		return nil, d.err
	}
	var out []string
	for name, value := range values {
		if found, ok := d.catalog.Find(name); ok && found.Type.Kind == variabledecl.KindChoice && value != found.Type.Options[0] {
			out = append(out, name)
		}
	}
	return out, nil
}

func (d declaredTypes) Label(_ context.Context, _ string, needs automationneeds.Needs) automationneeds.Needs {
	return needs.Labeled(d.catalog)
}

func typedDeps() ValidatorDeps {
	deps := validatorDeps()
	deps.Variables = declaredTypes{catalog: variabledecl.NewCatalog([]variabledecl.Declaration{{Name: "turno", Label: "Turno", Type: variabledecl.Type{Kind: variabledecl.KindChoice, Options: []string{"Manhã"}}}})}
	return deps
}

func TestADefaultThatDoesNotFitItsDeclaredTypeBlocksPublishing(t *testing.T) {
	a := agentWith()
	a.Variables = []agent.AgentVariable{{Name: "turno", DefaultValue: "Noite"}, {Name: "livre", DefaultValue: "qualquer"}, {Name: "obrigatoria"}}
	r := NewValidator(typedDeps()).Validate(context.Background(), a, nil)
	issue, ok := codeOf(r, agenttool.CodeVariableDefaultInvalid)
	if !ok || issue.Severity != lint.SeverityBlocking || issue.ToolID != "" || issue.Field != "turno" || len(r.Issues) != 1 {
		t.Fatalf("issues %+v", r.Issues)
	}
	a.Variables[0].DefaultValue = "Manhã"
	if r := NewValidator(typedDeps()).Validate(context.Background(), a, nil); !r.IsGreen() {
		t.Fatalf("a fitting default = %+v", r.Issues)
	}
}

func TestTheDefaultCheckFailsClosed(t *testing.T) {
	a := agentWith()
	a.Variables = []agent.AgentVariable{{Name: "turno", DefaultValue: "Manhã"}}
	for name, deps := range map[string]ValidatorDeps{"no rules": validatorDeps(), "rules down": func() ValidatorDeps {
		d := typedDeps()
		d.Variables = declaredTypes{err: errors.New("db down")}
		return d
	}()} {
		r := NewValidator(deps).Validate(context.Background(), a, nil)
		if _, ok := codeOf(r, agenttool.CodeValidationOffline); !ok || r.IsGreen() {
			t.Fatalf("%s: issues %+v", name, r.Issues)
		}
	}
	a.Variables = []agent.AgentVariable{{Name: "obrigatoria"}}
	if r := NewValidator(validatorDeps()).Validate(context.Background(), a, nil); !r.IsGreen() {
		t.Fatalf("an agent without defaults needs no rules: %+v", r.Issues)
	}
}
