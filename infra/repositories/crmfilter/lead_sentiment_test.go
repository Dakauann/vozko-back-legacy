package crmfilter

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/lib/pq"

	"vozko/domain/crmfilter"
	"vozko/domain/shared"
)

const sentimentHeld = "leads.id IN (SELECT ls_f.lead_id FROM lead_sentiments ls_f WHERE ls_f.workspace_id = ? AND ls_f.sentiment = ANY(?::text[]))"

func TestTheSentimentFieldReadsTheProjectedSentimentOfTheLead(t *testing.T) {
	cases := []struct {
		name     string
		pred     crmfilter.Predicate
		wantSQL  string
		wantArgs []interface{}
	}{
		{"the angry ones", pred(crmfilter.FieldSentiment, crmfilter.OpEquals, "negative"),
			sentimentHeld, []interface{}{testWorkspace, pq.Array([]string{"negative"})}},
		{"the happy or neutral ones", pred(crmfilter.FieldSentiment, crmfilter.OpIn, "positive", "neutral"),
			sentimentHeld, []interface{}{testWorkspace, pq.Array([]string{"positive", "neutral"})}},
		{"everyone but the angry ones, unanalysed included", pred(crmfilter.FieldSentiment, crmfilter.OpNotIn, "negative"),
			"NOT COALESCE(" + sentimentHeld + ", false)", []interface{}{testWorkspace, pq.Array([]string{"negative"})}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := compileLead(t, scopedLeadDesc(), tc.pred)
			if want := "(" + tc.wantSQL + ")"; sql != want {
				t.Fatalf("SQL\n got: %s\nwant: %s", sql, want)
			}
			if got := strings.Count(sql, "?"); got != len(args) {
				t.Fatalf("%d placeholders for %d args in %s", got, len(args), sql)
			}
			if !reflect.DeepEqual(args, tc.wantArgs) {
				t.Fatalf("args = %#v, want %#v", args, tc.wantArgs)
			}
		})
	}
}

func TestTheSentimentFieldNeedsTheWorkspace(t *testing.T) {
	f := crmfilter.Filter{Groups: []crmfilter.Group{{Conjunction: crmfilter.And, Predicates: []crmfilter.Predicate{pred(crmfilter.FieldSentiment, crmfilter.OpIn, "negative")}}}}
	if _, _, err := Compile(f, NewLeadDescriptor(), 0); !errors.Is(err, ErrWorkspaceScopeRequired) {
		t.Fatalf("Compile() = %v, want ErrWorkspaceScopeRequired", err)
	}
}

func TestTheSentimentFieldRefusesAValueTheAnalysisNeverWrites(t *testing.T) {
	f := crmfilter.Filter{Groups: []crmfilter.Group{{Conjunction: crmfilter.And, Predicates: []crmfilter.Predicate{pred(crmfilter.FieldSentiment, crmfilter.OpIn, "angry")}}}}
	if _, _, err := Compile(f, scopedLeadDesc(), 0); !errors.Is(err, crmfilter.ErrInvalidValue) {
		t.Fatalf("Compile() = %v, want ErrInvalidValue", err)
	}
}

func TestSentimentCountsCountOnlyTheThreeDomainSentiments(t *testing.T) {
	want := "COUNT(*) FILTER (WHERE ls.sentiment = '" + string(shared.SentimentPositive) + "') AS positive," +
		" COUNT(*) FILTER (WHERE ls.sentiment = '" + string(shared.SentimentNeutral) + "') AS neutral," +
		" COUNT(*) FILTER (WHERE ls.sentiment = '" + string(shared.SentimentNegative) + "') AS negative"
	if got := SentimentCountsSQL("ls"); got != want {
		t.Fatalf("SentimentCountsSQL\n got: %s\nwant: %s", got, want)
	}
}

func TestTheSentimentJoinMatchesTheLeadInsideItsWorkspace(t *testing.T) {
	want := "LEFT JOIN lead_sentiments ls ON ls.lead_id = a.lead_id AND ls.workspace_id = a.workspace_id"
	if got := LeadSentimentJoin("ls", "a.lead_id", "a.workspace_id"); got != want {
		t.Fatalf("LeadSentimentJoin\n got: %s\nwant: %s", got, want)
	}
}
