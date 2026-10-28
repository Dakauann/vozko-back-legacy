package sheetfit

import (
	"errors"
	"sort"

	"vozko/domain/googlesheet"
)

type Candidate struct {
	File       googlesheet.Spreadsheet
	Assignment Assignment
	Misfit     *Misfit
}

func Candidates(ws Workspace, model Model) ([]Candidate, error) {
	if err := model.Check(); err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(ws.Files))
	for _, file := range ws.Files {
		candidate := Candidate{File: file}
		assignment, err := Fit(model, file, ws.Account)
		if err != nil && !errors.As(err, &candidate.Misfit) {
			return nil, err
		}
		candidate.Assignment = assignment
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Misfit == nil && out[j].Misfit != nil })
	return out, nil
}
