package container

import (
	"log"

	"vozko/delivery/http/tutorialhttp"
	tutorial_repository "vozko/infra/repositories/tutorial"
	tutorial_usecase "vozko/usecases/tutorial"
)

func (c *Container) tutorialHandler() *tutorialhttp.Handler {
	service, err := tutorial_usecase.New(tutorial_usecase.Deps{
		Progress:  tutorial_repository.NewProgress(c.db),
		SiteFacts: tutorial_repository.NewSiteFacts(c.db),
	})
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	handler, err := tutorialhttp.NewHandler(service)
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	return handler
}
