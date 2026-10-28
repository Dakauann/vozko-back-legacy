package automationtransfer

import (
	"errors"
	"reflect"
	"testing"

	"vozko/domain/automationneeds"
	"vozko/domain/conversationvars"
	"vozko/domain/sheetfit"
)

func withPlanilha(needs automationneeds.Needs, name string) automationneeds.Needs {
	return append(needs, automationneeds.Need{Name: name, SheetUses: []sheetfit.Use{{ModelSheetID: "m", Kind: sheetfit.UseAddRow}}})
}

func TestAPlanilhaPassesOnItsOwnAndIsNeverInformedByTheModel(t *testing.T) {
	cases := map[string]struct {
		destination Destination
		needs       automationneeds.Needs
		want        error
		names       []string
	}{
		"same name passes without a mapping":           {matricula(VariableSource{Name: "turma", Source: SourceAI}), withPlanilha(needsOf("turma"), "planilha"), nil, nil},
		"another name is mapped from the conversation": {matricula(VariableSource{Name: "planilha_destino", Source: conversationvars.SourceCurrent, Value: "planilha_franquia"}), withPlanilha(nil, "planilha_destino"), nil, nil},
		"the model never picks the file":               {matricula(VariableSource{Name: "planilha", Source: SourceAI}), withPlanilha(nil, "planilha"), ErrVariableSource, []string{"planilha"}},
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
