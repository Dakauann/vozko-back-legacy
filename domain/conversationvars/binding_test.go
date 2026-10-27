package conversationvars

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"vozko/domain/campaign"
	"vozko/domain/customfield"
	"vozko/domain/lead"
)

func bindingDefs() []*customfield.Definition {
	return []*customfield.Definition{
		{Key: "matricula", ObjectType: customfield.ObjectLead, Type: customfield.TypeText},
		{Key: "cpf", ObjectType: customfield.ObjectLead, Type: customfield.TypeText, Sensitive: true},
	}
}

func refusedNames(t *testing.T, err error, cause error) []string {
	t.Helper()
	var refusal *VariablesError
	if !errors.As(err, &refusal) || !errors.Is(err, cause) {
		t.Fatalf("err = %v, want %v with the variables", err, cause)
	}
	return refusal.Names
}

func TestAssignRefusesBindingsNoConversationCouldResolve(t *testing.T) {
	cases := []struct {
		name     string
		bindings []Binding
		want     error
		names    []string
	}{
		{"nothing", nil, ErrNothingToSave, nil},
		{"a name with spaces", []Binding{{Name: "nome cliente", Source: campaign.BindLiteral, Value: "Ana"}}, ErrNameInvalid, nil},
		{"a name too long", []Binding{{Name: strings.Repeat("a", 65), Source: campaign.BindLiteral, Value: "Ana"}}, ErrNameInvalid, nil},
		{"a system key", []Binding{{Name: "lead_name", Source: campaign.BindLiteral, Value: "Ana"}}, ErrNameReserved, nil},
		{"the same name twice", []Binding{{Name: "turma", Source: SourceCurrent}, {Name: "turma", Source: campaign.BindLiteral, Value: "A"}}, ErrNameInvalid, nil},
		{"an unknown source", []Binding{{Name: "turma", Source: "lead.signo"}}, ErrSourceInvalid, []string{"turma"}},
		{"an unknown lead field", []Binding{{Name: "turma", Source: campaign.CustomBinding("turma")}}, ErrSourceInvalid, []string{"turma"}},
		{"a sensitive lead field", []Binding{{Name: "doc", Source: campaign.CustomBinding("cpf")}}, ErrSourceInvalid, []string{"doc"}},
		{"a value on a lead field", []Binding{{Name: "turma", Source: campaign.BindFirstName, Value: "x"}}, ErrSourceInvalid, []string{"turma"}},
		{"a value on the current value", []Binding{{Name: "turma", Source: SourceCurrent, Value: "x"}}, ErrSourceInvalid, []string{"turma"}},
		{"a blank fixed value", []Binding{{Name: "turma", Source: campaign.BindLiteral, Value: "  "}, {Name: "escola", Source: campaign.BindLiteral}}, ErrValueEmpty, []string{"turma", "escola"}},
		{"a fixed value too long", []Binding{{Name: "turma", Source: campaign.BindLiteral, Value: strings.Repeat("é", MaxValueRunes+1)}}, ErrValueInvalid, []string{"turma"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Assign(tc.bindings, bindingDefs())
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.names != nil && !reflect.DeepEqual(refusedNames(t, err, tc.want), tc.names) {
				t.Fatalf("names = %v, want %v", refusedNames(t, err, tc.want), tc.names)
			}
		})
	}
}

func TestResolveReadsLeadFieldsFixedTextAndTheCurrentValue(t *testing.T) {
	a, err := Assign([]Binding{
		{Name: " nome_cliente ", Source: campaign.BindFirstName},
		{Name: "matricula", Source: campaign.CustomBinding("matricula")},
		{Name: "escola", Source: campaign.BindLiteral, Value: " Prisma "},
		{Name: "turma", Source: SourceCurrent},
	}, bindingDefs())
	if err != nil {
		t.Fatal(err)
	}
	if !a.Plan().NeedsLead() || !reflect.DeepEqual(a.Names(), []string{"nome_cliente", "matricula", "escola", "turma"}) {
		t.Fatalf("names = %v", a.Names())
	}
	subject := campaign.BindingSubject{Lead: &lead.Lead{Name: "Dakauann Souza", CustomFields: map[string]any{"matricula": "2026-17"}}}
	got, err := a.Resolve(subject, map[string]string{"turma": "B", "nome_cliente": "da campanha"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"nome_cliente": "Dakauann", "matricula": "2026-17", "escola": "Prisma"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stored = %v, want %v; the current value stays where it is", got, want)
	}
}

func TestResolveRefusesEverySourceThatComesOutEmpty(t *testing.T) {
	a, err := Assign([]Binding{
		{Name: "nome_cliente", Source: campaign.BindNickname},
		{Name: "matricula", Source: campaign.CustomBinding("matricula")},
		{Name: "turma", Source: SourceCurrent},
		{Name: "escola", Source: campaign.BindLiteral, Value: "Prisma"},
	}, bindingDefs())
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Resolve(campaign.BindingSubject{Lead: &lead.Lead{Name: "Ana"}}, map[string]string{"turma": "  "})
	if names := refusedNames(t, err, ErrValueEmpty); !reflect.DeepEqual(names, []string{"nome_cliente", "matricula", "turma"}) {
		t.Fatalf("names = %v", names)
	}
	if _, err := a.Resolve(campaign.BindingSubject{}, map[string]string{"turma": "B"}); !errors.Is(err, ErrValueEmpty) {
		t.Fatalf("a conversation without a lead has no lead fields: %v", err)
	}
}

func TestResolveRefusesALeadValueTooLongToStore(t *testing.T) {
	a, err := Assign([]Binding{{Name: "matricula", Source: campaign.CustomBinding("matricula")}}, bindingDefs())
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Resolve(campaign.BindingSubject{Lead: &lead.Lead{CustomFields: map[string]any{"matricula": strings.Repeat("9", MaxValueRunes+1)}}}, nil)
	if names := refusedNames(t, err, ErrValueInvalid); !reflect.DeepEqual(names, []string{"matricula"}) {
		t.Fatalf("names = %v", names)
	}
}

func TestOnlyCurrentValuesNeedNothingStored(t *testing.T) {
	a, err := Assign([]Binding{{Name: "turma", Source: SourceCurrent}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Plan().NeedsLead() {
		t.Fatal("keeping the current value reads no lead")
	}
	got, err := a.Resolve(campaign.BindingSubject{}, map[string]string{"turma": "B"})
	if err != nil || len(got) != 0 {
		t.Fatalf("stored = %v, %v", got, err)
	}
}

func TestLiteralBindingsCarryTheOldValuesMapInNameOrder(t *testing.T) {
	got := LiteralBindings(map[string]string{"turma": "B", "escola": "Prisma"})
	want := []Binding{{Name: "escola", Source: campaign.BindLiteral, Value: "Prisma"}, {Name: "turma", Source: campaign.BindLiteral, Value: "B"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bindings = %+v", got)
	}
}
