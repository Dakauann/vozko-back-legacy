package container

import (
	"context"
	"errors"
	"log"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/sheetfit"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
	variabledecl_repository "vozko/infra/repositories/variabledecl"
	automationneeds_usecase "vozko/usecases/automationneeds"
	"vozko/usecases/sheetvariable"
	variabledecl_usecase "vozko/usecases/variabledecl"
)

var errSheetsNotBuilt = errors.New("sheet variables: the google sheets pipeline was not built")

func (c *Container) variableDeclarations() *variabledecl_usecase.Service {
	if c.variableDeclarationsSvc != nil {
		return c.variableDeclarationsSvc
	}
	registry, err := variabledecl.NewRegistry(append(variabledecl.Builtin(), c.sheetVariables())...)
	if err != nil {
		log.Fatalf("[variable-declarations] %v", err)
	}
	service, err := variabledecl_usecase.NewService(variabledecl_repository.New(c.db), registry)
	if err != nil {
		log.Fatalf("[variable-declarations] %v", err)
	}
	c.variableDeclarationsSvc = service
	return service
}

func (c *Container) sheetVariables() *sheetvariable.Service {
	if c.sheetVariablesSvc != nil {
		return c.sheetVariablesSvc
	}
	service, err := sheetvariable.NewService(builtSheetFiles{c: c})
	if err != nil {
		log.Fatalf("[sheet-variables] %v", err)
	}
	c.sheetVariablesSvc = service
	return service
}

type builtSheetFiles struct {
	c *Container
}

func (b builtSheetFiles) FitWorkspace(ctx context.Context, workspaceID string) (sheetfit.Workspace, error) {
	if b.c.googleSheets == nil {
		return sheetfit.Workspace{}, errSheetsNotBuilt
	}
	return b.c.googleSheets.Manager.FitWorkspace(ctx, workspaceID)
}

type conversationSheets struct {
	c *Container
}

func (s conversationSheets) AgentSheetFiles(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType, agentID string) (map[string]workflow.SheetFile, error) {
	return s.c.automationNeeds().service.AgentSheetFiles(ctx, workspaceID, entryID, entryType, agentID)
}

type campaignSheets struct {
	c *Container
}

func (s campaignSheets) CampaignSheets(ctx context.Context, workspaceID string, targets []conversation.Automation, values map[string]string) (automationneeds_usecase.SheetLaunch, error) {
	return s.c.automationNeeds().service.CampaignSheets(ctx, workspaceID, targets, values)
}
