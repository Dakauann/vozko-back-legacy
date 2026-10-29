package audiencetest

import (
	"context"
	"errors"

	"vozko/domain/sheetfit"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

var ErrNoSpreadsheets = errors.New("audience test: this channel has no spreadsheets")

type NoSpreadsheets struct{}

func (NoSpreadsheets) Resolve(context.Context, string, variabledecl.Type, []sheetfit.Use, string) (workflow.SheetFile, error) {
	return workflow.SheetFile{}, ErrNoSpreadsheets
}
