package platformtools

import (
	"context"
	"strings"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/copilot"
	"vozko/domain/lint"
	"vozko/domain/workspace"
)

type toolCheck struct {
	report agenttool.Report
	draft  []agent.ToolBinding
}

func (c *toolCheck) Validate(_ context.Context, _ string, draft []agent.ToolBinding) (agenttool.Report, error) {
	c.draft = draft
	return c.report, nil
}

func blockingCheck() *toolCheck {
	return &toolCheck{report: agenttool.Report{Issues: []agenttool.Issue{{Code: agenttool.CodeHTTPURLNotHTTPS, Severity: lint.SeverityBlocking, Message: "A URL de \"pedido\" usa http, sem criptografia"}}}}
}

func activeAgent() *agent.Agent {
	a := boundAgent()
	a.IsActive = true
	return a
}

func TestTheCopilotRefusesBeforeTheCardWhenTheActiveAgentWouldBreak(t *testing.T) {
	check := blockingCheck()
	tool := NewUpdateAgentTool(fakeGetAgent{a: activeAgent()}, &fakeUpdateAgent{}, AgentDeps{Catalog: httpCatalog(), ToolCheck: check})
	err := tool.(copilot.Validator).Validate(context.Background(), okContext(), httpBindingArgs(map[string]interface{}{"Accept": "json"}))
	if err == nil || !strings.Contains(err.Error(), "http, sem criptografia") {
		t.Fatalf("err = %v", err)
	}
	if len(check.draft) == 0 {
		t.Fatal("the check must see the merged tool set")
	}
}

func TestAnInactiveAgentCanBeChangedEvenWithIssues(t *testing.T) {
	tool := NewUpdateAgentTool(fakeGetAgent{a: boundAgent()}, &fakeUpdateAgent{}, AgentDeps{Catalog: httpCatalog(), ToolCheck: blockingCheck()})
	if err := tool.(copilot.Validator).Validate(context.Background(), okContext(), httpBindingArgs(map[string]interface{}{"Accept": "json"})); err != nil {
		t.Fatalf("err = %v", err)
	}
}

type refusingUpdate struct{}

func (refusingUpdate) Execute(context.Context, string, agent.UpdateAgentInput) (*agent.Agent, error) {
	return nil, agenttool.BlockedBy(agenttool.ErrNotActivatable, blockingCheck().report)
}

func TestABlockedApplyExplainsTheIssues(t *testing.T) {
	tool := NewUpdateAgentTool(fakeGetAgent{a: activeAgent()}, refusingUpdate{}, AgentDeps{Catalog: httpCatalog()})
	res := tool.Execute(context.Background(), okContext(), httpBindingArgs(map[string]interface{}{"Accept": "json"}))
	if res.Status != copilot.StatusError || !strings.Contains(res.Message, "http, sem criptografia") {
		t.Fatalf("res = %+v", res)
	}
}

type publishRecorder struct {
	ids []string
	err error
}

func (p *publishRecorder) Publish(_ context.Context, id string, _ workspace.Requester) (agent.Version, error) {
	p.ids = append(p.ids, id)
	return agent.Version{Version: 2}, p.err
}

func TestUpdatingWithPublishPublishesTheDraftAfterSaving(t *testing.T) {
	rec := &publishRecorder{}
	upd := &fakeUpdateAgent{}
	tool := NewUpdateAgentTool(fakeGetAgent{a: boundAgent()}, upd, AgentDeps{Catalog: httpCatalog(), Publishing: rec})
	args := map[string]interface{}{"id": "ag-1", "name": "Nova", "publish": true}
	res := tool.Execute(context.Background(), okContext(), args)
	if res.Status != copilot.StatusOK || len(rec.ids) != 1 || rec.ids[0] != "ag-1" || upd.calls != 1 {
		t.Fatalf("res %+v published %v", res, rec.ids)
	}
	delete(args, "publish")
	tool.Execute(context.Background(), okContext(), args)
	if len(rec.ids) != 1 {
		t.Fatal("without publish the draft stays unpublished")
	}
}

func TestABlockedPublishIsReportedAfterTheSave(t *testing.T) {
	rec := &publishRecorder{err: agenttool.BlockedBy(agenttool.ErrNotPublishable, blockingCheck().report)}
	tool := NewUpdateAgentTool(fakeGetAgent{a: boundAgent()}, &fakeUpdateAgent{}, AgentDeps{Catalog: httpCatalog(), Publishing: rec})
	res := tool.Execute(context.Background(), okContext(), map[string]interface{}{"id": "ag-1", "publish": true})
	if res.Status != copilot.StatusError || !strings.Contains(res.Message, "rascunho foi salvo") || !strings.Contains(res.Message, "http, sem criptografia") {
		t.Fatalf("res %+v", res)
	}
}
