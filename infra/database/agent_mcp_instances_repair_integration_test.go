package database

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func TestPostgresAgentMCPCollectionsBecomeOneSourceInstanceEachOnce(t *testing.T) {
	db := repotest.IsolatedDB(t, "agmcp", &schema.Agent{}, &schema.AgentTool{}, &schema.AgentToolRevision{},
		&schema.AgentMCPCollection{}, &schema.MCPCollection{}, &schema.MCPCollectionMember{}, &schema.MCPRemoteServer{})
	repotest.DropForeignKeys(t, db)
	ws, agentID, collection, remote := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.Create(&schema.Agent{ID: agentID, WorkspaceID: ws, Name: "A", MessagingPrompt: "p", Tags: "[]", Metadata: "{}", MediaIDs: "[]", Variables: "[]", ToolBindings: `{"bindings":[]}`}).Error)
	must(db.Create(&schema.AgentTool{ID: uuid.NewString(), WorkspaceID: ws, AgentID: agentID, Type: "http_request", Name: "mcp_github", Config: "{}", Visibility: "[]", Revision: 1, Position: 0}).Error)
	must(db.Create(&schema.MCPCollection{ID: collection, WorkspaceID: ws, Name: "Externos"}).Error)
	must(db.Create(&schema.MCPRemoteServer{ID: remote, WorkspaceID: ws, Name: "CRM Externo", URL: "https://crm.test/mcp", Transport: "http", Status: "connected"}).Error)
	must(db.Create(&schema.MCPCollectionMember{CollectionID: collection, Kind: "builtin", RefID: "github"}).Error)
	must(db.Create(&schema.MCPCollectionMember{CollectionID: collection, Kind: "remote", RefID: remote}).Error)
	must(db.Create(&schema.AgentMCPCollection{AgentID: agentID, CollectionID: collection}).Error)

	for run := 0; run < 2; run++ {
		if err := copyAgentMCPInstances(db); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}
	var copied []schema.AgentTool
	db.Where("agent_id = ? AND type = ?", agentID, "mcp_source").Order("position").Find(&copied)
	if len(copied) != 2 {
		t.Fatalf("copied %+v", copied)
	}
	if copied[0].Name != "mcp_github_2" || copied[0].Position != 1 || copied[0].TargetKey != "target:builtin:github" {
		t.Fatalf("builtin %+v", copied[0])
	}
	if copied[1].Name != "mcp_crm_externo" || copied[1].TargetKey != "target:remote:"+remote || copied[1].Description == "" {
		t.Fatalf("remote %+v", copied[1])
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(copied[1].Config), &config); err != nil || config["source_id"] != "remote:"+remote || config["all_tools"] != true {
		t.Fatalf("config %s", copied[1].Config)
	}
	var revisions int64
	db.Model(&schema.AgentToolRevision{}).Where("agent_id = ? AND type = ?", agentID, "mcp_source").Count(&revisions)
	if revisions != 2 {
		t.Fatalf("revisions %d", revisions)
	}
	var links int64
	db.Model(&schema.AgentMCPCollection{}).Where("agent_id = ?", agentID).Count(&links)
	if links != 1 {
		t.Fatalf("the legacy link stays for a rollback: %d", links)
	}
}
