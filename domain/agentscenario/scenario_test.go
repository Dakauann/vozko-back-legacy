package agentscenario

import (
	"errors"
	"strings"
	"testing"
)

func validScenario() Scenario {
	return Scenario{
		Name:     "Pedido atrasado",
		Messages: []Message{{Content: "Cadê meu pedido 42?"}},
		Mocks:    map[string]Mock{"t1": {Result: map[string]interface{}{"status": "enviado"}}},
		Assertions: []Assertion{
			{Kind: KindToolCalled, ToolID: "t1", Match: MatchSubset, Arguments: map[string]interface{}{"numero": "42"}},
			{Kind: KindReplyMatches, Match: MatchContains, Value: "enviado"},
		},
	}
}

func TestAValidScenarioPassesTheCheck(t *testing.T) {
	if err := validScenario().Check(); err != nil {
		t.Fatal(err)
	}
}

func TestScenarioLimitsAreRefusedWithTheirErrors(t *testing.T) {
	long := validScenario()
	long.Messages = make([]Message, MaxMessages+1)
	for i := range long.Messages {
		long.Messages[i] = Message{Content: "oi"}
	}
	big := validScenario()
	big.Mocks = map[string]Mock{"t1": {Result: strings.Repeat("x", MaxMockBytes+1)}}
	cases := map[string]struct {
		s    Scenario
		want error
	}{
		"too long":     {long, ErrTooLong},
		"mock large":   {big, ErrMockTooLarge},
		"no name":      {func() Scenario { s := validScenario(); s.Name = " "; return s }(), ErrInvalid},
		"no messages":  {func() Scenario { s := validScenario(); s.Messages = nil; return s }(), ErrInvalid},
		"empty msg":    {func() Scenario { s := validScenario(); s.Messages = []Message{{Content: "  "}}; return s }(), ErrInvalid},
		"bad kind":     {func() Scenario { s := validScenario(); s.Assertions = []Assertion{{Kind: "x"}}; return s }(), ErrInvalid},
		"tool missing": {func() Scenario { s := validScenario(); s.Assertions = []Assertion{{Kind: KindToolCalled}}; return s }(), ErrInvalid},
		"bad regex": {func() Scenario {
			s := validScenario()
			s.Assertions = []Assertion{{Kind: KindReplyMatches, Match: MatchRegex, Value: "("}}
			return s
		}(), ErrInvalid},
		"bad path": {func() Scenario {
			s := validScenario()
			s.Assertions = []Assertion{{Kind: KindToolOutputMatches, ToolID: "t1", Path: "a..b", Match: MatchExact}}
			return s
		}(), ErrInvalid},
		"no asserts": {func() Scenario { s := validScenario(); s.Assertions = nil; return s }(), ErrInvalid},
		"no rubric":  {func() Scenario { s := validScenario(); s.Assertions = []Assertion{{Kind: KindReplyJudged}}; return s }(), ErrInvalid},
		"bad match": {func() Scenario {
			s := validScenario()
			s.Assertions = []Assertion{{Kind: KindReplyMatches, Match: MatchSubset, Value: "x"}}
			return s
		}(), ErrInvalid},
	}
	for name, tc := range cases {
		if err := tc.s.Check(); !errors.Is(err, tc.want) {
			t.Errorf("%s: err %v, want %v", name, err, tc.want)
		}
	}
}

func observed() Observation {
	return Observation{
		Calls: []ObservedCall{{ToolID: "t1", Name: "consultar_pedido", Arguments: map[string]interface{}{"numero": "42", "canal": "whatsapp"}, Output: map[string]interface{}{"data": map[string]interface{}{"status": "enviado", "itens": []interface{}{map[string]interface{}{"id": "a1"}}}}}},
		Reply: "Seu pedido 42 já foi enviado.",
	}
}

