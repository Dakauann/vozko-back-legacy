package variabledecl

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func builtinRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry(Builtin()...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNewRegistryRefusesTwoRulesForOneKind(t *testing.T) {
	if _, err := NewRegistry(append(Builtin(), Builtin()[0])...); !errors.Is(err, ErrKindTaken) {
		t.Fatalf("err = %v, want ErrKindTaken", err)
	}
}

func TestBuiltinKindsAndShapes(t *testing.T) {
	r := builtinRegistry(t)
	want := map[Kind]Shape{KindText: ShapeText, KindNumber: ShapeNumber, KindDate: ShapeDate, KindChoice: ShapeChoice}
	got := map[Kind]Shape{}
	for _, kind := range r.Kinds() {
		rule, _ := r.Rule(kind)
		got[kind] = rule.Shape()
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
}

func TestCheckDeclarationNormalizesAndRefuses(t *testing.T) {
	r := builtinRegistry(t)
	ok, err := r.CheckDeclaration(Declaration{Name: " matricula ", Label: "  Matrícula ", Description: " Número do aluno ", Type: Type{Kind: KindText}})
	if err != nil {
		t.Fatal(err)
	}
	if ok.Name != "matricula" || ok.Label != "Matrícula" || ok.Description != "Número do aluno" {
		t.Fatalf("normalized = %+v", ok)
	}
	choice, err := r.CheckDeclaration(Declaration{Name: "turno", Label: "Turno", Type: Type{Kind: KindChoice, Options: []string{" Manhã ", "Tarde"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(choice.Type.Options, []string{"Manhã", "Tarde"}) {
		t.Fatalf("options = %v", choice.Type.Options)
	}
	cases := []struct {
		name string
		decl Declaration
		want error
	}{
		{"empty name", Declaration{Label: "X", Type: Type{Kind: KindText}}, ErrNameInvalid},
		{"spaces in name", Declaration{Name: "minha var", Label: "X", Type: Type{Kind: KindText}}, ErrNameInvalid},
		{"accent in name", Declaration{Name: "matrícula", Label: "X", Type: Type{Kind: KindText}}, ErrNameInvalid},
		{"long name", Declaration{Name: strings.Repeat("a", 65), Label: "X", Type: Type{Kind: KindText}}, ErrNameInvalid},
		{"system key", Declaration{Name: KeyLeadName, Label: "X", Type: Type{Kind: KindText}}, ErrNameReserved},
		{"no label", Declaration{Name: "a", Label: " ", Type: Type{Kind: KindText}}, ErrLabelInvalid},
		{"long label", Declaration{Name: "a", Label: strings.Repeat("a", 61), Type: Type{Kind: KindText}}, ErrLabelInvalid},
		{"long description", Declaration{Name: "a", Label: "A", Description: strings.Repeat("a", 501), Type: Type{Kind: KindText}}, ErrDescriptionTooLong},
		{"unknown kind", Declaration{Name: "a", Label: "A", Type: Type{Kind: "planet"}}, ErrKindUnknown},
		{"options on text", Declaration{Name: "a", Label: "A", Type: Type{Kind: KindText, Options: []string{"x"}}}, ErrTypeInvalid},
		{"settings on number", Declaration{Name: "a", Label: "A", Type: Type{Kind: KindNumber, Settings: map[string]string{"x": "y"}}}, ErrTypeInvalid},
		{"choice without options", Declaration{Name: "a", Label: "A", Type: Type{Kind: KindChoice}}, ErrTypeInvalid},
		{"choice repeated option", Declaration{Name: "a", Label: "A", Type: Type{Kind: KindChoice, Options: []string{"x", " x"}}}, ErrTypeInvalid},
		{"choice blank option", Declaration{Name: "a", Label: "A", Type: Type{Kind: KindChoice, Options: []string{"x", " "}}}, ErrTypeInvalid},
		{"choice long option", Declaration{Name: "a", Label: "A", Type: Type{Kind: KindChoice, Options: []string{strings.Repeat("o", 101)}}}, ErrTypeInvalid},
		{"choice too many", Declaration{Name: "a", Label: "A", Type: Type{Kind: KindChoice, Options: manyOptions(51)}}, ErrTypeInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := r.CheckDeclaration(tc.decl); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func manyOptions(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = strings.Repeat("o", i+1)
	}
	return out
}

func TestBuiltinValidators(t *testing.T) {
	r := builtinRegistry(t)
	cases := []struct {
		typ    Type
		value  string
		accept bool
	}{
		{Type{Kind: KindText}, "qualquer coisa", true},
		{Type{Kind: KindText}, strings.Repeat("a", 2000), true},
		{Type{Kind: KindText}, strings.Repeat("a", 2001), false},
		{Type{Kind: KindText}, "  ", false},
		{Type{Kind: KindNumber}, "1234", true},
		{Type{Kind: KindNumber}, "-12", true},
		{Type{Kind: KindNumber}, "1234.5", true},
		{Type{Kind: KindNumber}, "1234,5", true},
		{Type{Kind: KindNumber}, "1.234,56", true},
		{Type{Kind: KindNumber}, "1,234.56", true},
		{Type{Kind: KindNumber}, " 42 ", true},
		{Type{Kind: KindNumber}, "12a", false},
		{Type{Kind: KindNumber}, "1.2.3", false},
		{Type{Kind: KindNumber}, "R$ 10", false},
		{Type{Kind: KindNumber}, "", false},
		{Type{Kind: KindDate}, "17/10/2026", true},
		{Type{Kind: KindDate}, "2026-10-17", true},
		{Type{Kind: KindDate}, "31/02/2026", false},
		{Type{Kind: KindDate}, "2026-13-01", false},
		{Type{Kind: KindDate}, "17-10-2026", false},
		{Type{Kind: KindDate}, "amanhã", false},
		{Type{Kind: KindChoice, Options: []string{"Manhã", "Tarde"}}, "Tarde", true},
		{Type{Kind: KindChoice, Options: []string{"Manhã", "Tarde"}}, " Tarde ", true},
		{Type{Kind: KindChoice, Options: []string{"Manhã", "Tarde"}}, "tarde", false},
		{Type{Kind: KindChoice, Options: []string{"Manhã", "Tarde"}}, "Noite", false},
	}
	for _, tc := range cases {
		err := r.Accepts(context.Background(), Check{WorkspaceID: "ws", Name: "x", Type: tc.typ, Value: tc.value})
		if (err == nil) != tc.accept {
			t.Errorf("%s %q: err = %v, accept want %v", tc.typ.Kind, tc.value, err, tc.accept)
		}
		if err != nil && !errors.Is(err, ErrValueInvalid) {
			t.Errorf("%s %q: err = %v, want ErrValueInvalid", tc.typ.Kind, tc.value, err)
		}
	}
}

type portRule struct {
	err error
}

func (portRule) Kind() Kind                             { return "port" }
func (portRule) Sample(Type) string                     { return "x" }
func (portRule) Shape() Shape                           { return ShapeText }
func (portRule) CheckType(Type) error                   { return nil }
func (p portRule) Accepts(context.Context, Check) error { return p.err }

func TestRejectedNamesValuesThatFailTheirDeclaredTypeAndTreatsUndeclaredAsText(t *testing.T) {
	r := builtinRegistry(t)
	catalog := NewCatalog([]Declaration{
		{Name: "valor", Label: "Valor", Type: Type{Kind: KindNumber}},
		{Name: "turno", Label: "Turno", Type: Type{Kind: KindChoice, Options: []string{"Manhã"}}},
	})
	rejected, err := r.Rejected(context.Background(), "ws", catalog, map[string]string{"valor": "abc", "turno": "Manhã", "livre": "ok", "longo": strings.Repeat("a", 2001)})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rejected, []string{"longo", "valor"}) {
		t.Fatalf("rejected = %v", rejected)
	}
}

func TestRejectedFailsClosedWhenAValidatorCannotDecide(t *testing.T) {
	broken := errors.New("sheets down")
	r, err := NewRegistry(append(Builtin(), portRule{err: broken})...)
	if err != nil {
		t.Fatal(err)
	}
	catalog := NewCatalog([]Declaration{{Name: "planilha", Label: "P", Type: Type{Kind: "port"}}})
	if _, err := r.Rejected(context.Background(), "ws", catalog, map[string]string{"planilha": "x"}); !errors.Is(err, broken) {
		t.Fatalf("err = %v, want the port error", err)
	}
	r2, _ := NewRegistry(append(Builtin(), portRule{err: ErrValueInvalid})...)
	rejected, err := r2.Rejected(context.Background(), "ws", catalog, map[string]string{"planilha": "x"})
	if err != nil || !reflect.DeepEqual(rejected, []string{"planilha"}) {
		t.Fatalf("rejected = %v, err = %v", rejected, err)
	}
}

func TestRejectedRefusesADeclarationWhoseKindIsNotRegistered(t *testing.T) {
	r := builtinRegistry(t)
	catalog := NewCatalog([]Declaration{{Name: "x", Label: "X", Type: Type{Kind: "gone"}}})
	if _, err := r.Rejected(context.Background(), "ws", catalog, map[string]string{"x": "1"}); !errors.Is(err, ErrKindUnknown) {
		t.Fatalf("err = %v, want ErrKindUnknown", err)
	}
}

func TestCatalogLabels(t *testing.T) {
	catalog := NewCatalog([]Declaration{{Name: "matricula", Label: "Matrícula", Type: Type{Kind: KindText}}})
	if catalog.LabelOf("matricula") != "Matrícula" || catalog.LabelOf("nome_do_cliente") != "Nome do cliente" || catalog.LabelOf("x") != "X" {
		t.Fatalf("labels = %q %q", catalog.LabelOf("matricula"), catalog.LabelOf("nome_do_cliente"))
	}
	if catalog.LabelOf(KeyLeadName) != "Nome do contato" || SystemLabelKey(KeyCampaignName) != "conversation_campaign_name" {
		t.Fatalf("system label = %q", catalog.LabelOf(KeyLeadName))
	}
	if _, ok := catalog.Find("outra"); ok {
		t.Fatal("found an undeclared name")
	}
}

func TestSystemKeys(t *testing.T) {
	if !reflect.DeepEqual(SystemKeys(), []string{KeyLeadName, KeyLeadNumber, KeyCampaignName}) {
		t.Fatalf("system keys = %v", SystemKeys())
	}
	if !IsSystemKey("lead_number") || IsSystemKey("matricula") {
		t.Fatal("system key check")
	}
}

func TestCheckNameUsesTheDeclarationRule(t *testing.T) {
	if err := CheckName("matricula_2"); err != nil {
		t.Fatal(err)
	}
	if err := CheckName("campaign_name"); !errors.Is(err, ErrNameReserved) {
		t.Fatalf("err = %v", err)
	}
	if err := CheckName("a-b"); !errors.Is(err, ErrNameInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestEveryBuiltinTypeServesASampleItsOwnValidatorAccepts(t *testing.T) {
	r := builtinRegistry(t)
	types := []Type{{Kind: KindText}, {Kind: KindNumber}, {Kind: KindDate}, {Kind: KindChoice, Options: []string{"Manhã", "Tarde"}}}
	for _, typ := range types {
		rule, _ := r.Rule(typ.Kind)
		sample := rule.Sample(typ)
		if sample == "" {
			t.Fatalf("%s has no sample", typ.Kind)
		}
		if err := r.Accepts(context.Background(), Check{Name: "x", Type: typ, Value: sample}); err != nil {
			t.Fatalf("%s sample %q refused: %v", typ.Kind, sample, err)
		}
	}
	choice, _ := r.Rule(KindChoice)
	if choice.Sample(Type{Kind: KindChoice, Options: []string{"Manhã", "Tarde"}}) != "Manhã" {
		t.Fatal("a choice sample is its first option")
	}
}

func TestAnExampleMustFitTheTypeAndFallsBackToTheTypeSample(t *testing.T) {
	r := builtinRegistry(t)
	if _, err := r.CheckDeclaration(Declaration{Name: "valor", Label: "Valor", Example: "dez", Type: Type{Kind: KindNumber}}); !errors.Is(err, ErrExampleInvalid) {
		t.Fatalf("err = %v", err)
	}
	ok, err := r.CheckDeclaration(Declaration{Name: "valor", Label: "Valor", Example: " 1.250,00 ", Type: Type{Kind: KindNumber}})
	if err != nil || ok.Example != "1.250,00" {
		t.Fatalf("declaration = %+v, err = %v", ok, err)
	}
	if got := r.ExampleOf(ok); got != "1.250,00" {
		t.Fatalf("example = %q", got)
	}
	if got := r.ExampleOf(Declaration{Type: Type{Kind: KindDate}}); got == "" {
		t.Fatal("no type sample")
	}
	if got := r.ExampleOf(Declaration{Type: Type{Kind: "gone"}}); got != "" {
		t.Fatalf("unknown kind sample = %q", got)
	}
}

func TestSystemDataHasExamples(t *testing.T) {
	for _, name := range SystemKeys() {
		if SystemExample(name) == "" {
			t.Fatalf("%s has no example", name)
		}
	}
}
