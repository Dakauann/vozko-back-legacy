package agent_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	toolsdomain "vozko/domain/tools"
	"vozko/domain/workspace"
)

type instanceRegistry struct{ toolsdomain.Service }

func (instanceRegistry) Definitions() []toolsdomain.Definition {
	return []toolsdomain.Definition{
		{Name: "http_request", Description: "HTTP", Instances: toolsdomain.Many(), ConfigSchema: map[string]toolsdomain.ConfigParameter{
			"url": {Type: "string"}, "method": {Type: "string"}, "headers": {Type: "object", Sensitive: true},
		}},
		{Name: "run_workflow", Instances: toolsdomain.PerTarget("workflow_id"), ConfigSchema: map[string]toolsdomain.ConfigParameter{"workflow_id": {Type: "string"}}, RequiredConfig: []string{"workflow_id"}},
		{Name: "manage_lead_memory"},
	}
}

func (instanceRegistry) Handler(string) (toolsdomain.Handler, bool) { return nil, false }

type revisionedStore struct {
	*agentStore
	revisions map[string]int
}

func (s *revisionedStore) Update(id string, a *agent.Agent) error {
	for toolID, want := range a.ExpectedTools {
		if s.revisions[toolID] != want {
			return agenttool.ErrRevisionConflict
		}
	}
	for i, b := range a.InternalTools {
		s.revisions[b.ID]++
		a.InternalTools[i].Revision = s.revisions[b.ID]
	}
	return s.agentStore.Update(id, a)
}

func toolsFixture() (agent.AgentToolsUseCase, *revisionedStore) {
	base := &agentStore{byID: map[string]*agent.Agent{
		"ag-1": {ID: "ag-1", WorkspaceID: "ws1", Name: "Ana", MessagingPrompt: "p", MessagingModel: "m", Provider: agent.AgentProviderAI, InternalTools: []agent.ToolBinding{
			{ID: "t-mem", Name: "manage_lead_memory", Function: "manage_lead_memory", Revision: 1},
		}},
	}}
	store := &revisionedStore{agentStore: base, revisions: map[string]int{"t-mem": 1}}
	update := NewUpdateAgentUseCase(store, instanceRegistry{}, nil)
	validated(update)
	update.(ToolSecretSealed).SealToolSecretsWith(newTestSealer(&vault{values: map[string]string{}}))
	return NewAgentToolsUseCase(store, update, instanceRegistry{}, nil), store
}

var someone = workspace.Requester{UserID: "u-1"}

func httpDraft(description string) agent.ToolDraft {
	return agent.ToolDraft{Type: "http_request", Description: description, Config: map[string]interface{}{"url": "https://a.test/orders", "method": "GET"}}
}

func TestCreatingTwoHTTPToolsKeepsBothNamedAfterTheType(t *testing.T) {
	uc, store := toolsFixture()
	first, err := uc.Create(context.Background(), "ag-1", httpDraft("Consultar status do pedido"), someone)
	if err != nil {
		t.Fatal(err)
	}
	second, err := uc.Create(context.Background(), "ag-1", httpDraft("Criar ticket"), someone)
	if err != nil {
		t.Fatal(err)
	}
	if first.Function != "http_request" || second.Function != "http_request_2" || first.ID == second.ID {
		t.Fatalf("first %+v second %+v", first, second)
	}
	if got := store.byID["ag-1"].InternalTools; len(got) != 3 {
		t.Fatalf("stored %+v", got)
	}
}

func TestAnHTTPToolNeedsADescription(t *testing.T) {
	uc, _ := toolsFixture()
	if _, err := uc.Create(context.Background(), "ag-1", httpDraft("  "), someone); !errors.Is(err, agenttool.ErrDescriptionRequired) {
		t.Fatalf("err %v", err)
	}
}

