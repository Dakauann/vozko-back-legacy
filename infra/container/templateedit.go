package container

import (
	"log"
	"time"

	template_domain "vozko/domain/whatsapp/template"
	whatsapp_repository "vozko/infra/repositories/whatsapp"
	wc_repository "vozko/infra/repositories/whatsapp_campaign"
	template_usecase "vozko/usecases/whatsapp/template"
)

func (c *Container) templateContentEditor() template_domain.ContentEditor {
	if c.services.fileReader == nil || c.useCases.syncWhatsAppTemplate == nil || c.useCases.setHeaderMediaWhatsApp == nil {
		log.Fatalf("[template-edit] editing needs the media storage, the template sync and the header media use case")
	}
	editor, err := template_usecase.NewEditTemplate(template_usecase.EditTemplateDeps{
		Templates: c.repositories.whatsappTemplate,
		Access:    c.repositories.workspaceTemplateAccess,
		Phones:    c.repositories.businessPhone,
		Campaigns: wc_repository.NewTemplateUse(c.db),
		Edits:     whatsapp_repository.NewTemplateEditLog(c.db),
		Clients:   c.services.whatsappClientFactory,
		Storage:   c.services.fileReader,
		Sync:      c.useCases.syncWhatsAppTemplate,
		Media:     c.useCases.setHeaderMediaWhatsApp,
		Forms:     c.formPublications(),
		Now:       time.Now,
	})
	if err != nil {
		log.Fatalf("[template-edit] %v", err)
	}
	return editor
}
