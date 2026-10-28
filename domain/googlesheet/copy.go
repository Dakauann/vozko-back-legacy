package googlesheet

import (
	"fmt"
	"maps"
	"slices"
)

type SheetCopy struct {
	Columns  []ColumnInput
	LinkRefs map[int]string
	Unique   UniqueInput
}

func (s *Sheet) Copy() SheetCopy {
	out := SheetCopy{}
	positions := map[string]int{}
	for _, col := range s.Columns {
		if col.Archived {
			continue
		}
		positions[col.Key] = len(out.Columns)
		if col.Source == SourceExportLink {
			if out.LinkRefs == nil {
				out.LinkRefs = map[int]string{}
			}
			out.LinkRefs[len(out.Columns)] = col.LinkSheetID
		}
		out.Columns = append(out.Columns, ColumnInput{
			Header: col.Header, Type: col.Type, Options: slices.Clone(col.Options), Colors: maps.Clone(col.Colors),
			Required: col.Required, Instruction: col.Instruction, Source: col.Source, LinkLabel: col.LinkLabel, PreferredKey: col.Key,
		})
	}
	rule := s.Unique.Normalized()
	out.Unique = UniqueInput{Mode: rule.Mode, OnRepeat: rule.OnRepeat}
	for _, key := range rule.Columns {
		out.Unique.Columns = append(out.Unique.Columns, positions[key])
	}
	return out
}

func CopyOrder(sheets []*Sheet) ([]*Sheet, error) {
	byID := map[string]*Sheet{}
	for _, sheet := range sheets {
		byID[sheet.ID] = sheet
	}
	const (
		visiting = 1
		placed   = 2
	)
	state := map[string]int{}
	var out []*Sheet
	var place func(sheet *Sheet) error
	place = func(sheet *Sheet) error {
		switch state[sheet.ID] {
		case placed:
			return nil
		case visiting:
			return fmt.Errorf("%w: as abas %q formam um ciclo de links", ErrLinkTargetInvalid, sheet.Title)
		}
		state[sheet.ID] = visiting
		for _, col := range sheet.LinkColumns() {
			target, ok := byID[col.LinkSheetID]
			if !ok {
				return fmt.Errorf("%w: coluna %q: a aba de destino não está no arquivo copiado", ErrLinkTargetInvalid, col.Header)
			}
			if err := place(target); err != nil {
				return err
			}
		}
		state[sheet.ID] = placed
		out = append(out, sheet)
		return nil
	}
	for _, sheet := range sheets {
		if err := place(sheet); err != nil {
			return nil, err
		}
	}
	return out, nil
}
