package leadmap

import "vozko/domain/shared"

type SentimentCounts struct {
	Positive int
	Neutral  int
	Negative int
}

func (c SentimentCounts) Analysed() int {
	return c.Positive + c.Neutral + c.Negative
}

func (c SentimentCounts) With(mark shared.Sentiment) SentimentCounts {
	switch mark {
	case shared.SentimentPositive:
		c.Positive++
	case shared.SentimentNeutral:
		c.Neutral++
	case shared.SentimentNegative:
		c.Negative++
	}
	return c
}

func MoodOf(c SentimentCounts) shared.Sentiment {
	if c.Analysed() == 0 {
		return ""
	}
	if c.Negative >= c.Neutral && c.Negative >= c.Positive {
		return shared.SentimentNegative
	}
	if c.Neutral >= c.Positive {
		return shared.SentimentNeutral
	}
	return shared.SentimentPositive
}

func SentimentMark(raw string) shared.Sentiment {
	if s := shared.Sentiment(raw); s.Valid() {
		return s
	}
	return ""
}

type LeadAt struct {
	ID        string
	Sentiment shared.Sentiment
}
