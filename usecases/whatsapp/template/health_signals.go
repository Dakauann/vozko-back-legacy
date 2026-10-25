package template_usecase

import (
	"fmt"

	"vozko/domain/whatsapp/template"
)

type TemplateHealthSignals interface {
	TemplateChanged(externalID string)
}

func (h *handleTemplateWebhook) signalChanged(recordErr error, v template.TemplateWebhookValue) {
	if recordErr != nil || v.MessageTemplateID == 0 {
		return
	}
	h.signals.TemplateChanged(fmt.Sprintf("%d", v.MessageTemplateID))
}
