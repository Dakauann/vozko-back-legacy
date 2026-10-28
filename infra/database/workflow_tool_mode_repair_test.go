package database

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"vozko/domain/workflow"
)

const routeToolLeafGraph = `{"nodes":[{"id":"t","type":"trigger_webhook","config":{}},{"id":"agent","type":"action_ai_agent","config":{"custom_tools":[{"name":"anotar"}],"tool_mode":"route"}},{"id":"save","type":"action_set_variable","config":{}},{"id":"reply","type":"action_send_text","config":{}},{"id":"end","type":"end","config":{}}],"edges":[{"source":"t","target":"agent"},{"source":"agent","target":"save","label":"anotar"},{"source":"agent","target":"reply","label":"default"},{"source":"reply","target":"end"}]}`

func repairOutputs(n workflow.Node) ([]workflow.HandleDefinition, bool) {
	switch n.Type {
	case workflow.NodeTypeActionSetVariable, workflow.NodeTypeActionSendText:
		return nil, true
	}
	return nil, false
}

func retiredOf(t *testing.T, raw string) (string, string) {
	t.Helper()
	result, err := workflow.RetireToolModeJSON([]byte(raw), repairOutputs)
	if err != nil || !result.Changed {
		t.Fatalf("retire: %+v %v", result, err)
	}
	var g workflow.Graph
	if err := json.Unmarshal(result.Graph, &g); err != nil {
		t.Fatal(err)
	}
	return string(result.Graph), workflow.GraphHash(&g)
}

func TestTheToolModeRepairIsDeclaredAfterTheNodeTypeRename(t *testing.T) {
	names := repairNames()
	at := map[string]int{}
	for i, name := range names {
		at[name] = i
	}
	retire, declared := at["wf_retire_tool_mode"]
	if !declared || retire < at["wf_canonical_node_types"] {
		t.Fatalf("the tool mode repair must run at boot after legacy types are renamed: %v", names)
	}
}

func TestTheToolModeRepairRefusesToRunWithoutTheOutputCatalog(t *testing.T) {
	db, _, _ := sqlmockDB(t)
	if err := retireToolModes(nil)(db); !errors.Is(err, errNodeOutputsUnwired) {
		t.Fatalf("err = %v, want the boot to fail closed", err)
	}
}

