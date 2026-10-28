package automationtransfer

import (
	"errors"
	"reflect"
	"testing"

	"vozko/domain/automationneeds"
	"vozko/domain/campaign"
	"vozko/domain/conversationvars"
	"vozko/domain/customfield"
)

func needsOf(names ...string) automationneeds.Needs {
	out := automationneeds.Needs{}
	for _, name := range names {
		out = append(out, automationneeds.Need{Name: name, Description: "A " + name + " do aluno"})
	}
	return out
}

func matricula(variables ...VariableSource) Destination {
	return Destination{ID: "d1", Target: workflowTarget("wf-matricula"), Variables: variables}
}

var leadFields = []*customfield.Definition{{Key: "curso", Label: "Curso", Type: customfield.TypeText}}

func TestTheVariableMappingIsStoredWithTheDestination(t *testing.T) {
	config := destinationsConfig(map[string]interface{}{
		"id": "d1", "kind": "workflow", "automation_id": "wf-matricula",
		"variables": []interface{}{
			map[string]interface{}{"name": " turma ", "source": "ai", "description": " A turma que o aluno quer. "},
			map[string]interface{}{"name": "curso", "source": "lead.custom:curso"},
			map[string]interface{}{"name": "polo", "source": "literal", "value": "Centro"},
			map[string]interface{}{"name": "matricula", "source": "conversation"},
		},
	})

	normalized, err := Normalize(config, sequentialIDs())
	if err != nil {
		t.Fatal(err)
	}
	list, err := Parse(normalized)
	if err != nil {
		t.Fatal(err)
	}
	want := []VariableSource{
		{Name: "turma", Source: SourceAI, Description: "A turma que o aluno quer."},
		{Name: "curso", Source: "lead.custom:curso"},
		{Name: "polo", Source: campaign.BindLiteral, Value: "Centro"},
		{Name: "matricula", Source: conversationvars.SourceCurrent},
	}
	if !reflect.DeepEqual(list[0].Variables, want) {
		t.Fatalf("variables = %+v", list[0].Variables)
	}
	if got := list[0].AIVariables(); len(got) != 1 || got[0].Name != "turma" {
		t.Fatalf("ai variables = %+v", got)
	}
}

func TestAnUnreadableMappingIsRefused(t *testing.T) {
	for name, variables := range map[string]interface{}{
		"not a list":   "turma",
		"no name":      []interface{}{map[string]interface{}{"source": "ai"}},
		"no source":    []interface{}{map[string]interface{}{"name": "turma"}},
		"unknown key":  []interface{}{map[string]interface{}{"name": "turma", "source": "ai", "secret": "x"}},
		"repeated":     []interface{}{map[string]interface{}{"name": "turma", "source": "ai"}, map[string]interface{}{"name": "turma", "source": "conversation"}},
		"not text":     []interface{}{map[string]interface{}{"name": "turma", "source": 1}},
		"item not map": []interface{}{"turma"},
	} {
		config := destinationsConfig(map[string]interface{}{"id": "d1", "kind": "agent", "automation_id": "a", "variables": variables})
		if _, err := Parse(config); !errors.Is(err, ErrConfigUnreadable) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestTheMappingMustMatchWhatTheDestinationNeedsNow(t *testing.T) {
	cases := map[string]struct {
		destination Destination
		needs       automationneeds.Needs
		want        error
		names       []string
	}{
		"every need mapped": {matricula(VariableSource{Name: "turma", Source: SourceAI}, VariableSource{Name: "curso", Source: "lead.custom:curso"}), needsOf("turma", "curso"), nil, nil},
		"a new need":        {matricula(VariableSource{Name: "turma", Source: SourceAI}), needsOf("turma", "curso"), ErrVariableUnmapped, []string{"curso"}},
		"no longer needed":  {matricula(VariableSource{Name: "turma", Source: SourceAI}, VariableSource{Name: "velha", Source: SourceAI}), needsOf("turma"), ErrVariableNotNeeded, []string{"velha"}},
		"empty fixed text":  {matricula(VariableSource{Name: "turma", Source: campaign.BindLiteral}), needsOf("turma"), ErrVariableSource, []string{"turma"}},
		"unknown field":     {matricula(VariableSource{Name: "turma", Source: "lead.custom:sumiu"}), needsOf("turma"), ErrVariableSource, []string{"turma"}},
		"unknown source":    {matricula(VariableSource{Name: "turma", Source: "crystal_ball"}), needsOf("turma"), ErrVariableSource, []string{"turma"}},
		"nothing needed":    {matricula(), nil, nil, nil},
	}
	for name, c := range cases {
		err := c.destination.CheckMapping(c.needs, leadFields)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
			continue
		}
		var named *conversationvars.VariablesError
		if c.want != nil && (!errors.As(err, &named) || !reflect.DeepEqual(named.Names, c.names)) {
			t.Errorf("%s: names = %+v", name, named)
		}
	}
}

func TestTheModelFillsItsVariablesAndTheOthersKeepTheirSource(t *testing.T) {
	d := matricula(
		VariableSource{Name: "turma", Source: SourceAI},
		VariableSource{Name: "curso", Source: "lead.custom:curso"},
		VariableSource{Name: "velha", Source: SourceAI},
	)

	bindings, filled, err := d.Bindings(needsOf("turma", "curso"), map[string]string{"turma": " Noite "})

	if err != nil {
		t.Fatal(err)
	}
	wantBindings := []conversationvars.Binding{{Name: "turma", Source: campaign.BindLiteral, Value: "Noite"}, {Name: "curso", Source: "lead.custom:curso"}}
	if !reflect.DeepEqual(bindings, wantBindings) {
		t.Fatalf("bindings = %+v", bindings)
	}
	if !reflect.DeepEqual(filled, []Filled{{Name: "turma", Source: SourceAI}, {Name: "curso", Source: "lead.custom:curso"}}) {
		t.Fatalf("filled = %+v", filled)
	}
}

func TestAVariableTheModelLeftEmptyIsNamed(t *testing.T) {
	d := matricula(VariableSource{Name: "turma", Source: SourceAI}, VariableSource{Name: "turno", Source: SourceAI})

	_, _, err := d.Bindings(needsOf("turma", "turno"), map[string]string{"turma": "  "})

	var named *conversationvars.VariablesError
	if !errors.Is(err, ErrModelValueMissing) || !errors.As(err, &named) || !reflect.DeepEqual(named.Names, []string{"turma", "turno"}) {
		t.Fatalf("err = %v", err)
	}
}

func TestTheTimelineNamesFilledVariablesAndTheirSourceOnly(t *testing.T) {
	record := academicRecord()
	record.Filled = []Filled{{Name: "turma", Source: SourceAI}, {Name: "curso", Source: "lead.custom:curso"}}

	details := record.Details()

	if details["filled_variables"] != "turma=ai,curso=lead.custom:curso" {
		t.Fatalf("details = %v", details)
	}
	back, ok := RecordOf(details)
	if !ok || !reflect.DeepEqual(back.Filled, record.Filled) {
		t.Fatalf("back = %+v", back.Filled)
	}
}
