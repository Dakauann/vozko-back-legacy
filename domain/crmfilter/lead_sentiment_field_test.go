package crmfilter

import (
	"errors"
	"testing"
)

func TestTheSentimentFieldTakesOnlyTheThreeAnalysisSentiments(t *testing.T) {
	cases := []struct {
		name string
		p    Predicate
		want error
	}{
		{"one sentiment", pred(FieldSentiment, OpEquals, "negative"), nil},
		{"several sentiments", pred(FieldSentiment, OpIn, "positive", "neutral"), nil},
		{"everything but the angry ones", pred(FieldSentiment, OpNotIn, "negative"), nil},
		{"a sentiment the analysis never writes", pred(FieldSentiment, OpIn, "angry"), ErrInvalidValue},
		{"a sentiment in another case", pred(FieldSentiment, OpIn, "Negative"), ErrInvalidValue},
		{"a presence test", pred(FieldSentiment, OpIsSet), ErrUnsupportedOp},
		{"a text search", pred(FieldSentiment, OpContains, "neg"), ErrUnsupportedOp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := filterOf(tc.p).Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("Validate() = %v, want %v", err, tc.want)
			}
		})
	}
}
