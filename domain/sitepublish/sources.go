package sitepublish

import (
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type SourceKind string

const (
	SourceUTM      SourceKind = "utm"
	SourceReferrer SourceKind = "referrer"

	MaxSourceValueLength = 200
)

type SourceStat struct {
	ProjectID string
	Day       time.Time
	Kind      SourceKind
	Value     string
	Views     int64
	Submits   int64
}

type SourceTotal struct {
	Value   string `json:"value"`
	Views   int64  `json:"views"`
	Submits int64  `json:"submits"`
}

func readable(value string) bool {
	if value == "" || len(value) > MaxSourceValueLength || !utf8.ValidString(value) {
		return false
	}
	return !strings.ContainsFunc(value, unicode.IsControl)
}

func ValidSource(kind SourceKind, value string) bool {
	if !readable(value) {
		return false
	}
	switch kind {
	case SourceUTM:
		return true
	case SourceReferrer:
		u, err := url.Parse(value)
		return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && u.Scheme+"://"+u.Host == value
	}
	return false
}

func RankSources(stats []SourceStat, kind SourceKind, exclude func(string) bool, limit int) []SourceTotal {
	byValue := map[string]*SourceTotal{}
	for _, s := range stats {
		if s.Kind != kind || (exclude != nil && exclude(s.Value)) {
			continue
		}
		total, ok := byValue[s.Value]
		if !ok {
			total = &SourceTotal{Value: s.Value}
			byValue[s.Value] = total
		}
		total.Views += s.Views
		total.Submits += s.Submits
	}
	out := make([]SourceTotal, 0, len(byValue))
	for _, total := range byValue {
		out = append(out, *total)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Views != out[j].Views {
			return out[i].Views > out[j].Views
		}
		if out[i].Submits != out[j].Submits {
			return out[i].Submits > out[j].Submits
		}
		return out[i].Value < out[j].Value
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
