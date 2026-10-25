package workflow_usecase

import (
	"context"
	"testing"

	"vozko/domain/workflow"
)

func plainHTTPGraph(url string) workflow.Graph {
	return workflow.Graph{
		Nodes: []workflow.Node{
			{ID: "t", Type: workflow.NodeTypeTriggerMessageReceived, Config: map[string]interface{}{}},
			{ID: "h", Type: workflow.NodeTypeActionHTTPRequest, Config: map[string]interface{}{"url": url, "method": "GET"}},
		},
	}
}

func TestTheLinterBlocksANewPlainHTTPNode(t *testing.T) {
	repo := NewMockWorkflowRepository()
	storedWorkflow(t, repo, workflow.WorkflowStatusDraft, plainHTTPGraph("http://api.test/x"))
	w, _ := repo.FindByID("wf-1")
	report, err := gate(repo).Lint(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range report.Issues {
		if issue.Code == workflow.LintHTTPURLNotHTTPS {
			found = issue.Severity == workflow.SeverityBlocking && issue.NodeID == "h"
		}
	}
	if !found {
		t.Fatalf("issues %+v", report.Issues)
	}
}
