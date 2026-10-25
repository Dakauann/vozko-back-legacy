package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"vozko/domain/agent"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func TestPostgresPublishedAgentsPinTheirToolRevisionsAndNewAgentsStartAsDrafts(t *testing.T) {
	db := repotest.IsolatedDB(t, "agver", &schema.Agent{}, &schema.AgentTool{}, &schema.AgentToolRevision{}, &schema.AgentVersion{}, &schema.AgentKnowledgeBase{}, &schema.AgentMCPCollection{})
	repo := NewRepository(db).(*repository)
	ctx := context.Background()
	toolID := uuid.NewString()
	a := &agent.Agent{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), Name: "Ana", MessagingPrompt: "v1", MessagingModel: "m", Provider: agent.AgentProviderAI,
		InternalTools: []agent.ToolBinding{{ID: toolID, Name: "http_request", Function: "pedido", Config: map[string]interface{}{"url": "https://v1.test"}}}}
	if err := repo.Create(a); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindPublished(a.ID); !errors.Is(err, agent.ErrAgentNotPublished) {
		t.Fatalf("a new agent is a draft: %v", err)
	}
	if ids, _ := repo.UnpublishedAgentIDs(ctx, "00000000-0000-0000-0000-000000000000", 10); len(ids) != 0 {
		t.Fatalf("new drafts are not legacy agents: %v", ids)
	}
	draft, _ := repo.FindByID(a.ID)
	if _, err := repo.PublishVersion(ctx, a.ID, agent.SnapshotOf(draft), "u-1"); err != nil {
		t.Fatal(err)
	}
	draft.MessagingPrompt = "v2"
	draft.InternalTools[0].Config = map[string]interface{}{"url": "https://v2.test"}
	if err := repo.Update(a.ID, draft); err != nil {
		t.Fatal(err)
	}
	live, err := repo.FindPublished(a.ID)
	if err != nil || live.MessagingPrompt != "v1" || live.InternalTools[0].Config["url"] != "https://v1.test" || live.InternalTools[0].Revision != 1 || live.RunningVersion != 1 {
		t.Fatalf("live %+v err %v", live, err)
	}
	second, _ := repo.FindByID(a.ID)
	v2, err := repo.PublishVersion(ctx, a.ID, agent.SnapshotOf(second), "u-2")
	if err != nil || v2.Version != 2 {
		t.Fatalf("v2 %+v err %v", v2, err)
	}
	if live, _ := repo.FindPublished(a.ID); live.MessagingPrompt != "v2" || live.InternalTools[0].Config["url"] != "https://v2.test" || live.RunningVersion != 2 {
		t.Fatalf("after the second publish %+v", live)
	}
	history, _ := repo.Versions(ctx, a.ID)
	if len(history) != 2 || history[0].Version != 2 {
		t.Fatalf("history %+v", history)
	}
	legacy := uuid.NewString()
	if err := db.Create(&schema.Agent{ID: legacy, WorkspaceID: uuid.NewString(), Name: "Velho", MessagingPrompt: "p", Tags: "[]", Metadata: "{}", MediaIDs: "[]", Variables: "[]", ToolBindings: `{"bindings":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	if ids, _ := repo.UnpublishedAgentIDs(ctx, "00000000-0000-0000-0000-000000000000", 10); len(ids) != 1 || ids[0] != legacy {
		t.Fatalf("legacy agents %v", ids)
	}
}
