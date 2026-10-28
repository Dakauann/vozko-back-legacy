package workflow_usecase

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	"vozko/domain/shared/deps"
	"vozko/domain/workflow"
)

type TestValuesForm struct {
	Needs  automationneeds.Needs
	Values map[string]string
}

type TestValueRules interface {
	Rejected(ctx context.Context, workspaceID string, values map[string]string) ([]string, error)
	Label(ctx context.Context, workspaceID string, needs automationneeds.Needs) automationneeds.Needs
}

type TestValuesUseCase struct {
	store workflow.TestValuesRepository
	needs automationneeds.Source
	rules TestValueRules
}

func NewTestValuesUseCase(store workflow.TestValuesRepository, needs automationneeds.Source, rules TestValueRules) (*TestValuesUseCase, error) {
	if err := deps.Require("workflow test values", map[string]any{
		"test values store":  store,
		"automation sources": needs,
		"value rules":        rules,
	}); err != nil {
		return nil, err
	}
	return &TestValuesUseCase{store: store, needs: needs, rules: rules}, nil
}

func (uc *TestValuesUseCase) Form(wf *workflow.Workflow, userID string) (*TestValuesForm, error) {
	needs, err := uc.needsOf(wf)
	if err != nil {
		return nil, err
	}
	needs = uc.rules.Label(context.Background(), wf.WorkspaceID, needs)
	saved, err := uc.Saved(wf, userID)
	if err != nil {
		return nil, err
	}
	return &TestValuesForm{Needs: needs, Values: neededValues(needs, saved)}, nil
}

func (uc *TestValuesUseCase) Saved(wf *workflow.Workflow, userID string) (map[string]string, error) {
	saved, err := uc.store.TestValues(wf.WorkspaceID, wf.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", workflow.ErrTestValuesUnavailable, err)
	}
	return saved, nil
}

func (uc *TestValuesUseCase) Save(wf *workflow.Workflow, userID string, values map[string]string) (*TestValuesForm, error) {
	needs, err := uc.needsOf(wf)
	if err != nil {
		return nil, err
	}
	kept := make(map[string]string, len(values))
	for name, value := range values {
		if needs.Find(name) == nil {
			return nil, fmt.Errorf("%w: %s", workflow.ErrTestValueNotNeeded, name)
		}
		value = strings.TrimSpace(value)
		if utf8.RuneCountInString(value) > conversationvars.MaxValueRunes {
			return nil, fmt.Errorf("%w: %s", workflow.ErrTestValueInvalid, name)
		}
		if value != "" {
			kept[name] = value
		}
	}
	rejected, err := uc.rules.Rejected(context.Background(), wf.WorkspaceID, kept)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", workflow.ErrTestValuesUnavailable, err)
	}
	labeled := uc.rules.Label(context.Background(), wf.WorkspaceID, needs)
	if len(rejected) > 0 {
		labels := make(map[string]string, len(rejected))
		for _, name := range rejected {
			if need := labeled.Find(name); need != nil {
				labels[name] = need.Display()
			}
		}
		return nil, &conversationvars.VariablesError{Cause: workflow.ErrTestValueTypeInvalid, Names: rejected, Labels: labels}
	}
	if err := uc.store.SaveTestValues(wf.WorkspaceID, wf.ID, userID, kept); err != nil {
		return nil, fmt.Errorf("%w: %w", workflow.ErrTestValuesUnavailable, err)
	}
	return &TestValuesForm{Needs: labeled, Values: kept}, nil
}

func (uc *TestValuesUseCase) Clear(wf *workflow.Workflow, userID string) error {
	if err := uc.store.ClearTestValues(wf.WorkspaceID, wf.ID, userID); err != nil {
		return fmt.Errorf("%w: %w", workflow.ErrTestValuesUnavailable, err)
	}
	return nil
}

func (uc *TestValuesUseCase) RunValues(wf *workflow.Workflow, userID string) (map[string]string, error) {
	form, err := uc.Form(wf, userID)
	if err != nil {
		return nil, err
	}
	if missing := form.Needs.Missing(form.Values); len(missing) > 0 {
		return nil, &automationneeds.Refusal{Cause: automationneeds.ErrTestValuesMissing, Automation: testedWorkflow(wf), Needs: missing, Values: form.Values}
	}
	vars, err := automationneeds.VariablesOf(automationneeds.OverlayWorkflow(uc.needs, wf), wf.WorkspaceID, testedWorkflow(wf).Automation)
	if err != nil {
		return nil, err
	}
	return conversationvars.Merge(conversationvars.Lead{Name: simLeadName, Number: simContactNumber}, conversationvars.Campaign{}, vars.Fill(form.Values)), nil
}

func (uc *TestValuesUseCase) needsOf(wf *workflow.Workflow) (automationneeds.Needs, error) {
	if wf == nil {
		return nil, workflow.ErrWorkflowNotFound
	}
	return automationneeds.Of(automationneeds.OverlayWorkflow(uc.needs, wf), wf.WorkspaceID, testedWorkflow(wf).Automation)
}

func testedWorkflow(wf *workflow.Workflow) automationneeds.Link {
	return automationneeds.Link{Automation: conversation.Automation{Kind: conversation.AutomationWorkflow, ID: wf.ID}, Name: wf.Name}
}

func neededValues(needs automationneeds.Needs, saved map[string]string) map[string]string {
	out := make(map[string]string, len(needs))
	for _, need := range needs {
		if value := strings.TrimSpace(saved[need.Name]); value != "" {
			out[need.Name] = value
		}
	}
	return out
}
