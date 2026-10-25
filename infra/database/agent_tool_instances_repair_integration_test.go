package database

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func TestPostgresTheToolInstanceRepairCopiesOnceAndKeepsModelFacingNames(t *testing.T) {
	db := repotest.IsolatedDB(t, "agrepair", &schema.Agent{}, &schema.AgentTool{}, &schema.AgentToolRevision{})
	legacy, untouched, empty := uuid.NewString(), uuid.NewString(), uuid.NewString()
	ws := uuid.NewString()
	row := func(id, bindings string) *schema.Agent {
		return &schema.Agent{ID: id, WorkspaceID: ws, Name: "A", MessagingPrompt: "p", Tags: "[]", Metadata: "{}", MediaIDs: "[]", Variables: "[]", ToolBindings: bindings}
	}
	for _, a := range []*schema.Agent{
		row(legacy, `{"bindings":[{"name":"http_request","config":{"url":"https://a.test"}},{"name":"run_workflow","config":{"workflow_id":"wf-a"}}]}`),
		row(untouched, `{"bindings":[{"name":"http_request","config":{"url":"https://b.test"}}]}`),
		row(empty, `{"bindings":[]}`),
	} {
		if err := db.Create(a).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&schema.AgentTool{ID: uuid.NewString(), WorkspaceID: ws, AgentID: untouched, Type: "http_request", Name: "pedido", Config: "{}", Visibility: "[]", Revision: 1}).Error; err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		if err := copyAgentToolInstances(db); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}
	var copied []schema.AgentTool
	db.Where("agent_id = ?", legacy).Order("position").Find(&copied)
	if len(copied) != 2 || copied[0].Name != "http_request" || copied[1].Name != "run_workflow_wf-a" || copied[1].TargetKey != "target:wf-a" {
		t.Fatalf("copied %+v", copied)
	}
	var revisions, others int64
	db.Model(&schema.AgentToolRevision{}).Where("agent_id = ?", legacy).Count(&revisions)
	db.Model(&schema.AgentTool{}).Where("agent_id = ?", untouched).Count(&others)
	if revisions != 2 || others != 1 {
		t.Fatalf("revisions %d, rows of an agent that already had tools %d", revisions, others)
	}
}

func TestPostgresLegacyHTTPConfigsBecomeCanonicalOnceAndTheMirrorFollows(t *testing.T) {
	db := repotest.IsolatedDB(t, "aghttp", &schema.Agent{}, &schema.AgentTool{}, &schema.AgentToolRevision{})
	ws, agentID := uuid.NewString(), uuid.NewString()
	if err := db.Create(&schema.Agent{ID: agentID, WorkspaceID: ws, Name: "A", MessagingPrompt: "p", Tags: "[]", Metadata: "{}", MediaIDs: "[]", Variables: "[]",
		ToolBindings: `{"bindings":[{"name":"http_request","config":{"url":"https://a.test","method":"GET","headers":{"Accept":"json"}}}]}`}).Error; err != nil {
		t.Fatal(err)
	}
	if err := copyAgentToolInstances(db); err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		if err := canonicalizeAgentHTTPConfigs(db); err != nil {
			t.Fatal(err)
		}
	}
	var tool schema.AgentTool
	db.Where("agent_id = ?", agentID).First(&tool)
	if strings.Contains(tool.Config, "headers") || !strings.Contains(tool.Config, `"fields"`) || tool.Revision != 1 {
		t.Fatalf("tool %+v", tool)
	}
	var a schema.Agent
	db.First(&a, "id = ?", agentID)
	if !strings.Contains(a.ToolBindings, `"fields"`) {
		t.Fatalf("mirror %s", a.ToolBindings)
	}
}
