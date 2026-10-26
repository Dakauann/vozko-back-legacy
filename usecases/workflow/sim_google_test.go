package workflow_usecase

import (
	"context"
	"strings"
	"testing"

	"vozko/domain/googlesheet"
	"vozko/domain/workflow"
	gsuc "vozko/usecases/googlesheet"
)

type simSheetSink struct {
	got []gsuc.EnqueueInput
}

func (s *simSheetSink) Enqueue(_ context.Context, in gsuc.EnqueueInput) (*gsuc.EnqueueResult, error) {
	s.got = append(s.got, in)
	return &gsuc.EnqueueResult{RowID: "row-1", Sheet: &googlesheet.Sheet{ID: in.SheetID, Title: "Teste planilhas"}}, nil
}

func simGoogleNodeContext(nodeType workflow.NodeType, config map[string]interface{}) *workflow.NodeContext {
	state := workflow.NewRunState()
	node := &workflow.Node{ID: "google-node", Type: nodeType, Config: config}
	graph := &workflow.Graph{
		Nodes: []workflow.Node{*node, {ID: "ok"}, {ID: "bad"}},
		Edges: []workflow.Edge{{Source: "google-node", Target: "ok", Label: "sucesso"}, {Source: "google-node", Target: "bad", Label: "erro"}},
	}
	run := &workflow.WorkflowRun{ID: "run-1", WorkflowID: "wf-1", WorkspaceID: "ws1", EntryID: "lead-1", EntryType: "whatsapp", CurrentNodeID: "google-node", State: state}
	return &workflow.NodeContext{Run: run, Node: node, Graph: graph, Workflow: &workflow.Workflow{ID: "wf-1", WorkspaceID: "ws1"}, State: &run.State}
}

func TestATestRunWritesTheSheetRowThroughTheWorkspaceIntegration(t *testing.T) {
	sink := &simSheetSink{}
	sandbox := newSandboxRunConfirming(ExecutorDeps{SheetRows: sink}, true)
	executor, _ := sandbox.registry.Get(workflow.NodeTypeActionAddSheetRow)

	result, err := executor.Execute(simGoogleNodeContext(workflow.NodeTypeActionAddSheetRow, map[string]interface{}{"sheet_id": "sheet-1", "values": map[string]interface{}{"mensagem": "Teste"}}))

	if err != nil || result.NextNodeID != "ok" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if len(sink.got) != 1 || sink.got[0].SheetID != "sheet-1" {
		t.Fatalf("enqueued = %+v", sink.got)
	}
}

func TestATestRunReachesTheWorkspaceGoogleCalendarAccount(t *testing.T) {
	repo := &calendarEventRepoMock{}
	sandbox := newSandboxRunConfirming(ExecutorDeps{CalendarRepo: repo, GoogleCalendar: &calendarEventGoogleMock{}, CalendarAccess: repo}, true)
	executor, _ := sandbox.registry.Get(workflow.NodeTypeActionCreateCalendarEvent)

	result, err := executor.Execute(simGoogleNodeContext(workflow.NodeTypeActionCreateCalendarEvent, map[string]interface{}{
		"title": "Reunião", "start_time": "2026-03-31T14:00:00-03:00", "duration": 30, "timezone": "America/Sao_Paulo",
	}))

	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if msg, _ := result.Output["error"].(string); strings.Contains(msg, "indisponível no servidor") {
		t.Fatalf("the test run lost the calendar integration: %q", msg)
	}
	if repo.lastWorkspaceID != "ws1" {
		t.Fatalf("calendar access asked for %q", repo.lastWorkspaceID)
	}
}
