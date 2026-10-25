package agenttool_repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

const uniqueToolIndexes = `CREATE UNIQUE INDEX ux_agent_tools_name ON agent_tools (agent_id, lower(name)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX ux_agent_tools_target ON agent_tools (agent_id, type, target_key) WHERE deleted_at IS NULL AND target_key <> ''`

func toolDB(t *testing.T) (*gorm.DB, string, string) {
	t.Helper()
	db := repotest.IsolatedDB(t, "agtool", &schema.Agent{}, &schema.AgentTool{}, &schema.AgentToolRevision{})
	if err := db.Exec(uniqueToolIndexes).Error; err != nil {
		t.Fatal(err)
	}
	ws, agentID := uuid.NewString(), uuid.NewString()
	if err := db.Create(&schema.Agent{ID: agentID, WorkspaceID: ws, Name: "A", MessagingPrompt: "p", Tags: "[]", Metadata: "{}", MediaIDs: "[]", Variables: "[]", ToolBindings: `{"bindings":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	return db, ws, agentID
}

func sync(t *testing.T, db *gorm.DB, ws, agentID string, expected map[string]int, bindings ...agent.ToolBinding) error {
	t.Helper()
	return db.Transaction(func(tx *gorm.DB) error {
		return SyncTx(tx, SyncRequest{AgentID: agentID, WorkspaceID: ws, Actor: "u-1", Bindings: bindings, Expected: expected})
	})
}

func httpTool(id, name, url string) agent.ToolBinding {
	return agent.ToolBinding{ID: id, Name: "http_request", Function: name, Description: "d", Config: map[string]interface{}{"url": url}}
}

func TestPostgresSyncCreatesBumpsRevisionsOnlyOnChangeAndSoftDeletes(t *testing.T) {
	db, ws, agentID := toolDB(t)
	a, b := uuid.NewString(), uuid.NewString()
	if err := sync(t, db, ws, agentID, nil, httpTool(a, "pedido", "https://a"), httpTool(b, "ticket", "https://b")); err != nil {
		t.Fatal(err)
	}
	if err := sync(t, db, ws, agentID, nil, httpTool(b, "ticket", "https://b"), httpTool(a, "pedido", "https://a2")); err != nil {
		t.Fatal(err)
	}
	got, err := LoadBindings(db, agentID)
	if err != nil || len(got) != 2 || got[0].ID != b || got[0].Revision != 1 || got[1].ID != a || got[1].Revision != 2 {
		t.Fatalf("got %+v err %v", got, err)
	}
	var revisions int64
	db.Model(&schema.AgentToolRevision{}).Where("tool_id = ?", a).Count(&revisions)
	if revisions != 2 {
		t.Fatalf("revisions of a = %d", revisions)
	}
	if err := sync(t, db, ws, agentID, nil, httpTool(b, "ticket", "https://b")); err != nil {
		t.Fatal(err)
	}
	if got, _ := LoadBindings(db, agentID); len(got) != 1 {
		t.Fatalf("after delete %+v", got)
	}
	var mirrored schema.Agent
	db.First(&mirrored, "id = ?", agentID)
	if back := agent.DecodeLegacyBindings(mirrored.ToolBindings); len(back) != 1 || back[0].Config["url"] != "https://b" {
		t.Fatalf("mirror %s", mirrored.ToolBindings)
	}
}

func TestPostgresSyncRefusesAStaleRevision(t *testing.T) {
	db, ws, agentID := toolDB(t)
	a := uuid.NewString()
	if err := sync(t, db, ws, agentID, nil, httpTool(a, "pedido", "https://a")); err != nil {
		t.Fatal(err)
	}
	if err := sync(t, db, ws, agentID, map[string]int{a: 3}, httpTool(a, "pedido", "https://x")); !errors.Is(err, agenttool.ErrRevisionConflict) {
		t.Fatalf("err %v", err)
	}
}

func TestPostgresSyncSwapsTwoNamesInOneSave(t *testing.T) {
	db, ws, agentID := toolDB(t)
	a, b := uuid.NewString(), uuid.NewString()
	if err := sync(t, db, ws, agentID, nil, httpTool(a, "pedido", "https://a"), httpTool(b, "ticket", "https://b")); err != nil {
		t.Fatal(err)
	}
	if err := sync(t, db, ws, agentID, nil, httpTool(a, "ticket", "https://a"), httpTool(b, "pedido", "https://b")); err != nil {
		t.Fatalf("swap: %v", err)
	}
}

func TestPostgresTheUniqueIndexesRefuseDuplicateNamesAndTargets(t *testing.T) {
	db, ws, agentID := toolDB(t)
	if err := sync(t, db, ws, agentID, nil, httpTool(uuid.NewString(), "pedido", "https://a"), httpTool(uuid.NewString(), "PEDIDO", "https://b")); err == nil {
		t.Fatal("two names differing only in case must be refused")
	}
	flow := func(name string) agent.ToolBinding {
		return agent.ToolBinding{ID: uuid.NewString(), Name: "run_workflow", Function: name, TargetKey: "target:wf-a", Config: map[string]interface{}{"workflow_id": "wf-a"}}
	}
	if err := sync(t, db, ws, agentID, nil, flow("a"), flow("b")); err == nil {
		t.Fatal("two instances for the same workflow must be refused")
	}
}

func TestPostgresScannersSealInPlaceWithoutNewRevisions(t *testing.T) {
	db, ws, agentID := toolDB(t)
	a := uuid.NewString()
	plain := agent.ToolBinding{ID: a, Name: "http_request", Function: "pedido", Config: map[string]interface{}{"headers": map[string]interface{}{"Authorization": "Bearer x"}}}
	if err := sync(t, db, ws, agentID, nil, plain); err != nil {
		t.Fatal(err)
	}
	var saved []string
	live := NewLiveToolScanner(db, func(id string) { saved = append(saved, id) })
	rows, err := live.AgentToolsAfter(context.Background(), "00000000-0000-0000-0000-000000000000", 10)
	if err != nil || len(rows) != 1 || rows[0].WorkspaceID != ws {
		t.Fatalf("rows %+v err %v", rows, err)
	}
	sealed := plain
	sealed.Config = map[string]interface{}{"headers": map[string]interface{}{"Authorization": "{{secret.s}}"}}
	if err := live.SaveAgentTools(context.Background(), agentID, []agent.ToolBinding{sealed}); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadBindings(db, agentID)
	if got[0].Revision != 1 || got[0].Config["headers"].(map[string]interface{})["Authorization"] != "{{secret.s}}" || len(saved) != 1 {
		t.Fatalf("got %+v saved %v", got, saved)
	}
	revisions := NewRevisionScanner(db)
	history, err := revisions.AgentToolsAfter(context.Background(), "00000000-0000-0000-0000-000000000000", 10)
	if err != nil || len(history) != 1 || history[0].Bindings[0].Revision != 1 {
		t.Fatalf("history %+v err %v", history, err)
	}
	if err := revisions.SaveAgentTools(context.Background(), agentID, []agent.ToolBinding{{ID: a, Revision: 1, Config: sealed.Config}}); err != nil {
		t.Fatal(err)
	}
	var row schema.AgentToolRevision
	db.First(&row, "tool_id = ? AND revision = 1", a)
	var config map[string]map[string]string
	if err := json.Unmarshal([]byte(row.Config), &config); err != nil || config["headers"]["Authorization"] != "{{secret.s}}" {
		t.Fatalf("revision config %s", row.Config)
	}
}

func TestPostgresRevisionsAreListedNewestFirstAndReadOneByOne(t *testing.T) {
	db, ws, agentID := toolDB(t)
	a := uuid.NewString()
	_ = sync(t, db, ws, agentID, nil, httpTool(a, "pedido", "https://a"))
	_ = sync(t, db, ws, agentID, nil, httpTool(a, "pedido", "https://a2"))
	reader := NewRevisionReader(db)
	list, err := reader.ToolRevisions(context.Background(), agentID, a)
	if err != nil || len(list) != 2 || list[0].Revision != 2 || list[0].Config["url"] != "https://a2" || list[0].SavedBy != "u-1" {
		t.Fatalf("list %+v err %v", list, err)
	}
	first, err := reader.ToolRevision(context.Background(), agentID, a, 1)
	if err != nil || first.Config["url"] != "https://a" {
		t.Fatalf("first %+v err %v", first, err)
	}
	if _, err := reader.ToolRevision(context.Background(), agentID, a, 9); !errors.Is(err, agenttool.ErrRevisionNotFound) {
		t.Fatalf("err %v", err)
	}
	if _, err := reader.ToolRevisions(context.Background(), uuid.NewString(), a); !errors.Is(err, agenttool.ErrToolNotFound) {
		t.Fatalf("another agent must not read the history: %v", err)
	}
}

func TestPostgresToolNamesIncludeDeletedInstancesAndSkipUnknownIDs(t *testing.T) {
	db, ws, agentID := toolDB(t)
	a, b := uuid.NewString(), uuid.NewString()
	if err := sync(t, db, ws, agentID, nil, httpTool(a, "pedido", "https://a"), httpTool(b, "ticket", "https://b")); err != nil {
		t.Fatal(err)
	}
	if err := sync(t, db, ws, agentID, nil, httpTool(a, "pedido", "https://a")); err != nil {
		t.Fatal(err)
	}
	found, err := NewToolNames(db).FindToolsByIDs([]string{a, b, "draft", uuid.NewString()})
	if err != nil || len(found) != 2 {
		t.Fatalf("found %+v err %v", found, err)
	}
	names := map[string]string{}
	for _, tool := range found {
		names[tool.ID] = tool.Name + ":" + tool.Function
	}
	if names[a] != "http_request:pedido" || names[b] != "http_request:ticket" {
		t.Fatalf("names %+v", names)
	}
}
