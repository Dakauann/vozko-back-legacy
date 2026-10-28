package sitesubmission

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"vozko/domain/siteform"
	"vozko/domain/strictjson"
)

const (
	MaxBodyBytes       = 64 << 10
	maxUTMRunes        = 256
	maxReferrerRunes   = 2048
	maxClickIDRunes    = 512
	maxVariantRunes    = 64
	maxRouteRunes      = 512
	maxChallengeRunes  = 512
	maxUserAgentRunes  = 512
	identityProblemKey = "form"
)

var idempotencyKey = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

type UTM struct {
	Source   string `json:"source"`
	Medium   string `json:"medium"`
	Campaign string `json:"campaign"`
	Term     string `json:"term"`
	Content  string `json:"content"`
}

func (u UTM) values() []string {
	return []string{u.Source, u.Medium, u.Campaign, u.Term, u.Content}
}

func (u UTM) Empty() bool {
	return !slices.ContainsFunc(u.values(), func(v string) bool { return strings.TrimSpace(v) != "" })
}

type Body struct {
	UTM            UTM                        `json:"utm"`
	Referrer       string                     `json:"referrer"`
	Fbclid         string                     `json:"fbclid"`
	Gclid          string                     `json:"gclid"`
	Fbp            string                     `json:"fbp"`
	Fbc            string                     `json:"fbc"`
	Variant        string                     `json:"variant"`
	IdempotencyKey string                     `json:"idempotencyKey"`
	Website        string                     `json:"website"`
	Route          string                     `json:"route"`
	Fields         map[string]json.RawMessage `json:"fields"`
	ChallengeToken string                     `json:"challengeToken"`
	Nonce          string                     `json:"nonce"`
}

func bodyError(reason string) error {
	return fmt.Errorf("%w: %s", ErrBodyInvalid, reason)
}

func DecodeBody(raw []byte) (Body, error) {
	if len(raw) > MaxBodyBytes {
		return Body{}, bodyError("the body is too large")
	}
	var b Body
	if err := strictjson.Decode(raw, &b); err != nil {
		return Body{}, fmt.Errorf("%w: %w", ErrBodyInvalid, err)
	}
	if b.Fields == nil {
		return Body{}, bodyError("fields are required")
	}
	if !idempotencyKey.MatchString(b.IdempotencyKey) {
		return Body{}, bodyError("the idempotency key is invalid")
	}
	if strings.TrimSpace(b.ChallengeToken) == "" || utf8.RuneCountInString(b.ChallengeToken) > maxChallengeRunes {
		return Body{}, bodyError("the challenge token is required")
	}
	if !strings.HasPrefix(b.Route, "/") || strings.HasPrefix(b.Route, "//") || utf8.RuneCountInString(b.Route) > maxRouteRunes {
		return Body{}, bodyError("the route must be a path")
	}
	limits := []struct {
		value string
		max   int
	}{
		{b.Referrer, maxReferrerRunes}, {b.Fbclid, maxClickIDRunes}, {b.Gclid, maxClickIDRunes},
		{b.Fbp, maxClickIDRunes}, {b.Fbc, maxClickIDRunes}, {b.Variant, maxVariantRunes},
	}
	for _, v := range b.UTM.values() {
		limits = append(limits, struct {
			value string
			max   int
		}{v, maxUTMRunes})
	}
	for _, l := range limits {
		if utf8.RuneCountInString(l.value) > l.max {
			return Body{}, bodyError("a tracking value is too long")
		}
	}
	for name, value := range b.Fields {
		if !validFieldValue(value) {
			return Body{}, bodyError("field " + name + " must be text or a list of text")
		}
	}
	return b, nil
}

func validFieldValue(raw json.RawMessage) bool {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return true
	}
	var list []string
	return json.Unmarshal(raw, &list) == nil
}

func (b Body) Values() (map[string]string, []siteform.Problem) {
	values := make(map[string]string, len(b.Fields))
	var problems []siteform.Problem
	names := make([]string, 0, len(b.Fields))
	for name := range b.Fields {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		var text string
		if json.Unmarshal(b.Fields[name], &text) == nil {
			values[name] = text
			continue
		}
		problems = append(problems, siteform.Problem{Field: name, Code: siteform.CodeValueInvalid})
	}
	return values, problems
}

func ReferrerOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return ""
	}
	return u.Scheme + "://" + strings.ToLower(u.Host)
}

func TruncateUserAgent(raw string) string {
	raw = strings.TrimSpace(raw)
	if utf8.RuneCountInString(raw) <= maxUserAgentRunes {
		return raw
	}
	return string([]rune(raw)[:maxUserAgentRunes])
}

func Details(problems []siteform.Problem, manifest siteform.Manifest) map[string]string {
	details := map[string]string{}
	for _, p := range problems {
		if p.Field != "" {
			details[p.Field] = p.Code
			continue
		}
		named := false
		for _, target := range []string{siteform.TargetPhone, siteform.TargetEmail} {
			if _, ok := manifest.Field(target); ok {
				details[target] = p.Code
				named = true
			}
		}
		if !named {
			details[identityProblemKey] = p.Code
		}
	}
	return details
}
