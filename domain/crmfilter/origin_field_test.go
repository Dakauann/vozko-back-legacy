package crmfilter

import (
	"errors"
	"reflect"
	"testing"
)

func TestTheOriginFieldSelectsOneOrSeveralOrigins(t *testing.T) {
	cases := []struct {
		name string
		p    Predicate
		want error
	}{
		{"one origin", pred(FieldOrigin, OpEquals, "receptive"), nil},
		{"several origins", pred(FieldOrigin, OpIn, "receptive", "campaign"), nil},
		{"everything but one origin", pred(FieldOrigin, OpNotIn, "outreach"), nil},
		{"no origin at all", pred(FieldOrigin, OpIn), ErrMissingValue},
		{"a presence test", pred(FieldOrigin, OpIsSet), ErrUnsupportedOp},
		{"a text search", pred(FieldOrigin, OpContains, "rec"), ErrUnsupportedOp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := filterOf(tc.p).Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("Validate() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestHoistTakesOnlyThePredicatesEveryRowMustMeet(t *testing.T) {
	origin := pred(FieldOrigin, OpIn, "receptive")
	label := pred(FieldLabel, OpIn, "l-1")
	f := Filter{Groups: []Group{
		{Conjunction: And, Predicates: []Predicate{origin, label}},
		{Predicates: []Predicate{pred(FieldOrigin, OpNotIn, "campaign")}},
		{Conjunction: Or, Predicates: []Predicate{pred(FieldOrigin, OpIn, "outreach"), label}},
	}}

	hoisted, rest := f.Hoist(FieldOrigin)

	if len(hoisted) != 2 || hoisted[0].Operator != OpIn || hoisted[1].Operator != OpNotIn {
		t.Fatalf("hoisted = %+v, want the AND group's and the lone predicate's origins", hoisted)
	}
	want := Filter{Groups: []Group{
		{Conjunction: And, Predicates: []Predicate{label}},
		{Conjunction: Or, Predicates: []Predicate{pred(FieldOrigin, OpIn, "outreach"), label}},
	}}
	if !reflect.DeepEqual(rest, want) {
		t.Fatalf("rest = %+v\nwant %+v", rest, want)
	}
}

func TestHoistLeavesAFilterWithoutTheFieldAlone(t *testing.T) {
	f := filterOf(pred(FieldLabel, OpIn, "l-1"))
	hoisted, rest := f.Hoist(FieldOrigin)
	if hoisted != nil || !reflect.DeepEqual(rest, f) {
		t.Fatalf("hoisted %v rest %+v", hoisted, rest)
	}
}
