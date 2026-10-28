package automationneeds

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"vozko/domain/campaign"
	"vozko/domain/conversationvars"
	"vozko/domain/shared"
)

var resolveRef = conversationvars.Ref{WorkspaceID: "ws", EntryID: "e1", EntryType: shared.EntryTypeWhatsApp}

func TestResolvingBindingsReadsTheLeadAndStoresNothing(t *testing.T) {
	f := newFormFakes()

	values, err := f.queries(t, false).ResolveValues(context.Background(), resolveRef, []conversationvars.Binding{
		{Name: "turma", Source: campaign.CustomBinding("turma")},
		{Name: "polo", Source: campaign.BindLiteral, Value: " Centro "},
	})

	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(values, map[string]string{"turma": "C", "polo": "Centro"}) {
		t.Fatalf("values = %v", values)
	}
	if f.store.calls != 0 {
		t.Fatalf("resolving stores nothing: %d", f.store.calls)
	}
}

func TestResolvingNamesTheVariableThatCameOutEmpty(t *testing.T) {
	f := newFormFakes()
	f.subjects.leads["lead-1"].CustomFields = map[string]any{}

	_, err := f.queries(t, false).ResolveValues(context.Background(), resolveRef, []conversationvars.Binding{{Name: "turma", Source: campaign.CustomBinding("turma")}})

	var named *conversationvars.VariablesError
	if !errors.Is(err, conversationvars.ErrValueEmpty) || !errors.As(err, &named) || !reflect.DeepEqual(named.Names, []string{"turma"}) {
		t.Fatalf("err = %v", err)
	}
}
