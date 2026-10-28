package sheetvariable

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"vozko/domain/googlesheet"
	"vozko/domain/sheetfit"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

const fallbackSample = "Planilha da franquia"

var errUnwired = errors.New("sheet variables: the service needs the workspace files")

var knownSettings = []string{variabledecl.SettingModel, variabledecl.SettingModelTitle, variabledecl.SettingDefault}

type Files interface {
	FitWorkspace(ctx context.Context, workspaceID string) (sheetfit.Workspace, error)
}

type Service struct {
	files Files
}

func NewService(files Files) (*Service, error) {
	if files == nil {
		return nil, errUnwired
	}
	return &Service{files: files}, nil
}

func (s *Service) Kind() variabledecl.Kind { return variabledecl.KindSheet }

func (s *Service) Shape() variabledecl.Shape { return variabledecl.ShapeSheet }

func (s *Service) Sample(t variabledecl.Type) string {
	if title := strings.TrimSpace(t.Settings[variabledecl.SettingModelTitle]); title != "" {
		return title
	}
	return fallbackSample
}

func (s *Service) CheckType(t variabledecl.Type) error {
	if len(t.Options) > 0 {
		return fmt.Errorf("%w: a Planilha takes no options", variabledecl.ErrTypeInvalid)
	}
	for key := range t.Settings {
		if !slices.Contains(knownSettings, key) {
			return fmt.Errorf("%w: unknown setting %q", variabledecl.ErrTypeInvalid, key)
		}
	}
	if modelOf(t) == "" {
		return fmt.Errorf("%w: a Planilha needs its model file", variabledecl.ErrTypeInvalid)
	}
	return nil
}

func (s *Service) Prepare(ctx context.Context, workspaceID string, t variabledecl.Type) (variabledecl.Type, error) {
	ws, err := s.files.FitWorkspace(ctx, workspaceID)
	if err != nil {
		return variabledecl.Type{}, fmt.Errorf("%w: %w", variabledecl.ErrUnreadable, err)
	}
	model, ok := ws.File(modelOf(t))
	if !ok {
		return variabledecl.Type{}, fmt.Errorf("%w: the model is not a spreadsheet of this workspace", variabledecl.ErrTypeInvalid)
	}
	settings := make(map[string]string, len(t.Settings)+1)
	for key, value := range t.Settings {
		settings[key] = strings.TrimSpace(value)
	}
	settings[variabledecl.SettingModelTitle] = model.Title()
	if fallback := settings[variabledecl.SettingDefault]; fallback != "" {
		if err := wholeFit(ws, model.ID, fallback); err != nil {
			return variabledecl.Type{}, fmt.Errorf("%w: the default spreadsheet: %w", variabledecl.ErrTypeInvalid, err)
		}
	} else {
		delete(settings, variabledecl.SettingDefault)
	}
	return variabledecl.Type{Kind: t.Kind, Options: t.Options, Settings: settings}, nil
}

func (s *Service) Accepts(ctx context.Context, c variabledecl.Check) error {
	if strings.TrimSpace(c.WorkspaceID) == "" {
		return fmt.Errorf("%w: %s is checked inside a workspace", variabledecl.ErrValueInvalid, c.Name)
	}
	ws, err := s.files.FitWorkspace(ctx, c.WorkspaceID)
	if err != nil {
		return fmt.Errorf("sheet variables: %w", err)
	}
	if err := wholeFit(ws, modelOf(c.Type), strings.TrimSpace(c.Value)); err != nil {
		return fmt.Errorf("%w: %s: %w", variabledecl.ErrValueInvalid, c.Name, err)
	}
	return nil
}

func (s *Service) Resolve(ctx context.Context, workspaceID string, t variabledecl.Type, uses []sheetfit.Use, value string) (workflow.SheetFile, error) {
	ws, model, err := s.model(ctx, workspaceID, t, uses)
	if err != nil {
		return workflow.SheetFile{}, err
	}
	file, ok := ws.File(strings.TrimSpace(value))
	if !ok {
		return workflow.SheetFile{}, &sheetfit.Misfit{Code: sheetfit.CodeFileNotFound}
	}
	assignment, err := sheetfit.Fit(model, file, ws.Account)
	if err != nil {
		return workflow.SheetFile{}, err
	}
	return workflow.SheetFile{SpreadsheetID: assignment.SpreadsheetID, Tabs: assignment.Tabs}, nil
}

func (s *Service) Candidates(ctx context.Context, workspaceID string, t variabledecl.Type, uses []sheetfit.Use) ([]sheetfit.Candidate, error) {
	ws, model, err := s.model(ctx, workspaceID, t, uses)
	if err != nil {
		return nil, err
	}
	return sheetfit.Candidates(ws, model)
}

func (s *Service) model(ctx context.Context, workspaceID string, t variabledecl.Type, uses []sheetfit.Use) (sheetfit.Workspace, sheetfit.Model, error) {
	ws, err := s.files.FitWorkspace(ctx, workspaceID)
	if err != nil {
		return sheetfit.Workspace{}, sheetfit.Model{}, fmt.Errorf("sheet variables: %w", err)
	}
	model, ok := ws.Model(modelOf(t))
	if !ok {
		return sheetfit.Workspace{}, sheetfit.Model{}, &sheetfit.Misfit{Code: sheetfit.CodeModelMissing}
	}
	model.Uses = uses
	if len(uses) == 0 {
		model = sheetfit.WholeModel(model.Sheets)
	}
	if err := model.Check(); err != nil {
		return sheetfit.Workspace{}, sheetfit.Model{}, err
	}
	return ws, model, nil
}

func wholeFit(ws sheetfit.Workspace, modelID, value string) error {
	model, ok := ws.Model(modelID)
	if !ok {
		return &sheetfit.Misfit{Code: sheetfit.CodeModelMissing}
	}
	file, ok := ws.File(value)
	if !ok {
		return &sheetfit.Misfit{Code: sheetfit.CodeFileNotFound}
	}
	_, err := sheetfit.Fit(sheetfit.WholeModel(model.Sheets), file, ws.Account)
	return err
}

func modelOf(t variabledecl.Type) string {
	return strings.TrimSpace(t.Settings[variabledecl.SettingModel])
}

func (s *Service) Attention(ctx context.Context, workspaceID string, file workflow.SheetFile) (bool, error) {
	ws, err := s.files.FitWorkspace(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("sheet variables: %w", err)
	}
	found, ok := ws.File(file.SpreadsheetID)
	if !ok {
		return true, nil
	}
	for _, sheet := range found.Sheets {
		for _, tab := range file.Tabs {
			if sheet.ID == tab && sheet.Status != googlesheet.StatusActive {
				return true, nil
			}
		}
	}
	return false, nil
}
