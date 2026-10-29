package automationneeds

import (
	"context"
	"errors"
	"fmt"

	"vozko/domain/variabledecl"
)

var (
	ErrNotSheetVariable   = errors.New("automation needs: the variable is not declared as a Planilha")
	ErrSheetModelMismatch = errors.New("automation needs: the tab is not from the model spreadsheet of the variable")
)

type ValueRules interface {
	Rejected(ctx context.Context, workspaceID string, values map[string]string) ([]string, error)
	Label(ctx context.Context, workspaceID string, needs Needs) Needs
}

func RejectedDefaults(ctx context.Context, rules ValueRules, workspaceID string, defaults map[string]string) (Needs, error) {
	if len(defaults) == 0 {
		return nil, nil
	}
	rejected, err := rules.Rejected(ctx, workspaceID, defaults)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	if len(rejected) == 0 {
		return nil, nil
	}
	needs := make(Needs, 0, len(rejected))
	for _, name := range rejected {
		needs = append(needs, Need{Name: name})
	}
	return rules.Label(ctx, workspaceID, needs), nil
}

func SheetModelProblem(labeled Needs, spreadsheetID string) error {
	if len(labeled) != 1 || !labeled[0].Declared || labeled[0].Type.Kind != variabledecl.KindSheet {
		return ErrNotSheetVariable
	}
	if labeled[0].Type.Settings[variabledecl.SettingModel] != spreadsheetID {
		return ErrSheetModelMismatch
	}
	return nil
}
