package workflow_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/googlesheet"
	"vozko/domain/workflow"
)

type sheetLookup struct{ sheet *googlesheet.Sheet }

func (l sheetLookup) Get(_ context.Context, workspaceID, id string) (*googlesheet.Sheet, error) {
	if l.sheet.WorkspaceID != workspaceID || l.sheet.ID != id {
		return nil, googlesheet.ErrSheetNotFound
	}
	return l.sheet, nil
}

func TestGoogleSheetValidatorChecksTheSheetAndItsMapping(t *testing.T) {
	sheet, err := googlesheet.NewSheet(googlesheet.NewSheetInput{
		WorkspaceID: "ws-1", AccountID: "a", Title: "Pedidos", TabName: "Pedidos",
		Columns: []googlesheet.ColumnInput{{Header: "Produto", Type: googlesheet.TypeText, Required: true}, {Header: "Obs", Type: googlesheet.TypeText}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sheet.ID = "sheet-1"
	v := &googleSheetValidator{sheets: sheetLookup{sheet: sheet}, workspaceID: "ws-1"}
	node := func(sheetID string, values map[string]interface{}) *workflow.Node {
		return &workflow.Node{ID: "n1", Type: workflow.NodeTypeActionAddSheetRow, Config: map[string]interface{}{"sheet_id": sheetID, "values": values}}
	}
	if err := v.Validate(node("sheet-1", map[string]interface{}{"produto": "{{var.produto}}"})); err != nil {
		t.Fatalf("a valid node was refused: %v", err)
	}
	if err := v.Validate(node("other", map[string]interface{}{"produto": "x"})); !errors.Is(err, workflow.ErrNodeInvalidSheetID) {
		t.Fatalf("unknown sheet err = %v", err)
	}
	if err := v.Validate(node("sheet-1", map[string]interface{}{"obs": "x"})); !errors.Is(err, workflow.ErrNodeSheetMappingInvalid) {
		t.Fatalf("unmapped required column err = %v", err)
	}
	if err := v.Validate(node("sheet-1", map[string]interface{}{"produto": "x", "cpf": "1"})); !errors.Is(err, workflow.ErrNodeSheetMappingInvalid) {
		t.Fatalf("unknown column err = %v", err)
	}
	if err := v.Validate(&workflow.Node{ID: "n2", Type: workflow.NodeTypeActionSendText}); err != nil {
		t.Fatalf("other nodes are not checked: %v", err)
	}
}

func TestGoogleSheetValidatorOnlyLetsAnExportWriteToASheetWithMessageColumns(t *testing.T) {
	plain, err := googlesheet.NewSheet(googlesheet.NewSheetInput{
		WorkspaceID: "ws-1", AccountID: "a", Title: "Pedidos",
		Columns: []googlesheet.ColumnInput{{Header: "Produto", Type: googlesheet.TypeText}},
	})
	if err != nil {
		t.Fatal(err)
	}
	plain.ID = "plain"
	timeline, err := googlesheet.NewSheet(googlesheet.NewSheetInput{
		WorkspaceID: "ws-1", AccountID: "a", Title: "Cobranças",
		Columns: []googlesheet.ColumnInput{
			{Header: "Mensagem", Type: googlesheet.TypeLongText, Source: googlesheet.SourceMessageText},
			{Header: "Valor", Type: googlesheet.TypeMoney, Required: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	timeline.ID = "timeline"
	export := func(sheetID string, values map[string]interface{}) *workflow.Node {
		return &workflow.Node{ID: "n1", Type: workflow.NodeTypeActionExportMessagesSheet, Config: map[string]interface{}{"sheet_id": sheetID, "values": values}}
	}
	onTimeline := &googleSheetValidator{sheets: sheetLookup{sheet: timeline}, workspaceID: "ws-1"}
	if err := onTimeline.Validate(export("timeline", map[string]interface{}{"valor": "{{var.valor}}"})); err != nil {
		t.Fatalf("a valid export was refused: %v", err)
	}
	if err := onTimeline.Validate(export("timeline", nil)); !errors.Is(err, workflow.ErrNodeSheetMappingInvalid) {
		t.Fatalf("an export missing a required column err = %v", err)
	}
	onPlain := &googleSheetValidator{sheets: sheetLookup{sheet: plain}, workspaceID: "ws-1"}
	if err := onPlain.Validate(export("plain", nil)); !errors.Is(err, workflow.ErrNodeSheetMappingInvalid) || !errors.Is(err, googlesheet.ErrNoMessageColumns) {
		t.Fatalf("an export to a sheet without message columns err = %v", err)
	}
}
