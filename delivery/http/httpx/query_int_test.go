package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestQueryIntTellsAMissingValueFromAnUnreadableOne(t *testing.T) {
	cases := map[string]struct {
		want int
		ok   bool
	}{
		"/":                          {7, true},
		"/?n=12":                     {12, true},
		"/?n=-3":                     {-3, true},
		"/?n=abc":                    {0, false},
		"/?n=1.5":                    {0, false},
		"/?n=":                       {7, true},
		"/?n=9999999999999999999999": {0, false},
	}
	for target, c := range cases {
		got, ok := QueryInt(httptest.NewRequest("GET", target, nil), "n", 7)
		if ok != c.ok || (ok && got != c.want) {
			t.Fatalf("%s: got %d %v", target, got, ok)
		}
	}
}
