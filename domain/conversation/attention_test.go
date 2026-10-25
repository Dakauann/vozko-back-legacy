package conversation

import "testing"

func TestCustomerMessageAlertsOnlyTheRightViewers(t *testing.T) {
	cases := map[string]struct {
		attention MessageAttention
		viewer    string
		want      bool
	}{
		"the human owner is alerted": {
			attention: MessageAttention{Owner: "u-owner", Distribution: DistributionManual, Takers: []string{"u-owner", "u-colleague"}},
			viewer:    "u-owner",
			want:      true,
		},
		"a colleague of the owner is not alerted": {
			attention: MessageAttention{Owner: "u-owner", Distribution: DistributionManual, Takers: []string{"u-owner", "u-colleague"}},
			viewer:    "u-colleague",
			want:      false,
		},
		"an admin who sees the owned conversation is not alerted": {
			attention: MessageAttention{Owner: "u-owner", Distribution: DistributionAutomatic},
			viewer:    "u-admin",
			want:      false,
		},
		"an agent owner alerts nobody": {
			attention: MessageAttention{Owner: "ai:agent-1", Distribution: DistributionManual, Takers: []string{"u-taker"}},
			viewer:    "u-taker",
			want:      false,
		},
		"a workflow owner alerts nobody": {
			attention: MessageAttention{Owner: "workflow:flow-1", Distribution: DistributionManual, Takers: []string{"u-taker"}},
			viewer:    "u-taker",
			want:      false,
		},
		"an unowned conversation the roulette deals alerts nobody": {
			attention: MessageAttention{Distribution: DistributionAutomatic, Takers: []string{"u-taker"}},
			viewer:    "u-taker",
			want:      false,
		},
		"an unowned conversation in the manual queue alerts a member who can take it": {
			attention: MessageAttention{Distribution: DistributionManual, Takers: []string{"u-taker"}},
			viewer:    "u-taker",
			want:      true,
		},
		"an unowned conversation in the manual queue leaves out an admin who cannot take it": {
			attention: MessageAttention{Distribution: DistributionManual, Takers: []string{"u-taker"}},
			viewer:    "u-admin",
			want:      false,
		},
		"an unreadable distribution still alerts the human owner": {
			attention: MessageAttention{Owner: "u-owner", Distribution: DistributionUnknown},
			viewer:    "u-owner",
			want:      true,
		},
		"an unreadable distribution alerts nobody else": {
			attention: MessageAttention{Owner: "u-owner", Distribution: DistributionUnknown, Takers: []string{"u-taker"}},
			viewer:    "u-taker",
			want:      false,
		},
		"an unreadable distribution on an unowned conversation alerts nobody": {
			attention: MessageAttention{Distribution: DistributionUnknown, Takers: []string{"u-taker"}},
			viewer:    "u-taker",
			want:      false,
		},
		"a viewer without an id is never alerted": {
			attention: MessageAttention{Distribution: DistributionManual, Takers: []string{""}},
			viewer:    "",
			want:      false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.attention.Alerts(tc.viewer); got != tc.want {
				t.Fatalf("Alerts(%q) = %v, want %v", tc.viewer, got, tc.want)
			}
		})
	}
}

func TestAnAssignmentAlertsOnlyAPersonSomeoneElseGaveItTo(t *testing.T) {
	cases := map[string]struct {
		assignee, assignedBy string
		want                 bool
	}{
		"the roulette hands it to a person":      {assignee: "u-1", assignedBy: "system", want: true},
		"a colleague assigns it to a person":     {assignee: "u-1", assignedBy: "u-2", want: true},
		"an agent hands it off to a person":      {assignee: "u-1", assignedBy: "ai:agent-1", want: true},
		"a person takes it themselves":           {assignee: "u-1", assignedBy: "u-1", want: false},
		"it goes to an agent":                    {assignee: "ai:agent-1", assignedBy: "system", want: false},
		"it goes to a workflow":                  {assignee: "workflow:flow-1", assignedBy: "u-2", want: false},
		"it is released to the team":             {assignee: "", assignedBy: "system", want: false},
		"it is handed to the system placeholder": {assignee: "system", assignedBy: "u-2", want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := AlertsAssignee(tc.assignee, tc.assignedBy); got != tc.want {
				t.Fatalf("AlertsAssignee(%q, %q) = %v, want %v", tc.assignee, tc.assignedBy, got, tc.want)
			}
		})
	}
}
