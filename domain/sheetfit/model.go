package sheetfit

import (
	"slices"
	"strings"

	"vozko/domain/googlesheet"
)

type UseKind string

const (
	UseAddRow   UseKind = "add_row"
	UseExport   UseKind = "export"
	UseAgentRow UseKind = "agent_row"
)

func UseKinds() []UseKind { return []UseKind{UseAddRow, UseExport, UseAgentRow} }

func (k UseKind) Valid() bool { return slices.Contains(UseKinds(), k) }

type Use struct {
	ModelSheetID string
	Kind         UseKind
	Mapping      map[string]string
}

type Model struct {
	Sheets []*googlesheet.Sheet
	Uses   []Use
}

func (m Model) sheet(id string) *googlesheet.Sheet {
	for _, sheet := range m.Sheets {
		if sheet.ID == id {
			return sheet
		}
	}
	return nil
}

func (m Model) spreadsheets() []string {
	var out []string
	for _, sheet := range m.Sheets {
		if !slices.Contains(out, sheet.SpreadsheetID) {
			out = append(out, sheet.SpreadsheetID)
		}
	}
	return out
}

func (m Model) Check() error {
	_, err := m.targets()
	return err
}

type target struct {
	model *googlesheet.Sheet
	uses  []Use
}

func (m Model) targets() ([]target, error) {
	if len(m.Uses) == 0 {
		return nil, refuse(CodeUsesRequired)
	}
	wanted := map[string][]Use{}
	var pending []string
	for _, use := range m.Uses {
		id := strings.TrimSpace(use.ModelSheetID)
		if id == "" || !use.Kind.Valid() {
			return nil, refuse(CodeUseInvalid)
		}
		if m.sheet(id) == nil {
			return nil, &Misfit{Code: CodeModelMissing, ModelSheetID: id}
		}
		if _, seen := wanted[id]; !seen {
			pending = append(pending, id)
		}
		if use.Kind == UseAgentRow {
			use = Use{ModelSheetID: id, Kind: UseAddRow, Mapping: agentMapping(m.sheet(id))}
		}
		wanted[id] = append(wanted[id], use)
	}
	for len(pending) > 0 {
		id := pending[0]
		pending = pending[1:]
		for _, col := range m.sheet(id).LinkColumns() {
			if m.sheet(col.LinkSheetID) == nil {
				return nil, &Misfit{Code: CodeModelMissing, ModelSheetID: col.LinkSheetID}
			}
			if _, seen := wanted[col.LinkSheetID]; !seen {
				wanted[col.LinkSheetID] = nil
				pending = append(pending, col.LinkSheetID)
			}
		}
	}
	var out []target
	for _, sheet := range m.Sheets {
		if uses, ok := wanted[sheet.ID]; ok {
			out = append(out, target{model: sheet, uses: uses})
		}
	}
	return out, nil
}

func (t target) linkTarget(all []target) bool {
	for _, other := range all {
		for _, col := range other.model.LinkColumns() {
			if col.LinkSheetID == t.model.ID {
				return true
			}
		}
	}
	return false
}

func agentMapping(sheet *googlesheet.Sheet) map[string]string {
	mapping := map[string]string{}
	for _, col := range sheet.InputColumns() {
		mapping[col.Key] = col.Header
	}
	return mapping
}

func WholeModel(sheets []*googlesheet.Sheet) Model {
	model := Model{Sheets: sheets}
	for _, sheet := range sheets {
		wrote := false
		if sheet.HasMessageColumns() {
			model.Uses = append(model.Uses, Use{ModelSheetID: sheet.ID, Kind: UseExport})
			wrote = true
		}
		if sheet.TakesInput() {
			model.Uses = append(model.Uses, Use{ModelSheetID: sheet.ID, Kind: UseAgentRow})
			wrote = true
		}
		if !wrote {
			model.Uses = append(model.Uses, Use{ModelSheetID: sheet.ID, Kind: UseAddRow})
		}
	}
	return model
}

func IsModelFile(model Model, file googlesheet.Spreadsheet) bool {
	return slices.Contains(model.spreadsheets(), file.ID)
}

type Workspace struct {
	Files   []googlesheet.Spreadsheet
	Account string
}

func (w Workspace) File(spreadsheetID string) (googlesheet.Spreadsheet, bool) {
	for _, file := range w.Files {
		if file.ID == spreadsheetID {
			return file, true
		}
	}
	return googlesheet.Spreadsheet{}, false
}

func (w Workspace) Model(spreadsheetID string) (Model, bool) {
	file, ok := w.File(spreadsheetID)
	if !ok {
		return Model{}, false
	}
	return Model{Sheets: file.Sheets}, true
}
