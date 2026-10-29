package workflow_usecase

import (
	"context"
	"errors"
	"fmt"

	"vozko/domain/automationneeds"
	"vozko/domain/googlesheet"
	"vozko/domain/workflow"
)

type GoogleSheetLookup interface {
	Get(ctx context.Context, workspaceID, id string) (*googlesheet.Sheet, error)
}

type googleSheetValidator struct {
	sheets       GoogleSheetLookup
	workspaceID  string
	links        workflow.SheetLinks
	columns      workflow.SheetChannelColumns
	declarations VariableRules
	voice        bool
}

func (v *googleSheetValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionAddSheetRow && n.Type != workflow.NodeTypeActionExportMessagesSheet {
		return nil
	}
	sheetID := workflow.SheetStepTab(n)
	if sheetID == "" {
		return nil
	}
	sheet, err := v.sheets.Get(context.Background(), v.workspaceID, sheetID)
	if errors.Is(err, googlesheet.ErrSheetNotFound) {
		return fmt.Errorf("%w: node %q sheet_id %q", workflow.ErrNodeInvalidSheetID, n.ID, sheetID)
	}
	if err != nil {
		return err
	}
	if err := v.checkVariable(n, sheet); err != nil {
		return err
	}
	if err := sheet.CheckMapping(workflow.SheetStepMapping(n)); err != nil {
		return fmt.Errorf("%w: node %q: %v", workflow.ErrNodeSheetMappingInvalid, n.ID, err)
	}
	v.recordChannelColumns(sheetID, sheet)
	if n.Type == workflow.NodeTypeActionExportMessagesSheet && !sheet.HasMessageColumns() {
		return fmt.Errorf("%w: node %q: %w", workflow.ErrNodeSheetMappingInvalid, n.ID, googlesheet.ErrNoMessageColumns)
	}
	if n.Type == workflow.NodeTypeActionAddSheetRow {
		return v.recordLinks(n.ID, sheetID, sheet)
	}
	return nil
}

func (v *googleSheetValidator) recordLinks(nodeID, sheetID string, sheet *googlesheet.Sheet) error {
	columns := sheet.LinkColumns()
	links := make([]workflow.SheetLink, 0, len(columns))
	for _, col := range columns {
		target, err := v.sheets.Get(context.Background(), v.workspaceID, col.LinkSheetID)
		if errors.Is(err, googlesheet.ErrSheetNotFound) {
			return fmt.Errorf("%w: node %q: coluna %q: %w", workflow.ErrNodeSheetMappingInvalid, nodeID, col.Header, googlesheet.ErrLinkTargetInvalid)
		}
		if err != nil {
			return err
		}
		links = append(links, workflow.SheetLink{Column: col.Key, Header: col.Header, TargetID: target.ID, TargetTitle: target.TabBaseName()})
	}
	if len(links) == 0 {
		return nil
	}
	if v.links == nil {
		v.links = workflow.SheetLinks{}
	}
	v.links[sheetID] = links
	return nil
}

func (v *googleSheetValidator) recordChannelColumns(sheetID string, sheet *googlesheet.Sheet) {
	columns := workflow.SheetChannelColumnsOf(sheet.Columns)
	if len(columns) == 0 {
		return
	}
	if v.columns == nil {
		v.columns = workflow.SheetChannelColumns{}
	}
	v.columns[sheetID] = columns
}

func (v *googleSheetValidator) checkVariable(n *workflow.Node, model *googlesheet.Sheet) error {
	name, set, err := workflow.SheetStepVariable(n)
	if err != nil || !set {
		return err
	}
	if v.voice {
		return fmt.Errorf("%w: node %q: ligações não trazem dados da conversa; use uma planilha fixa", workflow.ErrNodeSheetVariableInvalid, n.ID)
	}
	if v.declarations == nil {
		return fmt.Errorf("%w: node %q: os dados da conversa não puderam ser lidos", workflow.ErrValidationUnavailable, n.ID)
	}
	labeled := v.declarations.Label(context.Background(), v.workspaceID, automationneeds.Needs{{Name: name}})
	switch err := automationneeds.SheetModelProblem(labeled, model.SpreadsheetID); {
	case errors.Is(err, automationneeds.ErrSheetModelMismatch):
		return fmt.Errorf("%w: node %q: a aba %q não é da planilha modelo de %s", workflow.ErrNodeSheetVariableInvalid, n.ID, model.Title, labeled[0].Display())
	case err != nil:
		return fmt.Errorf("%w: node %q: %q não é um dado da conversa do tipo Planilha", workflow.ErrNodeSheetVariableInvalid, n.ID, name)
	}
	return nil
}
