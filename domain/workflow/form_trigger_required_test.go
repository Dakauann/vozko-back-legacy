package workflow

import "testing"

func formTriggerNode(config map[string]interface{}) Node {
	return Node{ID: "t", Type: NodeTypeTriggerFormSubmitted, Config: config}
}

func formsFieldBlocker(report LintReport) (LintIssue, bool) {
	for _, issue := range report.Blocking() {
		if issue.NodeID == "t" && issue.Code == LintMissingRequiredField && issue.Field == FormTriggerFormIDsKey {
			return issue, true
		}
	}
	return LintIssue{}, false
}

func TestTheFormTriggerDefinitionRequiresTheForms(t *testing.T) {
	def, ok := NodeCatalogMap(BuiltinDefinitions())[NodeTypeTriggerFormSubmitted]
	if !ok {
		t.Fatal("the form trigger is not in the catalog")
	}
	for _, field := range def.ConfigSchema {
		if field.Key == FormTriggerFormIDsKey {
			if !field.Required {
				t.Fatalf("the forms field must be required: %+v", field)
			}
			return
		}
	}
	t.Fatal("the form trigger has no forms field")
}

func TestLintBlocksAFormTriggerWithoutForms(t *testing.T) {
	cases := map[string]map[string]interface{}{
		"no config":      nil,
		"a missing list": {},
		"a null list":    {FormTriggerFormIDsKey: nil},
		"an empty list":  {FormTriggerFormIDsKey: []interface{}{}},
		"a typed empty":  {FormTriggerFormIDsKey: []string{}},
		"only blanks":    {FormTriggerFormIDsKey: []interface{}{" ", ""}},
	}
	for name, config := range cases {
		t.Run(name, func(t *testing.T) {
			report := LintGraph(triggerGraph(formTriggerNode(config)), WorkflowTypeMessages, BuiltinDefinitions(), nil, nil)
			if _, found := formsFieldBlocker(report); !found {
				t.Fatalf("a form trigger without forms must block publishing: %+v", report.Issues)
			}
		})
	}
}

func TestLintAcceptsAFormTriggerThatNamesAForm(t *testing.T) {
	report := LintGraph(triggerGraph(formTriggerNode(map[string]interface{}{FormTriggerFormIDsKey: []interface{}{"f1"}})), WorkflowTypeMessages, BuiltinDefinitions(), nil, nil)
	if issue, found := formsFieldBlocker(report); found {
		t.Fatalf("a named form must not block: %+v", issue)
	}
}
