package agentscenario

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"vozko/domain/agenttool"
)

type ObservedCall struct {
	ToolID    string                 `json:"toolId"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
	Output    interface{}            `json:"output"`
	IsError   bool                   `json:"isError"`
	Mocked    bool                   `json:"mocked"`
}

type Observation struct {
	Calls []ObservedCall
	Reply string
}

type Verdict struct {
	Passed bool
	Reason string
}

func Evaluate(assertions []Assertion, seen Observation, verdicts map[int]Verdict) []AssertionResult {
	out := make([]AssertionResult, len(assertions))
	for i, a := range assertions {
		passed, reason := evaluate(i, a, seen, verdicts)
		out[i] = AssertionResult{Index: i, Kind: a.Kind, Passed: passed, Reason: reason}
	}
	return out
}

func Passed(results []AssertionResult) bool {
	if len(results) == 0 {
		return false
	}
	for _, r := range results {
		if !r.Passed {
			return false
		}
	}
	return true
}

func evaluate(index int, a Assertion, seen Observation, verdicts map[int]Verdict) (bool, string) {
	switch a.Kind {
	case KindToolCalled:
		return toolCalled(a, callsOf(seen, a.ToolID))
	case KindToolNotCalled:
		if calls := callsOf(seen, a.ToolID); len(calls) > 0 {
			return false, fmt.Sprintf("a ferramenta foi chamada %d vez(es)", len(calls))
		}
		return true, "a ferramenta não foi chamada"
	case KindToolOutputMatches:
		return outputMatches(a, callsOf(seen, a.ToolID))
	case KindReplyMatches:
		if textMatches(a.Match, a.Value, seen.Reply) {
			return true, "a resposta confere"
		}
		return false, "a resposta não confere: " + seen.Reply
	case KindReplyJudged:
		verdict, ok := verdicts[index]
		if !ok {
			return false, "o avaliador não deu um veredito"
		}
		return verdict.Passed, verdict.Reason
	}
	return false, "tipo de verificação desconhecido"
}

func toolCalled(a Assertion, calls []ObservedCall) (bool, string) {
	if len(calls) == 0 {
		return false, "a ferramenta não foi chamada"
	}
	if len(a.Arguments) == 0 {
		return true, fmt.Sprintf("a ferramenta foi chamada %d vez(es)", len(calls))
	}
	for _, c := range calls {
		if argumentsMatch(a.Match, a.Arguments, c.Arguments) {
			return true, "a ferramenta foi chamada com os argumentos esperados"
		}
	}
	return false, "a ferramenta foi chamada, mas com outros argumentos: " + encoded(calls[0].Arguments)
}

func outputMatches(a Assertion, calls []ObservedCall) (bool, string) {
	if len(calls) == 0 {
		return false, "a ferramenta não foi chamada"
	}
	for _, c := range calls {
		value, found := agenttool.ValueAtPath(c.Output, a.Path)
		if found && textMatches(a.Match, a.Value, text(value)) {
			return true, fmt.Sprintf("%s = %s", a.Path, text(value))
		}
	}
	value, found := agenttool.ValueAtPath(calls[len(calls)-1].Output, a.Path)
	if !found {
		return false, fmt.Sprintf("a saída não tem %s", a.Path)
	}
	return false, fmt.Sprintf("%s = %s", a.Path, text(value))
}

func callsOf(seen Observation, toolID string) []ObservedCall {
	out := []ObservedCall{}
	for _, c := range seen.Calls {
		if c.ToolID == toolID {
			out = append(out, c)
		}
	}
	return out
}

func argumentsMatch(match Match, want, got map[string]interface{}) bool {
	switch match {
	case MatchExact:
		return sameJSON(want, got)
	case MatchRegex:
		for key, pattern := range want {
			value, ok := got[key]
			if !ok {
				return false
			}
			p, _ := pattern.(string)
			re, err := regexp.Compile(p)
			if err != nil || !re.MatchString(text(value)) {
				return false
			}
		}
		return true
	default:
		for key, value := range want {
			actual, ok := got[key]
			if !ok || !sameJSON(value, actual) {
				return false
			}
		}
		return true
	}
}

func textMatches(match Match, want, got string) bool {
	switch match {
	case MatchRegex:
		re, err := regexp.Compile(want)
		return err == nil && re.MatchString(got)
	case MatchContains:
		return strings.Contains(strings.ToLower(got), strings.ToLower(want))
	default:
		return got == want
	}
}

func sameJSON(a, b interface{}) bool {
	var left, right interface{}
	if json.Unmarshal([]byte(encoded(a)), &left) != nil || json.Unmarshal([]byte(encoded(b)), &right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}

func encoded(v interface{}) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

func text(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return encoded(v)
}
