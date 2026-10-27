package container

import (
	"log"

	conversationhttp "vozko/delivery/http/conversation"
	conversation_usecase "vozko/usecases/conversation"
)

func (c *Container) conversationHandler() *conversationhttp.ConversationHandler {
	toggle := c.automationToggle()
	handler, err := conversationhttp.NewConversationHandler(conversationhttp.ConversationHandlerDeps{
		UploadMedia:           c.useCases.uploadConversationMedia,
		GetMedia:              c.useCases.getConversationMedia,
		ListEvents:            c.useCases.listConversationEvents,
		RequestCallPermission: c.services.messageSender,
		Automation:            toggle,
		Delegation:            c.automationDelegation(toggle),
		ToolCalls:             c.listAIToolCalls(),
		EmailHTML:             c.emailHTMLViewer(),
		ResponsibleWorkflow: conversation_usecase.NewResponsibleWorkflowView(conversation_usecase.ResponsibleWorkflowViewDeps{
			Gate:       c.services.automationGate,
			Visibility: c.conversationVisibility(),
			Workflows:  c.repositories.workflow,
		}),
	})
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	return handler
}
