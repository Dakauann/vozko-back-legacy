package workflow_usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"vozko/domain/workflow"
	"vozko/usecases/formlead"
	whatsappform_usecase "vozko/usecases/whatsappform"
	"vozko/usecases/workflow/node_executors"
)

var (
	_ formlead.TriggerFirer           = (*FormTriggerFirer)(nil)
	_ whatsappform_usecase.RunResumer = (*FormRunResumer)(nil)
	_ node_executors.FormSender       = (*whatsappform_usecase.Service)(nil)
)

var ErrFormTriggerIncomplete = errors.New("workflow: a form answer needs its workspace, conversation, form and response to start automations")

type FormTriggerFirer struct {
	evaluator workflow.TriggerEvaluator
}

func NewFormTriggerFirer(evaluator workflow.TriggerEvaluator) *FormTriggerFirer {
	return &FormTriggerFirer{evaluator: evaluator}
}

func (f *FormTriggerFirer) FormSubmitted(_ context.Context, in formlead.Trigger) error {
	required := map[string]string{"workspace": in.WorkspaceID, "entry": in.EntryID, "entry type": in.EntryType, "form": in.Form.ID, "response": in.SubmissionID}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: no %s", ErrFormTriggerIncomplete, name)
		}
	}
	f.evaluator.Evaluate(workflow.TriggerEvent{
		WorkspaceID: in.WorkspaceID,
		EntryID:     in.EntryID,
		EntryType:   in.EntryType,
		TriggerType: workflow.TriggerFormSubmitted,
		Data:        workflow.FormSubmissionData(in.Data, in.Form.ID, in.SubmissionID),
	})
	return nil
}
