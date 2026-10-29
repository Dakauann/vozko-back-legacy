package sitesubmission

import "vozko/domain/workflow"

const triggerSource = "site_form"

func (s Submission) TriggerData() map[string]any {
	answers := make(map[string]any, len(s.Fields))
	for k, v := range s.Fields {
		answers[k] = v
	}
	data := workflow.FormSubmittedRunData(s.FormKey, s.ID, answers)
	data["source"], data["project_id"], data["form_key"], data["page_form_id"] = triggerSource, s.ProjectID, s.FormKey, s.FormID
	data["route"], data["variant"] = s.Route, s.Variant
	data["utm"] = map[string]string{"source": s.UTM.Source, "medium": s.UTM.Medium, "campaign": s.UTM.Campaign, "term": s.UTM.Term, "content": s.UTM.Content}
	return data
}
