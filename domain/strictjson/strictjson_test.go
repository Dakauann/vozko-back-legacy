package strictjson

import (
	"errors"
	"testing"
)

type sample struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestDecodeAcceptsAKnownDocument(t *testing.T) {
	var got sample
	if err := Decode([]byte(` {"name":"a","count":2} `), &got); err != nil {
		t.Fatal(err)
	}
	if got != (sample{Name: "a", Count: 2}) {
		t.Fatalf("got %+v", got)
	}
}

func TestDecodeRefusesWhatALenientDecoderWouldSwallow(t *testing.T) {
	cases := map[string]string{
		"unknown field":  `{"name":"a","extra":1}`,
		"trailing data":  `{"name":"a"} {"name":"b"}`,
		"trailing junk":  `{"name":"a"} x`,
		"wrong type":     `{"count":"2"}`,
		"empty":          ``,
		"not an object":  `[1]`,
		"truncated":      `{"name":"a"`,
		"null document":  `null`,
		"blank document": `   `,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			var got sample
			if err := Decode([]byte(raw), &got); !errors.Is(err, ErrMalformed) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
