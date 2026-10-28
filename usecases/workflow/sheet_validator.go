package workflow_usecase

import (
	"context"
	"errors"
	"fmt"

	"vozko/domain/googlesheet"
	"vozko/domain/workflow"
	"vozko/usecases/workflow/node_executors"
)

type GoogleSheetLookup interface {
	Get(ctx context.Context, workspaceID, id string) (*googlesheet.Sheet, error)
}

type googleSheetValidator struct {
	sheets      GoogleSheetLookup
	workspaceID string
	links       workflow.SheetLinks
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
	if err := sheet.CheckMapping(sheetMapping(n.Config[node_executors.AddSheetRowValuesKey])); err != nil {
		return fmt.Errorf("%w: node %q: %v", workflow.ErrNodeSheetMappingInvalid, n.ID, err)
	}
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

func sheetMapping(raw interface{}) map[string]string {
	values, _ := raw.(map[string]interface{})
	mapping := make(map[string]string, len(values))
	for key, value := range values {
		text, ok := value.(string)
		if !ok {
			text = workflow.TemplateText(value)
		}
		mapping[key] = text
	}
	return mapping
}
