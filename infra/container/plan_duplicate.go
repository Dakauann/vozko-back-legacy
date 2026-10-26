package container

import (
	"log"

	"vozko/delivery/http/handlers"
	workspace_plan_repository "vozko/infra/repositories/workspace_plan"
	workspace_plan_usecase "vozko/usecases/workspace_plan"
)

func (c *Container) planDuplicateHandler() *handlers.WorkspacePlanDuplicateHandler {
	wiring := workspace_plan_usecase.DuplicatePlanDeps{
		Plans:        c.repositories.workspacePlan,
		Duplications: workspace_plan_repository.NewPlanDuplicationRepository(c.db),
	}
	duplicate, err := workspace_plan_usecase.NewDuplicatePlanDefinitionUseCase(wiring)
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	preview, err := workspace_plan_usecase.NewPreviewPlanDuplicateUseCase(wiring)
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	handler, err := handlers.NewWorkspacePlanDuplicateHandler(handlers.WorkspacePlanDuplicateDeps{Duplicate: duplicate, Preview: preview})
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	return handler
}