func TestTheToolModeRepairRewritesOnlyReadableGraphsThatChange(t *testing.T) {
	db, mock, _ := sqlmockDB(t)
	repair := toolModeGraphRepair(repairOutputs)
	retired, hash := retiredOf(t, routeToolLeafGraph)

	mock.ExpectQuery(sqlOf(draftGraphsWithStructuredConfigSQL)).WithArgs(workflowVersionRepairFirstKey, repair.path, workflowGraphCanonicalBatch).
		WillReturnRows(sqlmock.NewRows([]string{"id", "graph"}).
			AddRow("wf-a", routeToolLeafGraph).
			AddRow("wf-b", retired).
			AddRow("wf-c", "{not json"))
	mock.ExpectExec(sqlOf(rewriteDraftGraphSQL)).WithArgs(retired, "wf-a", routeToolLeafGraph).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(sqlOf(draftGraphsWithStructuredConfigSQL)).WithArgs("wf-c", repair.path, workflowGraphCanonicalBatch).
		WillReturnRows(sqlmock.NewRows([]string{"id", "graph"}))
	mock.ExpectQuery(sqlOf(versionGraphsWithStructuredConfigSQL)).WithArgs(workflowVersionRepairFirstKey, repair.path, workflowGraphCanonicalBatch).
		WillReturnRows(sqlmock.NewRows([]string{"id", "graph"}).AddRow("v-a", routeToolLeafGraph))
	mock.ExpectExec(sqlOf(rewriteVersionGraphSQL)).WithArgs(retired, hash, "v-a", routeToolLeafGraph).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(sqlOf(versionGraphsWithStructuredConfigSQL)).WithArgs("v-a", repair.path, workflowGraphCanonicalBatch).
		WillReturnRows(sqlmock.NewRows([]string{"id", "graph"}))

	if err := repairStoredGraphs(db, repair, workflowGraphCanonicalBatch); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTheToolModeRepairAgainstPostgres(t *testing.T) {
	tx := postgresTx(t)
	schemaName := "wf_tool_mode_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	for _, ddl := range []string{
		`CREATE SCHEMA ` + schemaName,
		`SET LOCAL search_path TO ` + schemaName,
		`CREATE TABLE workflows (id uuid PRIMARY KEY, graph jsonb, published_version integer, deleted_at timestamptz)`,
		`CREATE TABLE workflow_versions (id uuid PRIMARY KEY, workflow_id uuid NOT NULL, version integer NOT NULL, graph jsonb NOT NULL, graph_hash varchar(64) NOT NULL)`,
	} {
		if err := tx.Exec(ddl).Error; err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	var before workflow.Graph
	if err := json.Unmarshal([]byte(routeToolLeafGraph), &before); err != nil {
		t.Fatal(err)
	}
	workflowID, versionID, brokenID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	broken := `{"nodes":[{"id":"agent","type":"action_ai_agent","config":{"tool_mode":"route"}},{"id":"x","type":"action_run_tool","config":{}}],"edges":[{"source":"agent","target":"x","label":"usar"}]}`
	for _, seed := range []struct {
		sql  string
		args []interface{}
	}{
		{`INSERT INTO workflows (id, graph, published_version) VALUES (?, ?::jsonb, 1)`, []interface{}{workflowID, routeToolLeafGraph}},
		{`INSERT INTO workflows (id, graph) VALUES (?, ?::jsonb)`, []interface{}{brokenID, broken}},
		{`INSERT INTO workflow_versions (id, workflow_id, version, graph, graph_hash) VALUES (?, ?, 1, ?::jsonb, ?)`, []interface{}{versionID, workflowID, routeToolLeafGraph, workflow.GraphHash(&before)}},
	} {
		if err := tx.Exec(seed.sql, seed.args...).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	repair := retireToolModes(repairOutputs)
	if err := repair(tx); err != nil {
		t.Fatalf("repair: %v", err)
	}
	readRaw := func(query, id string) string {
		var raw string
		if err := tx.Raw(query, id).Scan(&raw).Error; err != nil {
			t.Fatal(err)
		}
		return raw
	}
	draftAfterFirst := readRaw(`SELECT graph::text FROM workflows WHERE id = ?`, workflowID)
	versionAfterFirst := readRaw(`SELECT graph::text FROM workflow_versions WHERE id = ?`, versionID)
	if err := repair(tx); err != nil {
		t.Fatalf("a second run must be harmless: %v", err)
	}
	if readRaw(`SELECT graph::text FROM workflows WHERE id = ?`, workflowID) != draftAfterFirst ||
		readRaw(`SELECT graph::text FROM workflow_versions WHERE id = ?`, versionID) != versionAfterFirst {
		t.Fatal("the second run changed a repaired graph")
	}

	for name, raw := range map[string]string{"draft": draftAfterFirst, "version": versionAfterFirst} {
		var g workflow.Graph
		if err := json.Unmarshal([]byte(raw), &g); err != nil {
			t.Fatal(err)
		}
		end := g.FindNode("end_save")
		if end == nil || end.Type != workflow.NodeTypeEnd || len(g.OutgoingEdges("save")) != 1 || g.OutgoingEdges("save")[0].Target != "end_save" {
			t.Fatalf("%s: the open tool leaf did not get its end: %+v", name, g)
		}
		if _, carries := g.FindNode("agent").Config["tool_mode"]; carries {
			t.Fatalf("%s: tool_mode kept", name)
		}
	}
	var version workflow.Graph
	if err := json.Unmarshal([]byte(versionAfterFirst), &version); err != nil {
		t.Fatal(err)
	}
	var draft workflow.Graph
	if err := json.Unmarshal([]byte(draftAfterFirst), &draft); err != nil {
		t.Fatal(err)
	}
	var storedHash string
	tx.Raw(`SELECT graph_hash FROM workflow_versions WHERE id = ?`, versionID).Scan(&storedHash)
	if storedHash != workflow.GraphHash(&version) || storedHash != workflow.GraphHash(&draft) {
		t.Fatalf("hash = %s, the workflow must still read as published", storedHash)
	}
	if got := readRaw(`SELECT graph::text FROM workflows WHERE id = ?`, brokenID); !strings.Contains(got, `"tool_mode": "route"`) {
		t.Fatalf("a graph the repair cannot read must stay as stored: %s", got)
	}
}
