package workflow_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/googlesheet"
	"vozko/domain/workflow"
	"vozko/usecases/workflow/node_executors"
)

const (
	paymentsSheetID = "pagamentos"
	dealsSheetID    = "negociacoes"
)

type sheetsByID struct {
	sheets map[string]*googlesheet.Sheet
	err    error
}

func (s sheetsByID) Get(_ context.Context, workspaceID, id string) (*googlesheet.Sheet, error) {
	if s.err != nil {
		return nil, s.err
	}
	sheet, ok := s.sheets[id]
	if !ok || sheet.WorkspaceID != workspaceID {
		return nil, googlesheet.ErrSheetNotFound
	}
	return sheet, nil
}

func linkedWorkbook() map[string]*googlesheet.Sheet {
	return map[string]*googlesheet.Sheet{
		dealsSheetID: {ID: dealsSheetID, WorkspaceID: "ws1", Title: "Cobrança", TabName: "Negociações", Columns: []googlesheet.Column{
			{Key: "mensagem", Header: "Mensagem", Type: googlesheet.TypeLongText, Source: googlesheet.SourceMessageText},
		}},
		paymentsSheetID: {ID: paymentsSheetID, WorkspaceID: "ws1", Title: "Cobrança", TabName: "Pagamentos", Columns: []googlesheet.Column{
			{Key: "valor", Header: "Valor", Type: googlesheet.TypeText, Source: googlesheet.SourceInput},
			{Key: "negociacao", Header: "Negociação", Type: googlesheet.TypeLink, Source: googlesheet.SourceExportLink, LinkSheetID: dealsSheetID, LinkLabel: "Ver negociação"},
		}},
	}
}

func sheetStepCatalog() []workflow.NodeDefinition {
	return []workflow.NodeDefinition{
		node_executors.NewAddSheetRowExecutor(nil).Definition(),
		node_executors.NewExportMessagesSheetExecutor(nil, nil).Definition(),
	}
}

func paymentDraft(exportFirst bool) *workflow.Workflow {
	row := workflow.Node{ID: "n8", Type: workflow.NodeTypeActionAddSheetRow, Config: map[string]interface{}{"sheet_id": paymentsSheetID, "values": map[string]interface{}{"valor": "10"}}}
	export := workflow.Node{ID: "n7", Type: workflow.NodeTypeActionExportMessagesSheet, Config: map[string]interface{}{"sheet_id": dealsSheetID, "values": map[string]interface{}{}}}
	first, second := row, export
	if exportFirst {
		first, second = export, row
	}
	return &workflow.Workflow{WorkspaceID: "ws1", Type: workflow.WorkflowTypeMessages, Graph: workflow.Graph{
		Nodes: []workflow.Node{{ID: "t", Type: workflow.NodeTypeTriggerMessageReceived, Config: map[string]interface{}{}}, first, second, {ID: "end", Type: workflow.NodeTypeEnd}},
		Edges: []workflow.Edge{
			{Source: "t", Target: first.ID},
			{Source: first.ID, Target: second.ID, Label: workflow.SheetStepWritten},
			{Source: second.ID, Target: "end", Label: workflow.SheetStepWritten},
		},
	}}
}

func sheetLintUseCase(sheets GoogleSheetLookup) *activateWorkflowUseCase {
	return &activateWorkflowUseCase{repo: NewMockWorkflowRepository(), catalogFn: sheetStepCatalog, googleSheets: sheets}
}

func issuesCoded(issues []workflow.LintIssue, code workflow.LintIssueCode) []workflow.LintIssue {
	var out []workflow.LintIssue
	for _, i := range issues {
		if i.Code == code {
			out = append(out, i)
		}
	}
	return out
}

func TestTheLintBlocksARowWhoseLinkedTabIsExportedOnlyAfterIt(t *testing.T) {
	report, err := sheetLintUseCase(sheetsByID{sheets: linkedWorkbook()}).Lint(context.Background(), paymentDraft(false))
	if err != nil {
		t.Fatal(err)
	}
	missing := issuesCoded(report.Blocking(), workflow.LintSheetLinkExportMissing)
	if len(missing) != 1 || missing[0].NodeID != "n8" {
		t.Fatalf("issues = %+v", report.Issues)
	}
	if missing[0].Values["column"] != "Negociação" || missing[0].Values["tab"] != "Negociações" || missing[0].Values["tab_id"] != dealsSheetID {
		t.Fatalf("values = %v, want the column and the tab named from the sheet", missing[0].Values)
	}
}

func TestPublishingARowThatCanNeverLinkIsRefused(t *testing.T) {
	uc := sheetLintUseCase(sheetsByID{sheets: linkedWorkbook()})
	err := uc.validateRunnable(context.Background(), paymentDraft(false))
	blocked, ok := workflow.NotPublishableOf(err)
	if !ok || len(issuesCoded(blocked.Issues, workflow.LintSheetLinkExportMissing)) != 1 {
		t.Fatalf("err = %v, want the missing export to block publishing", err)
	}
}

func TestTheLintAcceptsARowAfterTheExportOfItsLinkedTab(t *testing.T) {
	report, err := sheetLintUseCase(sheetsByID{sheets: linkedWorkbook()}).Lint(context.Background(), paymentDraft(true))
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []workflow.LintIssueCode{workflow.LintSheetLinkExportMissing, workflow.LintSheetLinkExportSomePaths} {
		if found := issuesCoded(report.Issues, code); len(found) != 0 {
			t.Fatalf("issues = %+v", found)
		}
	}
}

func TestARowWhoseLinkedTabWasDeletedIsAMappingProblem(t *testing.T) {
	sheets := linkedWorkbook()
	delete(sheets, dealsSheetID)
	v := &googleSheetValidator{sheets: sheetsByID{sheets: sheets}, workspaceID: "ws1"}
	err := v.Validate(&paymentDraft(true).Graph.Nodes[2])
	if !errors.Is(err, workflow.ErrNodeSheetMappingInvalid) || !errors.Is(err, googlesheet.ErrLinkTargetInvalid) {
		t.Fatalf("err = %v, want the deleted link target refused", err)
	}
	if len(v.links) != 0 {
		t.Fatalf("links = %v, want none recorded for a refused step", v.links)
	}
}

func TestTheValidatorRecordsTheLinkColumnsOfTheTabARowWrites(t *testing.T) {
	v := &googleSheetValidator{sheets: sheetsByID{sheets: linkedWorkbook()}, workspaceID: "ws1"}
	for _, n := range paymentDraft(true).Graph.Nodes {
		if err := v.Validate(&n); err != nil {
			t.Fatalf("%s: %v", n.ID, err)
		}
	}
	want := workflow.SheetLink{Column: "negociacao", Header: "Negociação", TargetID: dealsSheetID, TargetTitle: "Negociações"}
	if links := v.links[paymentsSheetID]; len(links) != 1 || links[0] != want {
		t.Fatalf("links = %+v, want %+v", v.links, want)
	}
	if _, recorded := v.links[dealsSheetID]; recorded {
		t.Fatalf("links = %+v, an export tab is not a row tab", v.links)
	}
}

func TestAnUnreadableSheetStopsTheLinkCheckInsteadOfPassingIt(t *testing.T) {
	report, err := sheetLintUseCase(sheetsByID{err: errors.New("db down")}).Lint(context.Background(), paymentDraft(false))
	if err != nil {
		t.Fatal(err)
	}
	if len(issuesCoded(report.Blocking(), workflow.LintValidationUnavailable)) == 0 {
		t.Fatalf("issues = %+v, want the unreadable sheet to block", report.Issues)
	}
}
