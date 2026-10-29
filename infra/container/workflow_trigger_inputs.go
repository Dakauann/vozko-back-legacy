package container

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"

	"vozko/domain/siteform"
	"vozko/domain/sitesubmission"
	"vozko/domain/stage"
	wf "vozko/domain/whatsappform"
	workflow_domain "vozko/domain/workflow"
	"vozko/domain/workspacesecret"
	whatsappform_repository "vozko/infra/repositories/whatsappform"
	workflow_repository "vozko/infra/repositories/workflow"
	"vozko/usecases/siteforms"
	workflow_usecase "vozko/usecases/workflow"
)

type triggerFormReads interface {
	Get(ctx context.Context, workspaceID, formID string) (*wf.Form, error)
}

type triggerFormVersions interface {
	Get(ctx context.Context, workspaceID, versionID string) (*wf.Version, error)
}

type triggerPageForms interface {
	LiveForm(ctx context.Context, workspaceID, formKey string) (siteforms.ActiveForm, error)
	PageForm(ctx context.Context, workspaceID, formKey string) (siteforms.PageForm, error)
}

type triggerFormCatalog struct {
	forms    triggerFormReads
	versions triggerFormVersions
	pages    triggerPageForms
}

func (c triggerFormCatalog) pageForm(ctx context.Context, workspaceID, formKey string) (workflow_domain.TriggerForm, error) {
	form, err := c.pages.LiveForm(ctx, workspaceID, formKey)
	if errors.Is(err, sitesubmission.ErrFormUnknown) {
		draft, draftErr := c.pages.PageForm(ctx, workspaceID, formKey)
		if draftErr != nil {
			return workflow_domain.TriggerForm{}, errors.Join(err, draftErr)
		}
		return workflow_domain.TriggerForm{ID: draft.FormKey, Name: draft.FormID}, nil
	}
	if err != nil {
		return workflow_domain.TriggerForm{}, err
	}
	out := workflow_domain.TriggerForm{ID: form.FormKey, Name: form.FormID}
	for _, field := range form.Manifest.Fields {
		prompt := field.Label
		if prompt == "" {
			prompt = field.Name
		}
		question := workflow_domain.TriggerFormQuestion{Key: field.Name, Prompt: prompt, Required: field.Required}
		if field.Kind == siteform.KindSelect || field.Kind == siteform.KindRadio {
			question.Choices = append(question.Choices, field.Options...)
		}
		out.Questions = append(out.Questions, question)
	}
	return out, nil
}

func (c triggerFormCatalog) TriggerForm(workspaceID, formID string) (workflow_domain.TriggerForm, error) {
	ctx := context.Background()
	if _, err := uuid.Parse(formID); err != nil && c.pages != nil && sitesubmission.ValidFormKey(formID) {
		return c.pageForm(ctx, workspaceID, formID)
	}
	form, err := c.forms.Get(ctx, workspaceID, formID)
	if err != nil {
		return workflow_domain.TriggerForm{}, err
	}
	if form == nil || form.WorkspaceID != workspaceID {
		return workflow_domain.TriggerForm{}, fmt.Errorf("%w: %s", wf.ErrFormNotFound, formID)
	}
	versionID := form.PublishedVersionID
	if versionID == "" {
		versionID = form.DraftVersionID
	}
	version, err := c.versions.Get(ctx, workspaceID, versionID)
	if err != nil {
		return workflow_domain.TriggerForm{}, err
	}
	if version == nil {
		return workflow_domain.TriggerForm{}, fmt.Errorf("%w: version %s of form %s", wf.ErrFormNotFound, versionID, formID)
	}
	out := workflow_domain.TriggerForm{ID: form.ID, Name: form.Name}
	for _, ref := range version.Document.Fields() {
		question := workflow_domain.TriggerFormQuestion{Key: ref.Field.Key, Prompt: ref.Field.Prompt(), Required: ref.Field.Required}
		if ref.Field.Kind.IsSingleChoice() {
			for _, option := range ref.Field.Options {
				question.Choices = append(question.Choices, option.Title)
			}
		}
		out.Questions = append(out.Questions, question)
	}
	return out, nil
}

type lazyPageForms struct {
	c *Container
}

func (p lazyPageForms) PageForm(ctx context.Context, workspaceID, formKey string) (siteforms.PageForm, error) {
	return p.c.siteForms().PageForm(ctx, workspaceID, formKey)
}

func (p lazyPageForms) LiveForm(ctx context.Context, workspaceID, formKey string) (siteforms.ActiveForm, error) {
	return p.c.siteForms().LiveForm(ctx, workspaceID, formKey)
}

type triggerStageCatalog struct {
	stages interface {
		ListByWorkspace(workspaceID string) ([]*stage.Stage, error)
	}
}

func (c triggerStageCatalog) TriggerStages(workspaceID string) ([]workflow_domain.TriggerInputOption, error) {
	stages, err := c.stages.ListByWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	options := make([]workflow_domain.TriggerInputOption, 0, len(stages))
	for _, s := range stages {
		if s != nil && s.WorkspaceID == workspaceID {
			options = append(options, workflow_domain.TriggerInputOption{Value: s.ID, Label: s.Name})
		}
	}
	return options, nil
}

func (c *Container) workflowTriggerInputs() workflow_usecase.TriggerInputReaders {
	return workflow_usecase.TriggerInputReaders{
		Forms:  triggerFormCatalog{forms: whatsappform_repository.NewFormStore(c.db), versions: whatsappform_repository.NewVersionStore(c.db), pages: lazyPageForms{c: c}},
		Stages: triggerStageCatalog{stages: c.repositories.stage},
	}
}

func (c *Container) lastTriggerInput() *workflow_usecase.LastTriggerInputUseCase {
	return workflow_usecase.NewLastTriggerInputUseCase(workflow_repository.NewTriggerInputHistory(c.db), c.conversationVisibility(), c.workflowTriggerInputs())
}

type workflowSecretNames struct {
	secrets workspacesecret.UseCases
}

func (s workflowSecretNames) SecretNames(ctx context.Context, workspaceID string) ([]string, error) {
	if s.secrets == nil {
		return nil, fmt.Errorf("the workflow secrets are not wired")
	}
	summaries, err := s.secrets.List(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		names = append(names, summary.Name)
	}
	return names, nil
}

func (c *Container) workflowVariables() *workflow_usecase.VariableCatalogReader {
	return &workflow_usecase.VariableCatalogReader{
		Readers:      c.workflowTriggerInputs(),
		Secrets:      workflowSecretNames{secrets: c.useCases.workspaceSecrets},
		Catalog:      c.useCases.nodeCatalogFn,
		Declarations: c.variableDeclarations(),
	}
}

func (c *Container) workflowTestValues() *workflow_usecase.TestValuesUseCase {
	uc, err := workflow_usecase.NewTestValuesUseCase(workflow_repository.NewTestValuesRepository(c.db), c.automationNeeds().service.Source(), c.automationNeeds().service)
	if err != nil {
		log.Fatalf("[workflows] %v", err)
	}
	return uc
}
