package container

import (
	"context"
	"log"

	"vozko/delivery/http/whatsappformhttp"
	"vozko/domain/copilot"
	"vozko/domain/shared/deps"
	"vozko/domain/whatsappform"
	workflow_domain "vozko/domain/workflow"
	"vozko/infra/meta"
	whatsappform_repository "vozko/infra/repositories/whatsappform"
	"vozko/infra/whatsapp/flows"
	conversation_usecase "vozko/usecases/conversation"
	"vozko/usecases/copilot/copilottools/formtools"
	"vozko/usecases/formlead"
	template_usecase "vozko/usecases/whatsapp/template"
	whatsappform_usecase "vozko/usecases/whatsappform"
	workflow_usecase "vozko/usecases/workflow"
)

func (c *Container) flowsGateway() *flows.Gateway {
	client, err := meta.NewClient(meta.Config{
		Host:       "graph.facebook.com",
		APIVersion: meta.DefaultGraphVersion,
		AppSecret:  c.cfg.MetaAppSecret,
	})
	if err != nil {
		log.Fatalf("[whatsapp-forms] the Meta Flows client: %v", err)
	}
	return flows.NewGateway(client)
}

func (c *Container) whatsappForms() *whatsappform_usecase.Service {
	if c.whatsappFormsService != nil {
		return c.whatsappFormsService
	}
	if c.services.messageSender == nil || c.services.conversationAuthImpl == nil || c.services.whatsappClientFactory == nil || c.useCases.personDeals == nil {
		log.Fatalf("[whatsapp-forms] the message sender, the authorizer, the WhatsApp clients and the deal scope must be wired first")
	}
	service, err := whatsappform_usecase.New(whatsappform_usecase.Deps{
		Forms:         whatsappform_repository.NewFormStore(c.db),
		Versions:      whatsappform_repository.NewVersionStore(c.db),
		Publications:  whatsappform_repository.NewPublicationStore(c.db),
		Sends:         whatsappform_repository.NewSendStore(c.db),
		Responses:     whatsappform_repository.NewResponseStore(c.db),
		Gateway:       c.flowsGateway(),
		Phones:        c.repositories.businessPhone,
		OwnedPhones:   formOwnedPhones{phones: c.repositories.businessPhone},
		Permissions:   c.services.conversationAuthImpl,
		Definitions:   c.repositories.customField,
		Assets:        formAssets{media: c.repositories.media, client: formImageClient()},
		Sender:        formSender{clients: c.services.whatsappClientFactory},
		Conversations: formConversations{sender: c.services.messageSender},
		Access:        c.services.conversationAuthImpl,
		Scopes:        formReadScopes{deals: c.useCases.personDeals},
		History:       formHistory{sender: c.services.messageSender},
		FormLeads:     c.formLeads(),
		Media:         formFiles{store: conversation_usecase.NewFormFileStore(c.services.whatsappClientFactory, c.mediaStore(), nil)},
		Runs:          c.formRunResumer(),
		LeadNames:     formLeadNames{leads: c.repositories.lead},
		Automations:   c.formAutomations(),
		Notices:       c.notifications().Publisher,
	})
	if err != nil {
		log.Fatalf("[whatsapp-forms] %v", err)
	}
	c.whatsappFormsService = service
	return service
}

func (c *Container) formLeads() *formlead.Service {
	service, err := formlead.New(formlead.Deps{
		Phones:   c.repositories.lead,
		Emails:   c.repositories.lead,
		Profiles: c.leadProfiles(),
		Consents: c.leadConsent(),
		Triggers: c.formTriggers(),
	})
	if err != nil {
		log.Fatalf("[form-leads] %v", err)
	}
	return service
}

