package campaignaudience

import (
	"testing"

	"vozko/domain/campaign"
	"vozko/domain/customfield"
	"vozko/domain/lead"
)

func TestFieldsListTheFixedLeadFieldsThenTheBindableCustomOnes(t *testing.T) {
	defs := []*customfield.Definition{
		{Key: "serie", Label: "Série", ObjectType: customfield.ObjectLead, Type: customfield.TypeText},
		{Key: "cpf", Label: "CPF", ObjectType: customfield.ObjectLead, Type: customfield.TypeText, Sensitive: true},
	}
	got := FieldsOf(defs)
	if len(got) != len(campaign.BindableSources())+1 {
		t.Fatalf("fields = %+v", got)
	}
	if got[0].Source != campaign.BindFirstName || got[0].Kind != FieldLead || got[0].Column == "" {
		t.Fatalf("first = %+v", got[0])
	}
	last := got[len(got)-1]
	if last.Source != campaign.CustomBinding("serie") || last.Kind != FieldCustom || last.Key != "serie" || last.Label != "Série" || last.Type != customfield.TypeText {
		t.Fatalf("custom = %+v", last)
	}
}

func TestTheFieldPlanReadsEveryFieldOfOneLeadInOrder(t *testing.T) {
	defs := []*customfield.Definition{{Key: "serie", ObjectType: customfield.ObjectLead, Type: customfield.TypeText}}
	fields := FieldsOf(defs)
	plan, err := FieldPlan(fields, defs)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.NeedsAddresses() || !plan.NeedsOwnerNames() {
		t.Fatal("the field plan reads addresses and owner names")
	}
	values := plan.Values(campaign.BindingSubject{Lead: &lead.Lead{Name: "Ana Souza", CustomFields: map[string]any{"serie": "1º ano"}}, OwnerName: "Rui"})
	if len(values) != len(fields) || values[0] != "Ana" || values[len(values)-1] != "1º ano" {
		t.Fatalf("values = %v", values)
	}
}
