package sitesubmission

import (
	"slices"
	"time"

	"vozko/domain/site"
	"vozko/domain/siteform"
)

type Effect string

const (
	EffectEmailOptIn   Effect = "email_opt_in"
	EffectOwner        Effect = "owner"
	EffectDeal         Effect = "deal"
	EffectConversation Effect = "conversation"
	EffectTrigger      Effect = "trigger"
	EffectNotice       Effect = "notice"
	EffectConversion   Effect = "conversion"
)

const (
	MaxEffectAttempts  = 12
	firstEffectBackoff = time.Minute
	maxEffectBackoff   = 6 * time.Hour
	stillPending       = "effects are still pending"
)

var effectOrder = []Effect{EffectEmailOptIn, EffectOwner, EffectDeal, EffectConversation, EffectTrigger, EffectNotice, EffectConversion}

var effectNeeds = map[Effect]Effect{EffectDeal: EffectOwner, EffectTrigger: EffectConversation}

func EffectOrder() []Effect {
	return slices.Clone(effectOrder)
}

type EffectValues struct {
	Name       string `json:"name,omitempty"`
	Phone      string `json:"phone,omitempty"`
	Email      string `json:"email,omitempty"`
	Consent    bool   `json:"consent,omitempty"`
	EmailOptIn bool   `json:"email_opt_in,omitempty"`
}

func (v EffectValues) For(field string) string {
	switch field {
	case siteform.TargetPhone:
		return v.Phone
	case siteform.TargetEmail:
		return v.Email
	case siteform.TargetName:
		return v.Name
	}
	return ""
}

type EffectPlan struct {
	Settings  site.FormSettings `json:"settings"`
	Values    EffectValues      `json:"values"`
	LeadOwner string            `json:"lead_owner,omitempty"`
	SourceURL string            `json:"source_url,omitempty"`
}

type EffectPlanInput struct {
	LeadID      string
	LeadCreated bool
	LeadOwner   string
	Settings    site.FormSettings
	Values      EffectValues
}

func PlanEffects(in EffectPlanInput) []Effect {
	lead := in.LeadID != ""
	s := in.Settings
	wanted := map[Effect]bool{
		EffectEmailOptIn:   in.Values.EmailOptIn && in.Values.Email != "",
		EffectOwner:        lead && s.Owner != nil && (in.LeadCreated || in.LeadOwner == ""),
		EffectDeal:         lead && s.FunnelID != "",
		EffectConversation: lead && s.WorkflowID != "" && s.FollowUp != nil,
		EffectTrigger:      lead && s.WorkflowID != "" && s.FollowUp != nil,
		EffectNotice:       s.Notify,
		EffectConversion:   in.Values.Consent,
	}
	var out []Effect
	for _, e := range effectOrder {
		if wanted[e] {
			out = append(out, e)
		}
	}
	return out
}

type EffectRun struct {
	SubmissionID  string
	WorkspaceID   string
	Plan          EffectPlan
	Pending       []Effect
	OwnerID       string
	EntryID       string
	EntryType     string
	Attempts      int
	NextAttemptAt time.Time
	FinishedAt    *time.Time
	LastError     string
	CreatedAt     time.Time
	Submission    Submission
}

func NewEffectRun(sub Submission, plan EffectPlan, pending []Effect) EffectRun {
	return EffectRun{
		SubmissionID: sub.ID, WorkspaceID: sub.WorkspaceID, Plan: plan, Pending: slices.Clone(pending),
		NextAttemptAt: sub.CreatedAt, CreatedAt: sub.CreatedAt, Submission: sub,
	}
}

func (r EffectRun) Has(e Effect) bool {
	return slices.Contains(r.Pending, e)
}

func (r EffectRun) Blocked(e Effect) bool {
	need, ok := effectNeeds[e]
	return ok && r.Has(need)
}

func (r *EffectRun) Done(e Effect) {
	r.Pending = slices.DeleteFunc(r.Pending, func(p Effect) bool { return p == e })
}

func (r *EffectRun) Reopen(e Effect) {
	if r.Has(e) {
		return
	}
	r.Pending = append(r.Pending, e)
	slices.SortFunc(r.Pending, func(a, b Effect) int { return slices.Index(effectOrder, a) - slices.Index(effectOrder, b) })
}

func (r EffectRun) Finished() bool {
	return len(r.Pending) == 0
}

func (r EffectRun) Abandoned() bool {
	return r.FinishedAt != nil && !r.Finished()
}

func (r *EffectRun) Settle(now time.Time, failure error) {
	if r.Finished() {
		r.FinishedAt, r.LastError = &now, ""
		return
	}
	r.Attempts++
	r.LastError = stillPending
	if failure != nil {
		r.LastError = failure.Error()
	}
	if r.Attempts >= MaxEffectAttempts {
		r.FinishedAt = &now
		return
	}
	r.NextAttemptAt = now.Add(EffectBackoff(r.Attempts))
}

func EffectBackoff(attempt int) time.Duration {
	wait := firstEffectBackoff
	for i := 1; i < attempt && wait < maxEffectBackoff; i++ {
		wait *= 2
	}
	return min(wait, maxEffectBackoff)
}
