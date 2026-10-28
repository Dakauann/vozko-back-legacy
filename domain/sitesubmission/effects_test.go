package sitesubmission

import (
	"errors"
	"slices"
	"testing"
	"time"

	"vozko/domain/site"
)

func TestEffectsArePlannedFromTheFormSettingsAndTheLead(t *testing.T) {
	roulette := &site.Owner{Kind: site.OwnerRoulette, ID: "dept-1"}
	followUp := &site.FollowUp{Kind: site.FollowUpWhatsApp, ID: "bp-1"}
	full := site.FormSettings{Owner: roulette, FunnelID: "pipe-1", WorkflowID: "wf-1", FollowUp: followUp, Notify: true}
	everything := EffectValues{Email: "ana@example.com", EmailOptIn: true, Consent: true}
	cases := map[string]struct {
		in   EffectPlanInput
		want []Effect
	}{
		"a new lead gets everything": {
			EffectPlanInput{LeadID: "l1", LeadCreated: true, Settings: full, Values: everything},
			[]Effect{EffectEmailOptIn, EffectOwner, EffectDeal, EffectConversation, EffectTrigger, EffectNotice, EffectConversion},
		},
		"an unowned existing lead is still placed": {
			EffectPlanInput{LeadID: "l1", Settings: site.FormSettings{Owner: roulette}},
			[]Effect{EffectOwner},
		},
		"an owned existing lead keeps its owner": {
			EffectPlanInput{LeadID: "l1", LeadOwner: "user-2", Settings: site.FormSettings{Owner: roulette, FunnelID: "pipe-1"}},
			[]Effect{EffectDeal},
		},
		"a lead this submission created is placed even when it came owned": {
			EffectPlanInput{LeadID: "l1", LeadCreated: true, LeadOwner: "user-2", Settings: site.FormSettings{Owner: roulette}},
			[]Effect{EffectOwner},
		},
		"no lead means nothing that needs one": {
			EffectPlanInput{Settings: full, Values: everything},
			[]Effect{EffectEmailOptIn, EffectNotice, EffectConversion},
		},
		"the conversion needs the consent": {
			EffectPlanInput{LeadID: "l1", Values: EffectValues{Email: "ana@example.com"}},
			nil,
		},
		"an opt in without an address": {
			EffectPlanInput{Values: EffectValues{EmailOptIn: true}},
			nil,
		},
		"a workflow without a channel opens nothing": {
			EffectPlanInput{LeadID: "l1", Settings: site.FormSettings{WorkflowID: "wf-1"}},
			nil,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := PlanEffects(tc.in); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAnEffectRunWaitsOnWhatItNeeds(t *testing.T) {
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	run := NewEffectRun(Submission{ID: "s1", WorkspaceID: "w1", CreatedAt: at}, EffectPlan{}, []Effect{EffectOwner, EffectDeal, EffectConversation, EffectTrigger})
	if run.SubmissionID != "s1" || run.WorkspaceID != "w1" || !run.NextAttemptAt.Equal(at) || run.Finished() {
		t.Fatalf("run %+v", run)
	}
	if !run.Blocked(EffectDeal) || !run.Blocked(EffectTrigger) || run.Blocked(EffectOwner) || run.Blocked(EffectConversation) {
		t.Fatal("the deal waits for the owner and the trigger for the conversation")
	}
	run.Done(EffectOwner)
	run.Done(EffectConversation)
	if run.Blocked(EffectDeal) || run.Blocked(EffectTrigger) || run.Has(EffectOwner) {
		t.Fatalf("pending %v", run.Pending)
	}
	run.Reopen(EffectOwner)
	run.Reopen(EffectOwner)
	if !run.Has(EffectOwner) || len(run.Pending) != 3 {
		t.Fatalf("pending %v", run.Pending)
	}
}

func TestSettlingAnEffectRunBacksOffThenGivesUp(t *testing.T) {
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	run := NewEffectRun(Submission{ID: "s1", CreatedAt: at}, EffectPlan{}, []Effect{EffectNotice})
	failure := errors.New("notices down")
	run.Settle(at, failure)
	if run.Attempts != 1 || !run.NextAttemptAt.Equal(at.Add(time.Minute)) || run.FinishedAt != nil || run.LastError != "notices down" {
		t.Fatalf("after one failure %+v", run)
	}
	run.Settle(at, failure)
	if !run.NextAttemptAt.Equal(at.Add(2 * time.Minute)) {
		t.Fatalf("the wait doubles: %v", run.NextAttemptAt)
	}
	for run.FinishedAt == nil {
		run.Settle(at, failure)
	}
	if run.Attempts != MaxEffectAttempts || !run.Abandoned() {
		t.Fatalf("given up after %d attempts: %+v", run.Attempts, run)
	}
	if EffectBackoff(40) != maxEffectBackoff {
		t.Fatalf("the wait is capped: %v", EffectBackoff(40))
	}
	done := NewEffectRun(Submission{ID: "s2", CreatedAt: at}, EffectPlan{}, []Effect{EffectNotice})
	done.Done(EffectNotice)
	done.Settle(at.Add(time.Second), nil)
	if done.FinishedAt == nil || !done.FinishedAt.Equal(at.Add(time.Second)) || done.Abandoned() || done.LastError != "" {
		t.Fatalf("a finished run %+v", done)
	}
}
