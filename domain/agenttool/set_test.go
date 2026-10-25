package agenttool

import (
	"errors"
	"fmt"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/tools"
)

func setRules() SetRules {
	n := 0
	return SetRules{
		Definitions: map[string]tools.Definition{
			"http_request":       {Name: "http_request", Description: "Chama uma API.", Instances: tools.Many()},
			"run_workflow":       {Name: "run_workflow", Description: "Executa um fluxo.", Instances: tools.PerTarget("workflow_id")},
			"transfer_to_human":  {Name: "transfer_to_human", Description: "Transfere.", Instances: tools.PerTarget("department_id")},
			"manage_lead_memory": {Name: "manage_lead_memory", Description: "Memórias."},
		},
		Reserved: func(name string) bool { return name == "builtin_crm__buscar" },
		NewID: func() string {
			n++
			return fmt.Sprintf("new-%d", n)
		},
	}
}

func http(id, function, description, url string) agent.ToolBinding {
	return agent.ToolBinding{ID: id, Name: "http_request", Function: function, Description: description, Config: map[string]interface{}{"url": url, "method": "GET"}}
}

func flow(id, workflow string) agent.ToolBinding {
	return agent.ToolBinding{ID: id, Name: "run_workflow", Config: map[string]interface{}{"workflow_id": workflow}}
}

