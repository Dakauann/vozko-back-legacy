package automationneeds

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"vozko/domain/variabledecl"
)

type typedRules struct {
	catalog variabledecl.Catalog
	asked   map[string]string
	err     error
}

func (r *typedRules) Rejected(_ context.Context, _ string, values map[string]string) ([]string, error) {
	r.asked = values
	if r.err != nil {
		return nil, r.err
	}
	var out []string
	for name, value := range values {
		if d, ok := r.catalog.Find(name); ok && d.Type.Kind == variabledecl.KindNumber && value == "dez" {
			out = append(out, name)
		}
	}
	return out, nil
}

func (r *typedRules) Label(_ context.Context, _ string, needs Needs) Needs {
	return needs.Labeled(r.catalog)
}

func TestRejectedDefaultsNamesTheDefaultsThatDoNotFitTheirDeclaredType(t *testing.T) {
	rules := &typedRules{catalog: variabledecl.NewCatalog([]variabledecl.Declaration{{Name: "valor", Label: "Valor da parcela", Type: variabledecl.Type{Kind: variabledecl.KindNumber}}})}
	got, err := RejectedDefaults(context.Background(), rules, "ws", map[string]string{"valor": "dez", "livre": "dez"})
	if err != nil || len(got) != 1 || got[0].Name != "valor" || got[0].Display() != "Valor da parcela" {
		t.Fatalf("rejected = %+v, err = %v", got, err)
	}
	if got, err := RejectedDefaults(context.Background(), rules, "ws", nil); err != nil || got != nil {
		t.Fatalf("no defaults = %+v, %v", got, err)
	}
	if !reflect.DeepEqual(rules.asked, map[string]string{"valor": "dez", "livre": "dez"}) {
		t.Fatalf("asked = %v", rules.asked)
	}
	rules.err = errors.New("down")
	if _, err := RejectedDefaults(context.Background(), rules, "ws", map[string]string{"valor": "1"}); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
}

func TestSheetModelProblemAcceptsOnlyADeclaredPlanilhaOfTheSameModel(t *testing.T) {
	catalog := variabledecl.NewCatalog([]variabledecl.Declaration{
		{Name: "planilha", Label: "Planilha da franquia", Type: variabledecl.Type{Kind: variabledecl.KindSheet, Settings: map[string]string{variabledecl.SettingModel: "sp-model"}}},
		{Name: "valor", Label: "Valor", Type: variabledecl.Type{Kind: variabledecl.KindNumber}},
	})
	labeled := func(name string) Needs { return Needs{{Name: name}}.Labeled(catalog) }
	if err := SheetModelProblem(labeled("planilha"), "sp-model"); err != nil {
		t.Fatal(err)
	}
	if err := SheetModelProblem(labeled("planilha"), "sp-other"); !errors.Is(err, ErrSheetModelMismatch) {
		t.Fatalf("other model = %v", err)
	}
	for _, needs := range []Needs{labeled("valor"), labeled("livre"), nil} {
		if err := SheetModelProblem(needs, "sp-model"); !errors.Is(err, ErrNotSheetVariable) {
			t.Fatalf("%+v = %v", needs, err)
		}
	}
}
