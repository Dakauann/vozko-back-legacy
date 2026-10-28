package automationtransfer

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"vozko/domain/stage"
)

func academicRecord() Record {
	return Record{
		From:          agentTarget("ag-prospeccao"),
		FromName:      "Agente Prospecção",
		To:            agentTarget("ag-academico"),
		ToName:        "Agente Acadêmico",
		DestinationID: "d1",
		Note:          "É aluno do polo Centro e quer a segunda via do boleto.",
		Stage:         stage.FunnelStage{StageID: "st-centro", StageName: "Atendimento", PipelineName: "Polo Centro"},
	}
}

func TestATransferRecordSurvivesTheTimelineDetails(t *testing.T) {
	record := academicRecord()

	details := record.Details()

	if details["from_actor_id"] != "ai:ag-prospeccao" || details["to_actor_id"] != "ai:ag-academico" || details["reason"] != record.Note {
		t.Fatalf("details = %v", details)
	}
	back, ok := RecordOf(details)
	if !ok || !reflect.DeepEqual(back, record) {
		t.Fatalf("back = %+v, ok = %v", back, ok)
	}
}

func TestAWorkflowTargetIsNamedAsAWorkflowOnTheTimeline(t *testing.T) {
	record := academicRecord()
	record.To, record.ToName, record.Stage = workflowTarget("wf-matricula"), "Matrícula", stage.FunnelStage{}

	details := record.Details()

	if details["to_actor_id"] != "workflow:wf-matricula" || details["to_kind"] != "workflow" {
		t.Fatalf("details = %v", details)
	}
	if _, has := details["stage_id"]; has {
		t.Fatalf("no stage means no stage keys: %v", details)
	}
}

func TestDetailsWithoutATargetAreNoRecord(t *testing.T) {
	if _, ok := RecordOf(map[string]string{"reason": "x"}); ok {
		t.Fatal("a record needs its target")
	}
}

func TestTheTargetReadsWhoSentTheConversationAndWhy(t *testing.T) {
	line := academicRecord().ContextLine(3 * time.Minute)

	for _, want := range []string{"Agente Prospecção", "3 min", "segunda via do boleto", "Polo Centro"} {
		if !strings.Contains(line, want) {
			t.Fatalf("missing %q in %q", want, line)
		}
	}
}

func TestTheLabelNamesTheFunnelAndStageWhenThereIsOne(t *testing.T) {
	if got := Label("Agente Acadêmico", stage.FunnelStage{StageName: "Atendimento", PipelineName: "Polo Centro"}); got != "Agente Acadêmico · Polo Centro · Atendimento" {
		t.Fatalf("got %q", got)
	}
	if got := Label("Agente Acadêmico", stage.FunnelStage{StageName: "Atendimento"}); got != "Agente Acadêmico · Atendimento" {
		t.Fatalf("got %q", got)
	}
	if got := Label("Matrícula", stage.FunnelStage{}); got != "Matrícula" {
		t.Fatalf("got %q", got)
	}
}
