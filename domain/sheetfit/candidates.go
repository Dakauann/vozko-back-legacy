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
	Tabs       []TabChoice
	Choice     Choice
	Chosen     bool
}

func (c Candidate) Choosable() bool {
	if c.Misfit == nil {
		return true
	}
	if len(c.Tabs) == 0 {
		return false
	}
	for _, tab := range c.Tabs {
		if !tab.fittable() {
			return false
		}
	}
	return true
}

func (t TabChoice) fittable() bool {
	for _, option := range t.Options {
		if option.Problem == nil {
			return true
		}
	}
	return false
}

func Candidates(ws Workspace, model Model, chosen Choice) ([]Candidate, error) {
	targets, err := model.targets()
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(ws.Files))
	for _, file := range ws.Files {
		candidate := Candidate{File: file, Choice: Choice{SpreadsheetID: file.ID}}
		if file.ID == chosen.SpreadsheetID {
			candidate.Choice, candidate.Chosen = chosen, true
		}
		assignment, err := FitPinned(model, file, ws.Account, candidate.Choice.Tabs)
		if err != nil && !errors.As(err, &candidate.Misfit) {
			return nil, err
		}
		candidate.Assignment = assignment
		if eligible(file, ws.Account) == nil {
			candidate.Tabs = newFitting(targets, file).choices()
		}
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out, nil
}

func rank(candidate Candidate) int {
	switch {
	case candidate.Misfit != nil:
		return 2
	case candidate.Chosen:
		return 0
	}
	return 1
}
