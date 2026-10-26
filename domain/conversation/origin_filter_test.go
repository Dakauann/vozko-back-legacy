package conversation

import (
	"errors"
	"reflect"
	"testing"

	"vozko/domain/crmfilter"
)

func originPredicate(op crmfilter.Operator, values ...string) crmfilter.Predicate {
	return crmfilter.Predicate{Field: crmfilter.FieldOrigin, Operator: op, Values: values}
}

func TestEachOriginPredicateNamesTheOriginsItAllows(t *testing.T) {
	cases := []struct {
		name string
		p    crmfilter.Predicate
		want []Origin
	}{
		{"in", originPredicate(crmfilter.OpIn, "receptive", "campaign"), []Origin{OriginReceptive, OriginCampaign}},
		{"eq", originPredicate(crmfilter.OpEquals, "outreach"), []Origin{OriginOutreach}},
		{"not in keeps every other recorded origin", originPredicate(crmfilter.OpNotIn, "campaign"), []Origin{OriginReceptive, OriginOutreach, OriginUnknown}},
		{"neq", originPredicate(crmfilter.OpNotEquals, "receptive", " "), []Origin{OriginCampaign, OriginOutreach, OriginUnknown}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := OriginsAllowedBy(tc.p)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v %v, want %v", got, err, tc.want)
			}
		})
	}
}

func TestAnOriginPredicateRefusesWhatTheVocabularyLacks(t *testing.T) {
	if _, err := OriginsAllowedBy(originPredicate(crmfilter.OpIn, "receptive", "inbound")); !errors.Is(err, ErrOriginInvalid) || !errors.Is(err, crmfilter.ErrInvalidValue) {
		t.Fatalf("err = %v, want an invalid origin value", err)
	}
	if _, err := OriginsAllowedBy(originPredicate(crmfilter.OpIsSet)); !errors.Is(err, crmfilter.ErrUnsupportedOp) {
		t.Fatalf("err = %v, want an unsupported operator", err)
	}
}

func TestTheOriginScopeIntersectsEveryPredicateAllRowsMustMeet(t *testing.T) {
	label := crmfilter.Predicate{Field: crmfilter.FieldLabel, Operator: crmfilter.OpIn, Values: []string{"l-1"}}
	f := crmfilter.Filter{Groups: []crmfilter.Group{{Conjunction: crmfilter.And, Predicates: []crmfilter.Predicate{
		originPredicate(crmfilter.OpIn, "receptive", "campaign"), originPredicate(crmfilter.OpNotIn, "campaign"), label,
	}}}}

	origins, rest, err := OriginScopeOf(f)

	if err != nil || !reflect.DeepEqual(origins, []Origin{OriginReceptive}) {
		t.Fatalf("origins = %v %v, want receptive", origins, err)
	}
	if want := (crmfilter.Filter{Groups: []crmfilter.Group{{Conjunction: crmfilter.And, Predicates: []crmfilter.Predicate{label}}}}); !reflect.DeepEqual(rest, want) {
		t.Fatalf("rest = %+v", rest)
	}
}

func TestContradictoryOriginsMatchNothingAndNoOriginMeansNoScope(t *testing.T) {
	contradiction := crmfilter.Filter{Groups: []crmfilter.Group{{Conjunction: crmfilter.And, Predicates: []crmfilter.Predicate{
		originPredicate(crmfilter.OpEquals, "receptive"), originPredicate(crmfilter.OpEquals, "campaign"),
	}}}}
	origins, _, err := OriginScopeOf(contradiction)
	if err != nil || origins == nil || len(origins) != 0 {
		t.Fatalf("a contradiction must allow no origin at all, got %v %v", origins, err)
	}
	none, _, err := OriginScopeOf(crmfilter.Filter{})
	if err != nil || none != nil {
		t.Fatalf("without an origin predicate there is no origin scope, got %v %v", none, err)
	}
}

func TestOneSearchOriginIsAScopeOfOne(t *testing.T) {
	if got := OriginScope(OriginReceptive); !reflect.DeepEqual(got, []Origin{OriginReceptive}) {
		t.Fatalf("OriginScope(receptive) = %v", got)
	}
	if got := OriginScope(""); got != nil {
		t.Fatalf("OriginScope(\"\") = %v, want no scope", got)
	}
}