func TestUpdatingWithAStaleRevisionIsRefused(t *testing.T) {
	uc, _ := toolsFixture()
	created, err := uc.Create(context.Background(), "ag-1", httpDraft("Consultar pedido"), someone)
	if err != nil {
		t.Fatal(err)
	}
	edit := agent.ToolEdit{Name: "pedido", Description: "Consulta.", Config: created.Config, Revision: created.Revision + 5}
	if _, err := uc.Update(context.Background(), "ag-1", created.ID, edit, someone); !errors.Is(err, agenttool.ErrRevisionConflict) {
		t.Fatalf("err %v", err)
	}
	edit.Revision = created.Revision
	renamed, err := uc.Update(context.Background(), "ag-1", created.ID, edit, someone)
	if err != nil || renamed.Function != "pedido" || renamed.ID != created.ID {
		t.Fatalf("renamed %+v err %v", renamed, err)
	}
}

func TestDeletingAndReorderingWorkOnIDs(t *testing.T) {
	uc, store := toolsFixture()
	created, _ := uc.Create(context.Background(), "ag-1", httpDraft("Consultar pedido"), someone)
	order, err := uc.Reorder(context.Background(), "ag-1", []string{created.ID, "t-mem"}, someone)
	if err != nil || order[0].ID != created.ID || order[1].ID != "t-mem" {
		t.Fatalf("order %+v err %v", order, err)
	}
	if _, err := uc.Reorder(context.Background(), "ag-1", []string{created.ID}, someone); !errors.Is(err, agenttool.ErrOrderMismatch) {
		t.Fatalf("err %v", err)
	}
	if err := uc.Delete(context.Background(), "ag-1", created.ID, someone); err != nil {
		t.Fatal(err)
	}
	if got := store.byID["ag-1"].InternalTools; len(got) != 1 || got[0].ID != "t-mem" {
		t.Fatalf("stored %+v", got)
	}
	if err := uc.Delete(context.Background(), "ag-1", "ghost", someone); !errors.Is(err, agenttool.ErrToolNotFound) {
		t.Fatalf("err %v", err)
	}
}

func TestDuplicatingAnHTTPToolCopiesItUnderAFreeName(t *testing.T) {
	uc, _ := toolsFixture()
	created, _ := uc.Create(context.Background(), "ag-1", httpDraft("Consultar pedido"), someone)
	copied, err := uc.Duplicate(context.Background(), "ag-1", created.ID, someone)
	if err != nil || copied.Function != "http_request_2" || copied.ID == created.ID || copied.Config["url"] != "https://a.test/orders" {
		t.Fatalf("copied %+v err %v", copied, err)
	}
	if _, err := uc.Duplicate(context.Background(), "ag-1", "t-mem", someone); !errors.Is(err, agenttool.ErrSingleTypeTaken) {
		t.Fatalf("err %v", err)
	}
}

func TestCreatedHeaderCredentialsAreSealed(t *testing.T) {
	uc, _ := toolsFixture()
	draft := httpDraft("Consultar pedido")
	draft.Config["headers"] = map[string]interface{}{"Authorization": "Bearer live"}
	created, err := uc.Create(context.Background(), "ag-1", draft, someone)
	if err != nil {
		t.Fatal(err)
	}
	if got := created.Config["headers"].(map[string]interface{})["Authorization"]; got == "Bearer live" {
		t.Fatalf("stored a plain credential: %v", got)
	}
}

func TestValidatingADraftSetChecksItWithoutSavingIt(t *testing.T) {
	uc, store := toolsFixture()
	v := &blockingValidator{}
	uc.(ToolValidated).ValidateToolsWith(v, nil)
	draft := []agent.ToolBinding{{ID: "t-mem", Name: "manage_lead_memory"}, {Name: "http_request", Description: "Consulta.", Config: map[string]interface{}{"url": "http://a.test", "method": "GET"}}}
	report, err := uc.(agenttool.ToolsValidation).Validate(context.Background(), "ag-1", draft)
	if err != nil || report.IsGreen() || len(store.byID["ag-1"].InternalTools) != 1 {
		t.Fatalf("report %+v err %v stored %+v", report, err, store.byID["ag-1"].InternalTools)
	}
	if len(v.seenStored) != 1 || v.seenStored[0].ID != "t-mem" {
		t.Fatalf("stored seen %+v", v.seenStored)
	}
	if _, err := uc.(agenttool.ToolsValidation).Validate(context.Background(), "ag-1", []agent.ToolBinding{{Name: "teleport"}}); !errors.Is(err, agenttool.ErrTypeUnknown) {
		t.Fatalf("err %v", err)
	}
}
