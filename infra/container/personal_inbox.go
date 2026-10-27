package container

import (
	"log"

	"vozko/delivery/http/personalinboxhttp"
	personalinbox_repository "vozko/infra/repositories/personalinbox"
	personalinbox_usecase "vozko/usecases/personalinbox"
)

func (c *Container) wirePersonalInbox() {
	store := personalinbox_repository.New(c.db)
	service, err := personalinbox_usecase.New(personalinbox_usecase.Deps{
		Marks:       store,
		Preferences: store,
		Access:      c.services.conversationAuth,
		Announcer:   c.services.conversationHub,
	})
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	c.services.personalInbox = service
}

func (c *Container) inboxPreferencesHandler() *personalinboxhttp.Handler {
	handler, err := personalinboxhttp.NewHandler(c.services.personalInbox)
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	return handler
}
