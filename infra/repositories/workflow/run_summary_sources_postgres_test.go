package workflow_repository

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/workflow"
	"vozko/infra/database/schema"
)

func activeSeedWorkflow(t *testing.T, db *gorm.DB, workspaceID, name, status string, published, draft workflow.Graph, legacy string) string {
	t.Helper()
	id, one := uuid.NewString(), 1
	rows := []interface{}{
		&schema.WorkflowSchema{ID: id, WorkspaceID: workspaceID, Name: name, Status: status, Type: "messages", TriggerConfig: legacy, Graph: jsonOf(t, draft), Version: 2, PublishedVersion: &one},
		&schema.WorkflowVersionSchema{WorkflowID: id, Version: 1, Type: "messages", Graph: jsonOf(t, published), GraphHash: "h", PublishedAt: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create %T: %v", row, err)
		}
	}
	return id
}

func TestActiveTriggersReadOnlyThePublishedOverlappingTriggersOfActiveWorkflowsAgainstPostgres(t *testing.T) {
	db := postgresSchemaTx(t, &schema.WorkflowSchema{}, &schema.WorkflowVersionSchema{})
	workspace, other := uuid.NewString(), uuid.NewString()
	birthday := workflow.Node{ID: "bday", Type: workflow.NodeTypeTriggerBirthday, Config: map[string]interface{}{}}
	form := workflow.Node{ID: "form", Type: workflow.NodeTypeTriggerFormSubmitted, Config: map[string]interface{}{workflow.FormTriggerFormIDsKey: []interface{}{"f-1"}}}
	message := workflow.Node{ID: "msg", Type: workflow.NodeTypeTriggerMessageReceived}
	end := workflow.Node{ID: "end", Type: workflow.NodeTypeEnd}

	running := activeSeedWorkflow(t, db, workspace, "Parabéns", "active", workflow.Graph{Nodes: []workflow.Node{birthday, form, message, end}}, workflow.Graph{Nodes: []workflow.Node{message, end}}, "{}")
	legacy := activeSeedWorkflow(t, db, workspace, "Antigo", "active", workflow.Graph{Nodes: []workflow.Node{{ID: "form", Type: workflow.NodeTypeTriggerFormSubmitted}, end}}, workflow.Graph{}, `{"form_ids":["f-1"]}`)
	activeSeedWorkflow(t, db, workspace, "Só no rascunho", "active", workflow.Graph{Nodes: []workflow.Node{message, end}}, workflow.Graph{Nodes: []workflow.Node{birthday, end}}, "{}")
	activeSeedWorkflow(t, db, workspace, "Pausado", "paused", workflow.Graph{Nodes: []workflow.Node{birthday, end}}, workflow.Graph{}, "{}")
	activeSeedWorkflow(t, db, other, "Outra área", "active", workflow.Graph{Nodes: []workflow.Node{birthday, end}}, workflow.Graph{}, "{}")
	deleted := activeSeedWorkflow(t, db, workspace, "Excluído", "active", workflow.Graph{Nodes: []workflow.Node{birthday, end}}, workflow.Graph{}, "{}")
	if err := db.Exec(`UPDATE workflows SET deleted_at = now() WHERE id = ?`, deleted).Error; err != nil {
		t.Fatal(err)
	}

	got, err := NewEntryPointRepository(db).ActiveTriggers(workspace, workflow.OverlappingTriggerTypes())
	if err != nil {
		t.Fatal(err)
	}
	type seen struct{ workflow, name, node string }
	var rows []seen
	for _, trigger := range got {
		rows = append(rows, seen{trigger.WorkflowID, trigger.WorkflowName, trigger.Node.ID})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name+rows[i].node < rows[j].name+rows[j].node })
	want := []seen{{legacy, "Antigo", "form"}, {running, "Parabéns", "bday"}, {running, "Parabéns", "form"}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("active triggers = %+v\nwant %+v", rows, want)
	}
	for _, trigger := range got {
		if trigger.WorkflowID == legacy && !reflect.DeepEqual(trigger.LegacyConfig, map[string]interface{}{"form_ids": []interface{}{"f-1"}}) {
			t.Fatalf("legacy config = %+v", trigger.LegacyConfig)
		}
		if trigger.WorkflowID == running && trigger.Node.ID == "form" && !workflow.FormTriggerAccepts(trigger.Node.Config, "f-1") {
			t.Fatalf("the published form config was not decoded: %+v", trigger.Node)
		}
	}
	if none, err := NewEntryPointRepository(db).ActiveTriggers(" ", workflow.OverlappingTriggerTypes()); err != nil || len(none) != 0 {
		t.Fatalf("no workspace reads nothing: %+v, %v", none, err)
	}
}

func TestEntryPointSourcesNameTheChosenStagesAndLinesAgainstPostgres(t *testing.T) {
	db := postgresSchemaTx(t, append(entryPointTables(), &schema.Stage{}, &schema.SIPTrunk{})...)
	workspace, other := uuid.NewString(), uuid.NewString()
	workflowID := uuid.NewString()
	stage, foreignStage, deletedStage := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, row := range []*schema.Stage{
		{ID: stage, WorkspaceID: workspace, Name: "Pagamento recebido"},
		{ID: foreignStage, WorkspaceID: other, Name: "De outra área"},
		{ID: deletedStage, WorkspaceID: workspace, Name: "Removida"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`UPDATE stages SET deleted_at = now() WHERE id = ?`, deletedStage).Error; err != nil {
		t.Fatal(err)
	}
	trunk, offTrunk, foreignTrunk := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, row := range []struct {
		id, workspace, name string
		enabled             bool
	}{{trunk, workspace, "Linha central", true}, {offTrunk, workspace, "Linha reserva", false}, {foreignTrunk, other, "Linha alheia", true}} {
		if err := db.Exec(`INSERT INTO sip_trunks (id, workspace_id, name, trunk_type, host, port, transport, enabled, settings, created_at, updated_at)
			VALUES (?, ?, ?, 'register', 'sip.example.com', 5060, 'udp', ?, '{}', now(), now())`, row.id, row.workspace, row.name, row.enabled).Error; err != nil {
			t.Fatal(err)
		}
	}

	sources, err := NewEntryPointRepository(db).EntryPointSources(workflow.EntryPointSourceQuery{
		WorkspaceID: workspace, WorkflowIDs: []string{workflowID},
		StageIDs: []string{stage, foreignStage, deletedStage, "not-a-uuid"},
		TrunkIDs: []string{trunk, offTrunk, foreignTrunk},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sources.Stages, map[string]string{stage: "Pagamento recebido"}) {
		t.Fatalf("stages = %+v", sources.Stages)
	}
	wantTrunks := map[string]workflow.EntryTrunk{trunk: {Name: "Linha central", Enabled: true}, offTrunk: {Name: "Linha reserva"}}
	if !reflect.DeepEqual(sources.Trunks, wantTrunks) {
		t.Fatalf("trunks = %+v", sources.Trunks)
	}
}