func TestToolCalledMatchesExactSubsetAndRegex(t *testing.T) {
	cases := []struct {
		a    Assertion
		pass bool
	}{
		{Assertion{Kind: KindToolCalled, ToolID: "t1"}, true},
		{Assertion{Kind: KindToolCalled, ToolID: "t2"}, false},
		{Assertion{Kind: KindToolCalled, ToolID: "t1", Match: MatchExact, Arguments: map[string]interface{}{"numero": "42", "canal": "whatsapp"}}, true},
		{Assertion{Kind: KindToolCalled, ToolID: "t1", Match: MatchExact, Arguments: map[string]interface{}{"numero": "42"}}, false},
		{Assertion{Kind: KindToolCalled, ToolID: "t1", Match: MatchSubset, Arguments: map[string]interface{}{"numero": "42"}}, true},
		{Assertion{Kind: KindToolCalled, ToolID: "t1", Match: MatchSubset, Arguments: map[string]interface{}{"numero": "43"}}, false},
		{Assertion{Kind: KindToolCalled, ToolID: "t1", Match: MatchRegex, Arguments: map[string]interface{}{"numero": "^4[0-9]$"}}, true},
		{Assertion{Kind: KindToolCalled, ToolID: "t1", Match: MatchRegex, Arguments: map[string]interface{}{"numero": "^5"}}, false},
		{Assertion{Kind: KindToolNotCalled, ToolID: "t2"}, true},
		{Assertion{Kind: KindToolNotCalled, ToolID: "t1"}, false},
	}
	for i, tc := range cases {
		got := Evaluate([]Assertion{tc.a}, observed(), nil)
		if len(got) != 1 || got[0].Passed != tc.pass || got[0].Reason == "" {
			t.Errorf("case %d: %+v", i, got)
		}
	}
}

func TestToolOutputMatchesReadsTheMappedOutputByPath(t *testing.T) {
	cases := []struct {
		a    Assertion
		pass bool
	}{
		{Assertion{Kind: KindToolOutputMatches, ToolID: "t1", Path: "data.status", Match: MatchExact, Value: "enviado"}, true},
		{Assertion{Kind: KindToolOutputMatches, ToolID: "t1", Path: "data.itens[0].id", Match: MatchRegex, Value: "^a"}, true},
		{Assertion{Kind: KindToolOutputMatches, ToolID: "t1", Path: "data.status", Match: MatchExact, Value: "entregue"}, false},
		{Assertion{Kind: KindToolOutputMatches, ToolID: "t1", Path: "data.ausente", Match: MatchExact, Value: ""}, false},
		{Assertion{Kind: KindToolOutputMatches, ToolID: "t9", Path: "data.status", Match: MatchExact, Value: "enviado"}, false},
	}
	for i, tc := range cases {
		if got := Evaluate([]Assertion{tc.a}, observed(), nil); got[0].Passed != tc.pass {
			t.Errorf("case %d: %+v", i, got)
		}
	}
}

func TestReplyMatchesContainsOrRegex(t *testing.T) {
	if got := Evaluate([]Assertion{{Kind: KindReplyMatches, Match: MatchContains, Value: "ENVIADO"}}, observed(), nil); !got[0].Passed {
		t.Fatalf("contains is case insensitive: %+v", got)
	}
	if got := Evaluate([]Assertion{{Kind: KindReplyMatches, Match: MatchRegex, Value: `pedido \d+ já`}}, observed(), nil); !got[0].Passed {
		t.Fatalf("regex: %+v", got)
	}
	if got := Evaluate([]Assertion{{Kind: KindReplyMatches, Match: MatchContains, Value: "cancelado"}}, observed(), nil); got[0].Passed {
		t.Fatalf("absent text passed: %+v", got)
	}
}

func TestAJudgedReplyUsesTheVerdictAndFailsClosedWithoutOne(t *testing.T) {
	judged := []Assertion{{Kind: KindReplyJudged, Rubric: "Educado e informa o status"}}
	got := Evaluate(judged, observed(), map[int]Verdict{0: {Passed: true, Reason: "Informou o status."}})
	if !got[0].Passed || got[0].Reason != "Informou o status." {
		t.Fatalf("got %+v", got)
	}
	if got := Evaluate(judged, observed(), nil); got[0].Passed {
		t.Fatalf("no verdict must fail: %+v", got)
	}
}

func TestPassedNeedsEveryAssertion(t *testing.T) {
	if !Passed([]AssertionResult{{Passed: true}, {Passed: true}}) || Passed([]AssertionResult{{Passed: true}, {Passed: false}}) || Passed(nil) {
		t.Fatal("passed rule")
	}
}
