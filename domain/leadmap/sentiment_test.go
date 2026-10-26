package leadmap

import (
	"testing"

	"vozko/domain/crmfilter"
	"vozko/domain/geo"
	"vozko/domain/shared"
)

func TestTheMoodOfAPositionIsTheSentimentMostOfItsLeadsHoldWithTiesGoingToTheWorse(t *testing.T) {
	tests := []struct {
		name   string
		counts SentimentCounts
		want   shared.Sentiment
	}{
		{"nobody analysed gives no mood", SentimentCounts{}, ""},
		{"one of each reads as negative", SentimentCounts{Positive: 1, Neutral: 1, Negative: 1}, shared.SentimentNegative},
		{"two happy against one angry reads as positive", SentimentCounts{Positive: 2, Negative: 1}, shared.SentimentPositive},
		{"neutral tied with positive reads as neutral", SentimentCounts{Positive: 3, Neutral: 3}, shared.SentimentNeutral},
		{"negative tied with neutral reads as negative", SentimentCounts{Neutral: 2, Negative: 2}, shared.SentimentNegative},
		{"a lone neutral lead reads as neutral", SentimentCounts{Neutral: 1}, shared.SentimentNeutral},
		{"a lone happy lead reads as positive", SentimentCounts{Positive: 1}, shared.SentimentPositive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MoodOf(tt.counts); got != tt.want {
				t.Fatalf("MoodOf(%+v) = %q, want %q", tt.counts, got, tt.want)
			}
		})
	}
}

func TestAnalysedCountsOnlyTheThreeSentiments(t *testing.T) {
	if got := (SentimentCounts{Positive: 2, Neutral: 1, Negative: 4}).Analysed(); got != 7 {
		t.Fatalf("Analysed() = %d, want 7", got)
	}
	if got := (SentimentCounts{}).Analysed(); got != 0 {
		t.Fatalf("Analysed() of nothing = %d", got)
	}
}

func TestNewPointCarriesItsSentimentCountsAndTheirMood(t *testing.T) {
	counts := SentimentCounts{Positive: 1, Negative: 2}
	p := NewPoint(geo.Point{Lat: -23.56, Lng: -46.65}, geo.PrecisionStreet, crmfilter.PlacementOnMap, 4, []string{"a"}, "", counts)
	if p.Sentiment != counts || p.Mood != shared.SentimentNegative {
		t.Fatalf("point = %+v, want the counts kept and a negative mood", p)
	}
	quiet := NewPoint(geo.Point{Lat: -23.56, Lng: -46.65}, geo.PrecisionStreet, crmfilter.PlacementOnMap, 4, []string{"a"}, "", SentimentCounts{})
	if quiet.Mood != "" {
		t.Fatalf("a position nobody was analysed at has no mood, got %q", quiet.Mood)
	}
}

func TestAStoredSentimentOutsideTheThreeGivesNoMark(t *testing.T) {
	for raw, want := range map[string]shared.Sentiment{
		"positive": shared.SentimentPositive,
		"neutral":  shared.SentimentNeutral,
		"negative": shared.SentimentNegative,
		"":         "",
		"angry":    "",
		"Positive": "",
		" neutral": "",
	} {
		if got := SentimentMark(raw); got != want {
			t.Fatalf("SentimentMark(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestCountingAMarkAddsItToItsOwnBucketOnly(t *testing.T) {
	var counts SentimentCounts
	for _, mark := range []shared.Sentiment{shared.SentimentPositive, shared.SentimentNegative, shared.SentimentNegative, "", shared.SentimentNeutral} {
		counts = counts.With(mark)
	}
	if counts != (SentimentCounts{Positive: 1, Neutral: 1, Negative: 2}) {
		t.Fatalf("counts = %+v", counts)
	}
}