func TestTwoHTTPInstancesWithTheirOwnNamesAreKept(t *testing.T) {
	got, err := PrepareSet(setRules(), nil, []agent.ToolBinding{
		http("", "", "Consultar status do pedido", "https://a.test/orders"),
		http("", "", "Criar ticket de suporte", "https://a.test/tickets"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "new-1" || got[0].Function != "consultar_status_do_pedido" || got[1].Function != "criar_ticket_de_suporte" {
		t.Fatalf("got %+v", got)
	}
}

func TestBindingsWithoutIDsKeepTheirStoredIdentityByTypeAndTarget(t *testing.T) {
	current := []agent.ToolBinding{
		{ID: "t-http", Name: "http_request", Function: "http_request", Description: "Chama uma API.", Revision: 3, Config: map[string]interface{}{"url": "https://old"}},
		{ID: "t-wf-a", Name: "run_workflow", Function: "run_workflow_wf-a", Description: "Executa um fluxo.", Config: map[string]interface{}{"workflow_id": "wf-a"}},
		{ID: "t-mem", Name: "manage_lead_memory", Function: "manage_lead_memory", Description: "Memórias."},
	}
	got, err := PrepareSet(setRules(), current, []agent.ToolBinding{
		flow("", "wf-b"),
		http("", "", "", "https://new"),
		flow("", "wf-a"),
		{Name: "manage_lead_memory"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{got[0].ID, got[1].ID, got[2].ID, got[3].ID}
	if ids[0] != "new-1" || ids[1] != "t-http" || ids[2] != "t-wf-a" || ids[3] != "t-mem" {
		t.Fatalf("ids %v", ids)
	}
	if got[0].Function != "run_workflow_wf-b" || got[1].Function != "http_request" || got[1].Description != "Chama uma API." {
		t.Fatalf("got %+v", got)
	}
}

func TestAnUnknownIDIsRefused(t *testing.T) {
	_, err := PrepareSet(setRules(), nil, []agent.ToolBinding{http("ghost", "x", "d", "https://a")})
	if !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("err %v", err)
	}
}

func TestPoliciesAreEnforced(t *testing.T) {
	cases := []struct {
		name string
		next []agent.ToolBinding
		want error
	}{
		{"single twice", []agent.ToolBinding{{Name: "manage_lead_memory"}, {Name: "manage_lead_memory", Function: "outra_memoria"}}, ErrSingleTypeTaken},
		{"same target twice", []agent.ToolBinding{flow("", "wf-a"), {Name: "run_workflow", Function: "outro", Config: map[string]interface{}{"workflow_id": "wf-a"}}}, ErrTargetTaken},
		{"same name twice", []agent.ToolBinding{http("", "pedido", "a", "https://a"), http("", "pedido", "b", "https://b")}, ErrNameTaken},
		{"name of a single type", []agent.ToolBinding{http("", "manage_lead_memory", "a", "https://a")}, ErrNameTaken},
		{"reserved mcp name", []agent.ToolBinding{http("", "builtin_crm__buscar", "a", "https://a")}, ErrNameInvalid},
		{"invalid new name", []agent.ToolBinding{http("", "Pedido-Status", "a", "https://a")}, ErrNameInvalid},
		{"unknown type", []agent.ToolBinding{{Name: "teleport"}}, ErrTypeUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := PrepareSet(setRules(), nil, tc.next); !errors.Is(err, tc.want) {
				t.Fatalf("err %v want %v", err, tc.want)
			}
		})
	}
}

func TestALegacyNameIsAcceptedOnlyWhenItIsAlreadyStored(t *testing.T) {
	current := []agent.ToolBinding{{ID: "t-wf", Name: "run_workflow", Function: "run_workflow_wf-a", Config: map[string]interface{}{"workflow_id": "wf-a"}}}
	if _, err := PrepareSet(setRules(), current, []agent.ToolBinding{{ID: "t-wf", Name: "run_workflow", Function: "run_workflow_wf-a", Config: map[string]interface{}{"workflow_id": "wf-a"}}}); err != nil {
		t.Fatalf("stored legacy name refused: %v", err)
	}
	if _, err := PrepareSet(setRules(), current, []agent.ToolBinding{{ID: "t-wf", Name: "run_workflow", Function: "run-workflow-x", Config: map[string]interface{}{"workflow_id": "wf-a"}}}); !errors.Is(err, ErrNameInvalid) {
		t.Fatalf("err %v", err)
	}
}

func TestAnyDepartmentIsATargetOfItsOwn(t *testing.T) {
	_, err := PrepareSet(setRules(), nil, []agent.ToolBinding{
		{Name: "transfer_to_human", Function: "transferir_qualquer"},
		{Name: "transfer_to_human", Function: "transferir_vendas", Config: map[string]interface{}{"department_id": "d-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = PrepareSet(setRules(), nil, []agent.ToolBinding{
		{Name: "transfer_to_human", Function: "a"},
		{Name: "transfer_to_human", Function: "b"},
	})
	if !errors.Is(err, ErrTargetTaken) {
		t.Fatalf("err %v", err)
	}
}

func TestANewInstanceWithoutADescriptionIsNamedAfterItsType(t *testing.T) {
	got, err := PrepareSet(setRules(), nil, []agent.ToolBinding{http("", "", "", "https://a")})
	if err != nil || got[0].Function != "http_request" || got[0].Description != "" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestTargetKeysLetTheDatabaseEnforceThePolicies(t *testing.T) {
	got, err := PrepareSet(setRules(), nil, []agent.ToolBinding{
		{Name: "manage_lead_memory"},
		flow("", "wf-a"),
		http("", "", "", "https://a"),
		{Name: "transfer_to_human", Function: "transferir"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].TargetKey != "single" || got[1].TargetKey != "target:wf-a" || got[2].TargetKey != "" || got[3].TargetKey != "target:" {
		t.Fatalf("keys %q %q %q %q", got[0].TargetKey, got[1].TargetKey, got[2].TargetKey, got[3].TargetKey)
	}
}

func TestTheSetNormalizesConfigsThroughTheRules(t *testing.T) {
	rules := setRules()
	rules.Normalize = func(def tools.Definition, config map[string]interface{}) (map[string]interface{}, error) {
		if def.Name == "http_request" {
			return map[string]interface{}{"normalized": true}, nil
		}
		return config, nil
	}
	got, err := PrepareSet(rules, nil, []agent.ToolBinding{http("", "pedido", "d", "https://a")})
	if err != nil || got[0].Config["normalized"] != true {
		t.Fatalf("got %+v err %v", got, err)
	}
}