func (c *Container) wireWhatsAppForms() {
	forms := c.whatsappForms()
	if err := c.formRepliesHandle().Bind(formReplies{forms: forms}); err != nil {
		log.Fatalf("[whatsapp-forms] %v", err)
	}
	c.formSenderRelay.Bind(forms)
	if err := c.formSendsHandle().Bind(forms); err != nil {
		log.Fatalf("[whatsapp-forms] %v", err)
	}
	consumer := whatsappform_usecase.NewFlowConsumer(whatsappform_usecase.FlowConsumerDeps{
		QueueSub:    c.services.webhookQueueSub,
		QueuePub:    c.services.webhookQueuePub,
		SharedState: c.redisProvider.SharedState(),
		Durable:     c.processedWebhookEvents(),
		Handler:     forms,
	})
	if err := consumer.Start(); err != nil {
		log.Fatalf("[whatsapp-forms] the flows webhook consumer: %v", err)
	}
}

func (c *Container) whatsappFormHandler() *whatsappformhttp.Handler {
	return whatsappformhttp.NewHandler(c.whatsappForms())
}

func (c *Container) formPublications() *whatsappform_repository.PublicationStore {
	return whatsappform_repository.NewPublicationStore(c.db)
}

func (c *Container) formSendsHandle() *deps.Handle[template_usecase.FormSends] {
	if c.formSends == nil {
		c.formSends = deps.Tracked[template_usecase.FormSends](&c.handles, "whatsapp form sends")
	}
	return c.formSends
}

type lateFormSends struct {
	forms *deps.Handle[template_usecase.FormSends]
}

func (l lateFormSends) PrepareTemplateSend(ctx context.Context, in whatsappform_usecase.TemplateSend) (whatsappform.Send, error) {
	forms, err := l.forms.Get()
	if err != nil {
		return whatsappform.Send{}, err
	}
	return forms.PrepareTemplateSend(ctx, in)
}

func (l lateFormSends) SettleTemplateSend(ctx context.Context, token, messageID string, sendErr error) error {
	forms, err := l.forms.Get()
	if err != nil {
		return err
	}
	return forms.SettleTemplateSend(ctx, token, messageID, sendErr)
}

func (c *Container) wireFormWorkflows(engine *workflow_usecase.RunEngine) {
	if c.useCases.triggerEvaluator == nil || c.useCases.createWorkflow == nil || c.useCases.opportunity == nil {
		log.Fatalf("[whatsapp-forms] the trigger evaluator, workflow creation and deals must exist before the form workflows")
	}
	published, ok := c.repositories.workflow.(workflow_domain.PublishedWorkflows)
	if !ok {
		log.Fatalf("[whatsapp-forms] the workflow store cannot read published graphs for a whole workspace")
	}
	automations, err := workflow_usecase.NewFormAutomations(published, c.useCases.createWorkflow, c.useCases.opportunity)
	if err != nil {
		log.Fatalf("[whatsapp-forms] form automations: %v", err)
	}
	c.formRuns = workflow_usecase.NewFormRunResumer(c.repositories.workflow, c.repositories.workflowRun, engine)
	c.formTriggerFirer = workflow_usecase.NewFormTriggerFirer(c.useCases.triggerEvaluator)
	c.formAutomationCatalog = automations
}

func (c *Container) formRunResumer() *workflow_usecase.FormRunResumer {
	if c.formRuns == nil {
		log.Fatalf("[whatsapp-forms] the form run resumer is built with the workflow engine")
	}
	return c.formRuns
}

func (c *Container) formTriggers() *workflow_usecase.FormTriggerFirer {
	if c.formTriggerFirer == nil {
		log.Fatalf("[whatsapp-forms] the form trigger is built with the workflow engine")
	}
	return c.formTriggerFirer
}

func (c *Container) formAutomations() *workflow_usecase.FormAutomations {
	if c.formAutomationCatalog == nil {
		log.Fatalf("[whatsapp-forms] the form automations are built with the workflow engine")
	}
	return c.formAutomationCatalog
}

func (c *Container) whatsAppFormTools() []copilot.Tool {
	return formtools.FormTools(formtools.FormDeps{
		Forms:   c.whatsappForms(),
		Reports: c.services.reportService,
		Entries: c.services.conversationEntryLookup,
	})
}
