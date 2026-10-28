package sheetfit

import (
	"errors"
	"slices"
	"sort"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

type Assignment struct {
	SpreadsheetID string
	Tabs          map[string]string
}

func Fit(model Model, file googlesheet.Spreadsheet, account string) (Assignment, error) {
	targets, err := model.targets()
	if err != nil {
		return Assignment{}, err
	}
	switch {
	case len(file.Sheets) == 0:
		return Assignment{}, refuse(CodeFileNotFound)
	case IsModelFile(model, file):
		return Assignment{}, refuse(CodeModelFile)
	case account == "" || file.Oldest().AccountID != account:
		return Assignment{}, refuse(CodeFileOtherAccount)
	}
	f := newFitting(targets, file)
	tabs, err := f.assign()
	if err != nil {
		return Assignment{}, err
	}
	return Assignment{SpreadsheetID: file.ID, Tabs: tabs}, nil
}

type fitting struct {
	targets    []target
	file       googlesheet.Spreadsheet
	byID       map[string]*googlesheet.Sheet
	candidates map[string][]*googlesheet.Sheet
	refused    map[string]map[string]Problem
	assigned   map[string]string
	owner      map[string]string
}

func newFitting(targets []target, file googlesheet.Spreadsheet) *fitting {
	f := &fitting{
		targets: targets, file: file, byID: map[string]*googlesheet.Sheet{},
		candidates: map[string][]*googlesheet.Sheet{}, refused: map[string]map[string]Problem{},
		assigned: map[string]string{}, owner: map[string]string{},
	}
	for _, sheet := range file.Sheets {
		f.byID[sheet.ID] = sheet
	}
	for _, tg := range targets {
		f.refused[tg.model.ID] = map[string]Problem{}
		for _, sheet := range file.Sheets {
			if problem := f.check(tg, sheet); problem != nil {
				f.refused[tg.model.ID][sheet.ID] = *problem
				continue
			}
			f.candidates[tg.model.ID] = append(f.candidates[tg.model.ID], sheet)
		}
	}
	return f
}

func (f *fitting) assign() (map[string]string, error) {
	for {
		progress, err := f.narrowAll()
		if err != nil {
			return nil, err
		}
		if progress || f.settleByTitle() {
			continue
		}
		break
	}
	for _, tg := range f.targets {
		if _, done := f.assigned[tg.model.ID]; !done {
			misfit := &Misfit{Code: CodeTabAmbiguous, ModelSheetID: tg.model.ID, ModelTab: tg.model.Title}
			for _, sheet := range f.candidates[tg.model.ID] {
				misfit.Candidates = append(misfit.Candidates, Tab{SheetID: sheet.ID, Title: sheet.Title})
			}
			return nil, misfit
		}
	}
	if err := f.verifyLinks(); err != nil {
		return nil, err
	}
	return f.assigned, nil
}

func (f *fitting) narrowAll() (bool, error) {
	progress := false
	for _, tg := range f.targets {
		id := tg.model.ID
		if _, done := f.assigned[id]; done {
			continue
		}
		var live []*googlesheet.Sheet
		for _, sheet := range f.candidates[id] {
			if problem := f.conflict(tg, sheet); problem != nil {
				f.refused[id][sheet.ID] = *problem
				continue
			}
			live = append(live, sheet)
		}
		f.candidates[id] = live
		switch len(live) {
		case 0:
			return false, f.missing(tg)
		case 1:
			f.take(id, live[0].ID)
			progress = true
		}
	}
	return progress, nil
}

func (f *fitting) settleByTitle() bool {
	for _, tg := range f.targets {
		if _, done := f.assigned[tg.model.ID]; done {
			continue
		}
		var named []*googlesheet.Sheet
		for _, sheet := range f.candidates[tg.model.ID] {
			if sameTitle(sheet, tg.model) {
				named = append(named, sheet)
			}
		}
		if len(named) == 1 {
			f.take(tg.model.ID, named[0].ID)
			return true
		}
	}
	return false
}

func (f *fitting) take(modelID, sheetID string) {
	f.assigned[modelID] = sheetID
	f.owner[sheetID] = modelID
}

func (f *fitting) missing(tg target) *Misfit {
	misfit := &Misfit{Code: CodeTabShared, ModelSheetID: tg.model.ID, ModelTab: tg.model.Title}
	for _, sheet := range f.file.Sheets {
		problem, ok := f.refused[tg.model.ID][sheet.ID]
		if !ok {
			continue
		}
		misfit.Problems = append(misfit.Problems, problem)
		if problem.Code != CodeTabShared {
			misfit.Code = CodeTabMissing
		}
	}
	sort.SliceStable(misfit.Problems, func(i, j int) bool {
		return sameTitle(f.byID[misfit.Problems[i].SheetID], tg.model) && !sameTitle(f.byID[misfit.Problems[j].SheetID], tg.model)
	})
	return misfit
}

func (f *fitting) conflict(tg target, sheet *googlesheet.Sheet) *Problem {
	if other, taken := f.owner[sheet.ID]; taken && other != tg.model.ID {
		return &Problem{Code: CodeTabShared, SheetID: sheet.ID, Tab: sheet.Title, ModelTab: f.modelTitle(other)}
	}
	for _, col := range tg.model.LinkColumns() {
		own, _ := sheet.Column(col.Key)
		if !f.reachable(col.LinkSheetID, own.LinkSheetID) {
			return problemAt(CodeLinkMismatch, sheet, own.Header)
		}
	}
	for _, source := range f.targets {
		linking, assigned := f.byID[f.assigned[source.model.ID]]
		if !assigned {
			continue
		}
		for _, col := range source.model.LinkColumns() {
			own, _ := linking.Column(col.Key)
			if col.LinkSheetID == tg.model.ID && own.LinkSheetID != sheet.ID {
				return problemAt(CodeLinkMismatch, sheet, own.Header)
			}
		}
	}
	return nil
}

func (f *fitting) reachable(modelTarget, sheetTarget string) bool {
	if assigned, done := f.assigned[modelTarget]; done {
		return assigned == sheetTarget
	}
	return slices.ContainsFunc(f.candidates[modelTarget], func(s *googlesheet.Sheet) bool { return s.ID == sheetTarget })
}

func (f *fitting) verifyLinks() error {
	for _, tg := range f.targets {
		sheet := f.byID[f.assigned[tg.model.ID]]
		for _, col := range tg.model.LinkColumns() {
			own, _ := sheet.Column(col.Key)
			if own.LinkSheetID != f.assigned[col.LinkSheetID] {
				return &Misfit{Code: CodeTabMissing, ModelSheetID: tg.model.ID, ModelTab: tg.model.Title, Problems: []Problem{*problemAt(CodeLinkMismatch, sheet, own.Header)}}
			}
		}
	}
	return nil
}

func (f *fitting) modelTitle(id string) string {
	for _, tg := range f.targets {
		if tg.model.ID == id {
			return tg.model.Title
		}
	}
	return ""
}

func (f *fitting) check(tg target, sheet *googlesheet.Sheet) *Problem {
	for _, use := range tg.uses {
		if problem := mappingProblem(tg.model, sheet, use.Mapping); problem != nil {
			return problem
		}
		if use.Kind == UseExport && !sheet.HasMessageColumns() {
			return problemAt(CodeNoMessageColumns, sheet, "")
		}
	}
	if tg.linkTarget(f.targets) && !sheet.HasMessageColumns() {
		return problemAt(CodeNoMessageColumns, sheet, "")
	}
	return f.linkProblem(tg.model, sheet)
}

func mappingProblem(model, sheet *googlesheet.Sheet, mapping map[string]string) *Problem {
	if err := sheet.CheckMapping(mapping); err != nil {
		return valueProblem(model, sheet, err)
	}
	for _, want := range model.Columns {
		if _, mapped := mapping[want.Key]; !mapped {
			continue
		}
		have, _ := sheet.Column(want.Key)
		if have.Type != want.Type {
			return problemAt(CodeColumnType, sheet, have.Header)
		}
		for _, option := range want.Options {
			if !hasOption(have.Options, option) {
				problem := problemAt(CodeColumnOptions, sheet, have.Header)
				problem.Option = option
				return problem
			}
		}
	}
	return nil
}

func valueProblem(model, sheet *googlesheet.Sheet, err error) *Problem {
	var value *googlesheet.ValueError
	if !errors.As(err, &value) {
		return problemAt(CodeColumnMissing, sheet, "")
	}
	switch {
	case errors.Is(err, googlesheet.ErrColumnArchived):
		return problemAt(CodeColumnArchived, sheet, value.Header)
	case errors.Is(err, googlesheet.ErrSystemColumn):
		return problemAt(CodeColumnSystem, sheet, value.Header)
	case errors.Is(err, googlesheet.ErrRequiredColumnMissing):
		return problemAt(CodeRequiredUnmapped, sheet, value.Header)
	}
	header := value.Header
	if col, ok := model.Column(value.Column); ok {
		header = col.Header
	}
	return problemAt(CodeColumnMissing, sheet, header)
}

func (f *fitting) linkProblem(model, sheet *googlesheet.Sheet) *Problem {
	modelLinks := model.LinkColumns()
	for _, want := range modelLinks {
		have, ok := sheet.Column(want.Key)
		if !ok || have.Archived || have.Source != googlesheet.SourceExportLink {
			return problemAt(CodeLinkMismatch, sheet, want.Header)
		}
		if err := googlesheet.CheckLinkTarget(sheet, have, f.byID[have.LinkSheetID]); err != nil {
			return problemAt(CodeLinkMismatch, sheet, have.Header)
		}
	}
	for _, have := range sheet.LinkColumns() {
		if !slices.ContainsFunc(modelLinks, func(want googlesheet.Column) bool { return want.Key == have.Key }) {
			return problemAt(CodeLinkMismatch, sheet, have.Header)
		}
	}
	return nil
}

func problemAt(code string, sheet *googlesheet.Sheet, column string) *Problem {
	return &Problem{Code: code, SheetID: sheet.ID, Tab: sheet.Title, Column: column}
}

func hasOption(options []string, want string) bool {
	folded := shared.FoldForMatch(want)
	return slices.ContainsFunc(options, func(option string) bool { return shared.FoldForMatch(option) == folded })
}

func sameTitle(sheet, model *googlesheet.Sheet) bool {
	return sheet != nil && shared.FoldForMatch(sheet.Title) == shared.FoldForMatch(model.Title)
}
