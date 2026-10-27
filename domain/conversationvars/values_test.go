package conversationvars

import (
	"reflect"
	"testing"
)

func TestMergeLayersCampaignStoredAndSystemValues(t *testing.T) {
	got := Merge(
		Lead{Name: "Ana Souza", Number: "5511900000001"},
		Campaign{Name: "Volta às aulas", Values: map[string]string{"escola": "Prisma", "turma": "A", "lead_name": "da planilha", "1": "Ana"}},
		map[string]string{"turma": "B", "valor": "10"},
	)
	want := map[string]string{
		"escola": "Prisma", "turma": "B", "valor": "10", "1": "Ana",
		"lead_name": "Ana Souza", "lead_number": "5511900000001", "campaign_name": "Volta às aulas",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge = %v\nwant %v", got, want)
	}
}

func TestMergeWithoutACampaignKeepsTheSystemKeys(t *testing.T) {
	got := Merge(Lead{Name: "Bia", Number: "@bia"}, Campaign{}, nil)
	want := map[string]string{"lead_name": "Bia", "lead_number": "@bia", "campaign_name": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge = %v", got)
	}
}

func TestCampaignValuesNumberTheTemplateVariablesAndFormatMetadata(t *testing.T) {
	got := CampaignValuesOf(map[string]any{"escola": "Prisma", "vagas": 3, "vazio": nil}, []string{"Ana", "1º ano"})
	want := map[string]string{"escola": "Prisma", "vagas": "3", "vazio": "", "1": "Ana", "2": "1º ano"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("values = %v", got)
	}
}

func TestCampaignVarsCarriesTheValuesForAWorkflowRun(t *testing.T) {
	got := CampaignVars(map[string]string{"escola": "Prisma"})
	if !reflect.DeepEqual(got, map[string]interface{}{"escola": "Prisma"}) {
		t.Fatalf("campvars = %v", got)
	}
}
