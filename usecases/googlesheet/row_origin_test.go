package googlesheet

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

type workspaceNames struct {
	names map[string]string
	asked map[string]int
	err   error
}

func newWorkspaceNames(names map[string]string) *workspaceNames {
	return &workspaceNames{names: names, asked: map[string]int{}}
}

func (n *workspaceNames) lookup(_ context.Context, workspaceID, id string) (string, error) {
	n.asked[id]++
	if n.err != nil {
		return "", n.err
	}
	if workspaceID != testWorkspace {
		return "", nil
	}
	return n.names[id], nil
}

func (w *world) addRow(t *testing.T, key string, origin googlesheet.Origin) string {
	t.Helper()
	res, err := w.intake.Enqueue(context.Background(), EnqueueInput{
		WorkspaceID: testWorkspace, SheetID: w.sheet.ID, Values: map[string]any{"nome": key}, IdempotencyKey: key, Origin: origin,
	})
	if err != nil {
		t.Fatal(err)
	}
	return res.RowID
}

func (w *world) listedOrigins(t *testing.T) map[string]OriginView {
	t.Helper()
	page, err := w.manager.ListRows(context.Background(), testWorkspace, w.sheet.ID, shared.Pagination{Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]OriginView{}
	for _, item := range page.Items {
		out[item.Row.ID] = item.Origin
	}
	return out
}

func TestListedRowsSayWhatWroteThemAndLinkOnlyToNamedThingsOfTheWorkspace(t *testing.T) {
	w := newWorld(t)
	w.agents.names["agent-1"] = "Sofia"
	w.workflows.names["wf-1"] = "Cobrança"
	w.people.names["user-1"] = "Ana"
	byAgent := w.addRow(t, "agent", googlesheet.Origin{Kind: googlesheet.OriginAgent, AgentID: "agent-1", EntryID: "conv-1", EntryType: "whatsapp"})
	byRun := w.addRow(t, "run", googlesheet.Origin{Kind: googlesheet.OriginWorkflow, WorkflowID: "wf-1", RunID: "run-1", EntryID: "conv-2", EntryType: "instagram"})
	byLeadRun := w.addRow(t, "lead-run", googlesheet.Origin{Kind: googlesheet.OriginWorkflow, WorkflowID: "wf-1", RunID: "run-2", EntryID: "lead-1", EntryType: "lead", LeadID: "lead-1"})
	byForeignRun := w.addRow(t, "foreign-run", googlesheet.Origin{Kind: googlesheet.OriginWorkflow, WorkflowID: "wf-other", RunID: "run-3", EntryID: "conv-3", EntryType: "whatsapp"})
	byHand := w.addRow(t, "manual", googlesheet.Origin{Kind: googlesheet.OriginManual, ActorID: "user-1"})
	byElo := w.addRow(t, "elo", googlesheet.Origin{Kind: googlesheet.OriginCopilot, ActorID: "user-gone"})

	origins := w.listedOrigins(t)

	agent := origins[byAgent]
	if agent.Kind != googlesheet.OriginAgent || agent.Agent == nil || *agent.Agent != (NamedRef{ID: "agent-1", Name: "Sofia"}) ||
		agent.Conversation == nil || *agent.Conversation != (shared.EntryRef{EntryID: "conv-1", EntryType: shared.EntryTypeWhatsApp}) {
		t.Fatalf("agent origin = %+v", agent)
	}
	run := origins[byRun]
	if run.Workflow == nil || *run.Workflow != (NamedRef{ID: "wf-1", Name: "Cobrança"}) || run.RunID != "run-1" || run.Conversation == nil || run.Conversation.EntryID != "conv-2" {
		t.Fatalf("workflow origin = %+v", run)
	}
	if leadRun := origins[byLeadRun]; leadRun.Conversation != nil || leadRun.RunID != "run-2" {
		t.Fatalf("a run on a lead has no conversation: %+v", leadRun)
	}
	if foreign := origins[byForeignRun]; foreign.Workflow != nil || foreign.RunID != "" || foreign.Kind != googlesheet.OriginWorkflow {
		t.Fatalf("a workflow outside the workspace was exposed: %+v", foreign)
	}
	if manual := origins[byHand]; manual.Person == nil || *manual.Person != (NamedRef{ID: "user-1", Name: "Ana"}) || manual.Conversation != nil {
		t.Fatalf("manual origin = %+v", manual)
	}
	if elo := origins[byElo]; elo.Kind != googlesheet.OriginCopilot || elo.Person != nil {
		t.Fatalf("a person who is not a member was named: %+v", elo)
	}
	if w.workflows.asked["wf-1"] != 1 {
		t.Fatalf("each workflow is looked up once per page, asked %d", w.workflows.asked["wf-1"])
	}
}

func TestRowsAreNotListedWhenANameCannotBeRead(t *testing.T) {
	w := newWorld(t)
	w.addRow(t, "agent", googlesheet.Origin{Kind: googlesheet.OriginAgent, AgentID: "agent-1", EntryID: "conv-1", EntryType: "whatsapp"})
	w.agents.err = errors.New("database down")

	if _, err := w.manager.ListRows(context.Background(), testWorkspace, w.sheet.ID, shared.Pagination{Page: 1, PageSize: 25}); err == nil {
		t.Fatal("rows were listed without knowing who wrote them")
	}
}
